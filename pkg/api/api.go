package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/auth"
	"deploystackr/pkg/cluster"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/git"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/mcp"
	"deploystackr/pkg/provider"
	"deploystackr/pkg/store"
	"deploystackr/pkg/webhook"
)

// Server sets up HTTP routing and orchestrates API calls.
type Server struct {
	cfg            *config.Config
	store          *store.MemoryStore
	indexer        *indexer.Indexer
	deployer       *deployer.Manager
	dispatcher     *webhook.Dispatcher
	clusterChecker *cluster.Checker
	distFS         fs.FS
	authMgr        *auth.AuthManager
	mcpServer      *mcp.Server
	mux            *http.ServeMux

	releasePRsMu         sync.Mutex
	releasePRsCache      map[string]map[string][]int
	contextVersionsMu    sync.Mutex
	contextVersionsCache map[string][]provider.VersionInfo
}

// NewServer initializes the API server with all subsystems.
func NewServer(cfg *config.Config, s *store.MemoryStore, idx *indexer.Indexer, dep *deployer.Manager, distFS fs.FS) *Server {
	srv := &Server{
		cfg:                  cfg,
		store:                s,
		indexer:              idx,
		deployer:             dep,
		dispatcher:           webhook.NewDispatcher(cfg),
		clusterChecker:       cluster.NewChecker(cfg.Cluster),
		distFS:               distFS,
		authMgr:              auth.NewAuthManager(cfg.Auth),
		mux:                  http.NewServeMux(),
		releasePRsCache:      make(map[string]map[string][]int),
		contextVersionsCache: make(map[string][]provider.VersionInfo),
	}
	srv.mcpServer = mcp.NewServer(mcp.Deps{
		Config:         cfg,
		Store:          s,
		Indexer:        idx,
		Deployer:       dep,
		Dispatcher:     srv.dispatcher,
		Auth:           srv.authMgr,
		ClusterChecker: func() *cluster.Checker { return srv.clusterChecker },
	})
	srv.routes()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for development (support direct connection from dev servers e.g. localhost:5173)
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-GitHub-Token, X-Forwarded-User, Accept, Mcp-Session-Id, MCP-Protocol-Version")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/health", s.handleHealth)

	s.mux.HandleFunc("/api/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("/api/auth/providers", s.handleAuthProviders)
	s.mux.HandleFunc("/api/auth/login/", s.handleAuthLogin)
	s.mux.HandleFunc("/api/auth/callback/", s.handleAuthCallback)
	s.mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)

	s.mux.HandleFunc("/api/services", s.handleServices)
	s.mux.HandleFunc("/api/services/", s.handleServiceSubroutes)
	s.mux.HandleFunc("/api/promote", s.handlePromote)
	s.mux.HandleFunc("/api/webhooks/release", s.handleWebhookRelease)
	s.mux.HandleFunc("/api/webhooks/pr", s.handleWebhookPR)
	s.mux.HandleFunc("/api/webhooks", s.handleWebhooksList)
	s.mux.HandleFunc("/api/webhook", s.handleWebhook)
	s.mux.Handle("/mcp", s.mcpServer)
	s.mux.Handle("/api/mcp", s.mcpServer)

	// Frontend SPA static server
	if s.distFS != nil {
		fileServer := http.FileServer(http.FS(s.distFS))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/mcp/") {
				http.NotFound(w, r)
				return
			}

			// Check if file exists in distFS
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}

			f, err := s.distFS.Open(path)
			if err != nil {
				// Fallback to index.html for SPA client-side routing
				indexBytes, err := fs.ReadFile(s.distFS, "index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(indexBytes)
				return
			}
			f.Close()

			fileServer.ServeHTTP(w, r)
		})
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// EnvironmentPermission represents per-environment deployment capability for the user.
type EnvironmentPermission struct {
	CanDeploy bool   `json:"can_deploy"`
	Reason    string `json:"reason,omitempty"`
}

// AuthMeResponse represents current authentication session state.
type AuthMeResponse struct {
	Authenticated bool       `json:"authenticated"`
	User          *auth.User `json:"user,omitempty"`
}

// DevPersona represents an available persona in dev mode.
type DevPersona struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
}

// AuthProvidersResponse represents enabled login providers.
type AuthProvidersResponse struct {
	Providers         []string     `json:"providers"`
	ConnectedProvider string       `json:"connected_provider"`
	DevMode           bool         `json:"dev_mode"`
	Personas          []DevPersona `json:"personas,omitempty"`
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	user := s.authMgr.GetUserFromSession(r)
	if user == nil {
		writeJSON(w, http.StatusOK, AuthMeResponse{Authenticated: false})
		return
	}

	if s.cfg != nil {
		deployReposMap := make(map[string]bool)
		if s.cfg.GitOps.Repo != "" {
			deployReposMap[s.cfg.GitOps.Repo] = true
		}
		for _, svc := range s.cfg.Services {
			for _, env := range svc.Environments {
				if env.Repo != "" {
					deployReposMap[env.Repo] = true
				}
			}
		}
		var deployRepos []string
		for r := range deployReposMap {
			deployRepos = append(deployRepos, r)
		}
		sort.Strings(deployRepos)
		user.DeployRepos = deployRepos
	}

	writeJSON(w, http.StatusOK, AuthMeResponse{
		Authenticated: true,
		User:          user,
	})
}

func (s *Server) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	primary := s.authMgr.PrimaryProvider()
	if primary == "" {
		primary = "dev"
	}

	devMode := false
	var personas []DevPersona

	if _, ok := s.authMgr.GetProvider("dev"); ok {
		devMode = true
		personas = []DevPersona{
			{
				ID:          "dev_admin",
				Name:        "Alex Rivera",
				Role:        "Platform Admin",
				Description: "Member of org/devops, org/platform-team (Full Access)",
			},
			{
				ID:          "dev_billing",
				Name:        "Sarah Chen",
				Role:        "Billing Lead",
				Description: "Member of org/billing-team (Billing Service Access)",
			},
			{
				ID:          "dev_guest",
				Name:        "Guest Developer",
				Role:        "Viewer",
				Description: "Read-only viewer (No Deploy Access)",
			},
		}
	}

	writeJSON(w, http.StatusOK, AuthProvidersResponse{
		Providers:         []string{primary},
		ConnectedProvider: primary,
		DevMode:           devMode,
		Personas:          personas,
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	provName := strings.TrimPrefix(r.URL.Path, "/api/auth/login/")
	provName = strings.TrimSpace(provName)
	if provName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider required"})
		return
	}

	p, ok := s.authMgr.GetProvider(provName)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("provider '%s' not found", provName)})
		return
	}

	state := r.URL.Query().Get("persona")
	if state == "" {
		state = "state"
	}

	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" {
		returnTo = r.URL.Query().Get("redirect")
	}

	// Dev provider: instant local session creation without external roundtrips
	if provName == "dev" {
		persona := state
		if persona == "" || persona == "state" {
			persona = "dev_admin"
		}
		user, _ := p.HandleCallback(r.Context(), persona, "")
		_, cookieVal := s.authMgr.CreateSession(user)
		http.SetCookie(w, &http.Cookie{
			Name:     "deploystackr_session",
			Value:    cookieVal,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   7 * 86400,
		})

		if r.Header.Get("Accept") == "application/json" || r.URL.Query().Get("format") == "json" {
			writeJSON(w, http.StatusOK, AuthMeResponse{Authenticated: true, User: user})
			return
		}

		returnURL := "/"
		if returnTo != "" && strings.HasPrefix(returnTo, "/") {
			returnURL = returnTo
		} else if ref := r.Header.Get("Referer"); ref != "" {
			if u, err := url.Parse(ref); err == nil && u.Host != "" {
				returnURL = fmt.Sprintf("%s://%s/", u.Scheme, u.Host)
			}
		}
		http.Redirect(w, r, returnURL, http.StatusTemporaryRedirect)
		return
	}

	if returnTo != "" && strings.HasPrefix(returnTo, "/") {
		http.SetCookie(w, &http.Cookie{
			Name:     "deploystackr_return_to",
			Value:    returnTo,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600,
		})
	}

	redirectURI := s.getRedirectURI(r, provName)
	loginURL := p.LoginURL(state, redirectURI)
	http.Redirect(w, r, loginURL, http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	provName := strings.TrimPrefix(r.URL.Path, "/api/auth/callback/")
	provName = strings.TrimSpace(provName)

	p, ok := s.authMgr.GetProvider(provName)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("provider '%s' not found", provName)})
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		if errMsg == "" {
			errMsg = "missing code parameter"
		}
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(errMsg), http.StatusTemporaryRedirect)
		return
	}

	redirectURI := s.getRedirectURI(r, provName)
	user, err := p.HandleCallback(r.Context(), code, redirectURI)
	if err != nil {
		log.Printf("[auth] callback error for provider %s: %v", provName, err)
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(err.Error()), http.StatusTemporaryRedirect)
		return
	}

	_, cookieVal := s.authMgr.CreateSession(user)
	http.SetCookie(w, &http.Cookie{
		Name:     "deploystackr_session",
		Value:    cookieVal,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 86400,
	})

	returnURL := "/"
	if c, err := r.Cookie("deploystackr_return_to"); err == nil && c.Value != "" && strings.HasPrefix(c.Value, "/") {
		returnURL = c.Value
		http.SetCookie(w, &http.Cookie{
			Name:     "deploystackr_return_to",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	} else if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host != "" {
			returnURL = fmt.Sprintf("%s://%s/", u.Scheme, u.Host)
		}
	} else if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		returnURL = fmt.Sprintf("http://%s/", fwdHost)
	}

	http.Redirect(w, r, returnURL, http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.authMgr.DeleteSession(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "deploystackr_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) getRedirectURI(r *http.Request, provName string) string {
	proto := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	host := r.Host
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host != "" {
			host = u.Host
			proto = u.Scheme
		}
	} else if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return fmt.Sprintf("%s://%s/api/auth/callback/%s", proto, host, provName)
}

// TopEnvStatus summarizes how the highest-priority environment compares to the latest release.
type TopEnvStatus struct {
	Environment    string           `json:"environment"`
	ActiveVersion  string           `json:"active_version"`
	LatestVersion  string           `json:"latest_version"`
	VersionsBehind int              `json:"versions_behind"`
	IsUpToDate     bool             `json:"is_up_to_date"`
	PendingPR      *store.PendingPR `json:"pending_pr,omitempty"`
}

// ServiceResponse is returned by GET /api/services
type ServiceResponse struct {
	Service             string                           `json:"service"`
	DisplayName         string                           `json:"display_name,omitempty"`
	Icon                string                           `json:"icon,omitempty"`
	GitOpsRepo          string                           `json:"gitops_repo,omitempty"`
	DeployRepos         []string                         `json:"deploy_repos,omitempty"`
	Source              config.SourceConfig              `json:"source"`
	Context             config.ContextConfig             `json:"context"`
	Permissions         *config.ServicePermissions       `json:"permissions,omitempty"`
	EnvironmentPriority []string                         `json:"environment_priority,omitempty"`
	Environments        map[string]store.EnvironmentView `json:"environments"`
	UserPermissions     map[string]EnvironmentPermission `json:"user_permissions"`
	TopEnvStatus        *TopEnvStatus                    `json:"top_env_status,omitempty"`
	LatestVersion       string                           `json:"latest_version,omitempty"`
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authMgr.GetUserFromDeploymentRequest(r)

	orderedNames := make([]string, 0, len(s.cfg.Services))
	seen := make(map[string]bool)

	for _, name := range s.cfg.ServiceOrder {
		if _, ok := s.cfg.Services[name]; ok && !seen[name] {
			orderedNames = append(orderedNames, name)
			seen[name] = true
		}
	}

	var remaining []string
	for name := range s.cfg.Services {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	orderedNames = append(orderedNames, remaining...)

	results := make([]ServiceResponse, 0, len(orderedNames))
	for _, svcName := range orderedNames {
		svcCfg := s.cfg.Services[svcName]
		envViews := make(map[string]store.EnvironmentView)
		userPerms := make(map[string]EnvironmentPermission)

		for envName, envCfg := range svcCfg.Environments {
			activeEvt := s.store.GetActiveEvent(svcName, envName)
			recentEvts := s.store.GetEnvironmentHistory(svcName, envName)
			if len(recentEvts) > 5 {
				recentEvts = recentEvts[:5]
			}

			repo := envCfg.Repo
			if repo == "" && s.cfg != nil {
				repo = s.cfg.GitOps.Repo
			}

			view := store.EnvironmentView{
				Repo:          repo,
				WriteStrategy: envCfg.WriteStrategy.Mode,
				TargetFile:    envCfg.Target.File,
				TargetPath:    envCfg.Target.Path,
				RecentEvents:  recentEvts,
			}

			if activeEvt != nil {
				view.ActiveVersion = activeEvt.Version
				view.LastDeployed = &activeEvt.DeployedAt
				view.DeployedBy = activeEvt.AuthorName
				view.CommitSHA = activeEvt.CommitSHA
				view.PRNumber = activeEvt.PRNumber
			} else {
				view.ActiveVersion = "unknown"
			}

			// Include pending deployment PR if one is open
			pendingPR := s.store.GetPendingPR(svcName, envName)
			if pendingPR != nil {
				targetRepo := envCfg.Repo
				if targetRepo == "" && s.cfg != nil {
					targetRepo = s.cfg.GitOps.Repo
				}
				if activeEvt != nil && (activeEvt.Version == pendingPR.TargetVersion || strings.TrimPrefix(activeEvt.Version, "v") == strings.TrimPrefix(pendingPR.TargetVersion, "v")) {
					s.store.ClearPendingPR(svcName, envName)
					pendingPR = nil
				} else if pendingPR.PRNumber > 0 && isPRClosed(r.Context(), targetRepo, pendingPR.PRNumber) {
					s.store.ClearPendingPR(svcName, envName)
					pendingPR = nil
				}
			}
			view.PendingPR = pendingPR

			envViews[envName] = view

			// Evaluate user permissions for this environment
			if user != nil {
				canDeploy, reason := auth.EvaluatePermission(user, svcCfg, envCfg)
				userPerms[envName] = EnvironmentPermission{
					CanDeploy: canDeploy,
					Reason:    reason,
				}
			} else {
				userPerms[envName] = EnvironmentPermission{
					CanDeploy: false,
					Reason:    "Sign in to deploy",
				}
			}
		}

		envPriority := svcCfg.EnvironmentPriority
		if len(envPriority) == 0 && s.cfg != nil {
			envPriority = s.cfg.EnvironmentPriority
		}

		svcDeployReposMap := make(map[string]bool)
		gitopsRepo := ""
		if s.cfg != nil && s.cfg.GitOps.Repo != "" {
			gitopsRepo = s.cfg.GitOps.Repo
			svcDeployReposMap[gitopsRepo] = true
		}
		for _, envCfg := range svcCfg.Environments {
			if envCfg.Repo != "" {
				svcDeployReposMap[envCfg.Repo] = true
			}
		}
		var svcDeployRepos []string
		for r := range svcDeployReposMap {
			svcDeployRepos = append(svcDeployRepos, r)
		}
		sort.Strings(svcDeployRepos)

		topStatus := computeTopEnvStatus(svcName, svcCfg, envViews, envPriority, s.store)
		latestVer := ""
		if topStatus != nil {
			latestVer = topStatus.LatestVersion
		}

		results = append(results, ServiceResponse{
			Service:             svcName,
			DisplayName:         svcCfg.DisplayName,
			Icon:                svcCfg.Icon,
			GitOpsRepo:          gitopsRepo,
			DeployRepos:         svcDeployRepos,
			Source:              svcCfg.Source,
			Context:             svcCfg.Context,
			Permissions:         svcCfg.Permissions,
			EnvironmentPriority: envPriority,
			Environments:        envViews,
			UserPermissions:     userPerms,
			TopEnvStatus:        topStatus,
			LatestVersion:       latestVer,
		})
	}

	writeJSON(w, http.StatusOK, results)
}

func extractVersionTag(v string) string {
	v = strings.TrimSpace(v)
	if idx := strings.LastIndex(v, ":"); idx != -1 {
		v = v[idx+1:]
	}
	if idx := strings.Index(v, "@"); idx != -1 {
		v = v[:idx]
	}
	return strings.TrimPrefix(v, "v")
}

func areVersionsEqual(v1, v2 string) bool {
	if v1 == v2 {
		return true
	}
	t1 := extractVersionTag(v1)
	t2 := extractVersionTag(v2)
	if t1 != "" && t1 == t2 {
		return true
	}
	if (v1 != "" && strings.HasSuffix(v1, ":"+v2)) || (v2 != "" && strings.HasSuffix(v2, ":"+v1)) {
		return true
	}
	return false
}

func computeTopEnvStatus(svcName string, svcCfg *config.ServiceConfig, envViews map[string]store.EnvironmentView, envPriority []string, memStore *store.MemoryStore) *TopEnvStatus {
	if len(envViews) < 1 {
		return nil
	}

	// 1. Pick top priority environment
	topEnvName := ""
	for _, p := range envPriority {
		for name := range envViews {
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(p)) {
				topEnvName = name
				break
			}
		}
		if topEnvName != "" {
			break
		}
	}
	if topEnvName == "" {
		for name := range envViews {
			lower := strings.ToLower(name)
			if strings.Contains(lower, "prod") || strings.Contains(lower, "live") || lower == "main" || lower == "release" {
				topEnvName = name
				break
			}
		}
	}
	if topEnvName == "" {
		for name := range envViews {
			topEnvName = name
			break
		}
	}

	topView, ok := envViews[topEnvName]
	if !ok {
		return nil
	}

	// 2. Determine latest version from cached upstream candidates only;
	// never fall back to deployed env versions, which would falsely mark services up to date.
	cachedVersions := memStore.GetVersions(svcName)
	latestVersion := ""
	if len(cachedVersions) > 0 {
		latestVersion = cachedVersions[0]
	}

	versionsBehind := 0
	isUpToDate := false

	activeVer := topView.ActiveVersion
	if activeVer != "" && activeVer != "unknown" && latestVersion != "" {
		if areVersionsEqual(activeVer, latestVersion) {
			isUpToDate = true
			versionsBehind = 0
		} else if len(cachedVersions) > 0 {
			foundIdx := -1
			for idx, v := range cachedVersions {
				if areVersionsEqual(v, activeVer) {
					foundIdx = idx
					break
				}
			}
			if foundIdx >= 0 {
				versionsBehind = foundIdx
				isUpToDate = (foundIdx == 0)
			} else {
				versionsBehind = len(cachedVersions)
			}
		}
	}

	return &TopEnvStatus{
		Environment:    topEnvName,
		ActiveVersion:  activeVer,
		LatestVersion:  latestVersion,
		VersionsBehind: versionsBehind,
		IsUpToDate:     isUpToDate,
		PendingPR:      topView.PendingPR,
	}
}

func (s *Server) handleServiceSubroutes(w http.ResponseWriter, r *http.Request) {
	// Route format: /api/services/:service/(versions|history|diff)
	path := strings.TrimPrefix(r.URL.Path, "/api/services/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	serviceName := parts[0]
	action := parts[1]

	svcCfg, exists := s.cfg.Services[serviceName]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("service '%s' not found", serviceName)})
		return
	}

	if action == "deployment" && len(parts) >= 3 {
		if parts[2] == "status" {
			s.handleServiceStatus(w, r, serviceName, svcCfg)
			return
		}
		if parts[2] == "watch" {
			s.handleServiceWatch(w, r, serviceName, svcCfg)
			return
		}
	}

	switch action {
	case "versions":
		s.handleVersions(w, r, serviceName, svcCfg)
	case "history":
		s.handleHistory(w, r, serviceName)
	case "diff":
		s.handleDiff(w, r, serviceName, svcCfg)
	case "status", "deployment-status":
		s.handleServiceStatus(w, r, serviceName, svcCfg)
	case "watch", "deployment-watch":
		s.handleServiceWatch(w, r, serviceName, svcCfg)
	case "prs", "release-prs":
		s.handleReleasePRs(w, r, serviceName, svcCfg)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request, serviceName string, svcCfg *config.ServiceConfig) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authMgr.GetUserFromDeploymentRequest(r)
	_ = user

	envQuery := r.URL.Query().Get("env")
	statuses := make(map[string]*cluster.Status)

	for envName, envCfg := range svcCfg.Environments {
		if envQuery != "" && envName != envQuery {
			continue
		}
		if s.clusterChecker != nil && s.clusterChecker.IsAvailable() {
			activeVer := ""
			if activeEvt := s.store.GetActiveEvent(serviceName, envName); activeEvt != nil {
				activeVer = activeEvt.Version
			}
			statuses[envName] = s.resolveClusterStatus(r.Context(), serviceName, envName, activeVer, svcCfg, envCfg)
		}
	}

	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) watchDeployment(ctx context.Context, serviceName, envName, targetVer string, svcCfg *config.ServiceConfig, envCfg *config.EnvironmentConfig, timeoutSec int, pollInterval time.Duration, onTick func(*cluster.Status)) (*cluster.Status, error) {
	watchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var lastStatus *cluster.Status

	for {
		st := s.resolveClusterStatus(watchCtx, serviceName, envName, targetVer, svcCfg, envCfg)
		if st != nil {
			lastStatus = st
			if onTick != nil {
				onTick(st)
			}
			if st.Status == "ready" {
				return st, nil
			}
			if st.Status == "degraded" || st.Status == "error" {
				return st, fmt.Errorf("deployment %s: %s", st.Status, st.Message)
			}
		}

		select {
		case <-watchCtx.Done():
			if lastStatus != nil {
				return lastStatus, fmt.Errorf("timed out waiting for deployment to reach ready state (status: %s, message: %s)", lastStatus.Status, lastStatus.Message)
			}
			return nil, fmt.Errorf("timed out waiting for deployment to reach ready state")
		case <-ticker.C:
		}
	}
}

func (s *Server) handleServiceWatch(w http.ResponseWriter, r *http.Request, serviceName string, svcCfg *config.ServiceConfig) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authMgr.GetUserFromDeploymentRequest(r)

	envName := r.URL.Query().Get("env")
	if envName == "" {
		envName = r.URL.Query().Get("environment")
	}
	if envName == "" {
		if len(svcCfg.Environments) == 1 {
			for name := range svcCfg.Environments {
				envName = name
				break
			}
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "env query parameter is required"})
			return
		}
	}

	envCfg, exists := svcCfg.Environments[envName]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("environment '%s' not found for service '%s'", envName, serviceName)})
		return
	}

	if allowed, reason := auth.EvaluatePermission(user, svcCfg, envCfg); !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden: " + reason})
		return
	}

	targetVer := r.URL.Query().Get("version")
	if targetVer == "" {
		targetVer = r.URL.Query().Get("target_version")
	}
	if targetVer == "" {
		if activeEvt := s.store.GetActiveEvent(serviceName, envName); activeEvt != nil {
			targetVer = activeEvt.Version
		}
	}

	timeoutSec := 300
	if tStr := r.URL.Query().Get("timeout"); tStr != "" {
		if val, err := strconv.Atoi(tStr); err == nil && val > 0 {
			timeoutSec = val
		}
	}
	if timeoutSec > 900 {
		timeoutSec = 900
	}

	pollInterval := 2 * time.Second
	if pStr := r.URL.Query().Get("poll_interval"); pStr != "" {
		if val, err := strconv.Atoi(pStr); err == nil && val > 0 {
			pollInterval = time.Duration(val) * time.Second
		}
	}

	isStream := r.URL.Query().Get("stream") == "true" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")

	if s.clusterChecker == nil || !s.clusterChecker.IsAvailable() {
		writeJSON(w, http.StatusNotImplemented, map[string]interface{}{
			"status":      "unsupported",
			"service":     serviceName,
			"environment": envName,
			"message":     "cluster status checker is not enabled or available",
		})
		return
	}

	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		var flusher http.Flusher
		if f, ok := w.(http.Flusher); ok {
			flusher = f
		}

		_, err := s.watchDeployment(r.Context(), serviceName, envName, targetVer, svcCfg, envCfg, timeoutSec, pollInterval, func(st *cluster.Status) {
			payload, _ := json.Marshal(map[string]interface{}{
				"service":          serviceName,
				"environment":      envName,
				"target_version":   targetVer,
				"status":           st.Status,
				"ready_replicas":   st.ReadyReplicas,
				"updated_replicas": st.UpdatedReplicas,
				"desired_replicas": st.DesiredReplicas,
				"current_version":  st.CurrentVersion,
				"message":          st.Message,
			})
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", st.Status, string(payload))
			if flusher != nil {
				flusher.Flush()
			}
		})
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
			if flusher != nil {
				flusher.Flush()
			}
		}
		return
	}

	finalStatus, err := s.watchDeployment(r.Context(), serviceName, envName, targetVer, svcCfg, envCfg, timeoutSec, pollInterval, nil)
	if err != nil {
		statusHTTP := http.StatusExpectationFailed
		if strings.Contains(err.Error(), "timed out") {
			statusHTTP = http.StatusGatewayTimeout
		}
		writeJSON(w, statusHTTP, map[string]interface{}{
			"status":         "failed",
			"service":        serviceName,
			"environment":    envName,
			"target_version": targetVer,
			"deployment":     finalStatus,
			"error":          err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ready",
		"service":        serviceName,
		"environment":    envName,
		"target_version": targetVer,
		"deployment":     finalStatus,
	})
}

func (s *Server) resolveClusterStatus(ctx context.Context, svcName, envName, expectedVersion string, svcCfg *config.ServiceConfig, envCfg *config.EnvironmentConfig) *cluster.Status {
	if s.clusterChecker == nil || !s.clusterChecker.IsAvailable() {
		return nil
	}

	//guard against explicitly disabled checks
	if envCfg.StatusCheck != nil && envCfg.StatusCheck.Enabled != nil && !*envCfg.StatusCheck.Enabled {
		return nil
	}
	if svcCfg.StatusCheck != nil && svcCfg.StatusCheck.Enabled != nil && !*svcCfg.StatusCheck.Enabled {
		return nil
	}

	deployment := ""
	namespace := ""

	if envCfg.StatusCheck != nil {
		deployment = envCfg.StatusCheck.Deployment
		namespace = envCfg.StatusCheck.Namespace
	}
	if deployment == "" && svcCfg.StatusCheck != nil {
		deployment = svcCfg.StatusCheck.Deployment
	}
	if namespace == "" && svcCfg.StatusCheck != nil {
		namespace = svcCfg.StatusCheck.Namespace
	}

	//sniff target file from git bare clone if available
	if (deployment == "" || namespace == "") && envCfg.Target.File != "" && s.indexer != nil {
		repoURL := envCfg.Repo
		if repoURL == "" {
			repoURL = s.cfg.GitOps.Repo
		}
		repoDir := s.indexer.GetRepoPath(repoURL)
		if data, err := git.ShowFileAtCommit(ctx, repoDir, envCfg.Branch, envCfg.Target.File); err == nil {
			if dName, dNs, ok := cluster.SniffDeploymentManifest(data); ok {
				if deployment == "" {
					deployment = dName
				}
				if namespace == "" && dNs != "" {
					namespace = dNs
				}
			}
		}
	}

	if deployment == "" {
		deployment = svcName
	}
	if namespace == "" {
		namespace = s.clusterChecker.DefaultNamespace()
	}

	st, err := s.clusterChecker.GetDeploymentStatus(ctx, namespace, deployment, expectedVersion)
	if err != nil {
		log.Printf("[cluster] error getting deployment status for %s/%s: %v", namespace, deployment, err)
		return nil
	}
	return st
}

func (s *Server) handleVersions(w http.ResponseWriter, r *http.Request, serviceName string, svcCfg *config.ServiceConfig) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	prov := provider.NewProvider(svcCfg.Source)
	versions, err := prov.GetVersions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if versions == nil {
		versions = []provider.VersionInfo{}
	} else if len(versions) > 0 {
		s.enrichVersionsFromContext(r.Context(), versions, svcCfg)
		versions = provider.DeduplicateVersionsByDigest(versions)
		verStrs := make([]string, 0, len(versions))
		for _, v := range versions {
			verStrs = append(verStrs, v.Version)
		}
		s.store.SetVersions(serviceName, verStrs)
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *Server) enrichVersionsFromContext(ctx context.Context, versions []provider.VersionInfo, svcCfg *config.ServiceConfig) {
	if svcCfg == nil || strings.TrimSpace(svcCfg.Context.Repo) == "" {
		return
	}

	contextRepo := strings.TrimSpace(svcCfg.Context.Repo)
	tagPrefix := svcCfg.Context.GetTagPrefix()

	s.contextVersionsMu.Lock()
	ctxVersions, found := s.contextVersionsCache[contextRepo]
	s.contextVersionsMu.Unlock()

	if !found {
		//query releases from github if context is a github repository
		if !strings.Contains(contextRepo, "gitlab") {
			ctxProv := provider.NewProvider(config.SourceConfig{
				Type:       "github_releases",
				Repository: contextRepo,
				TrimPrefix: tagPrefix,
			})
			fetched, err := ctxProv.GetVersions(ctx)
			if err == nil && len(fetched) > 0 {
				ctxVersions = fetched
				s.contextVersionsMu.Lock()
				s.contextVersionsCache[contextRepo] = ctxVersions
				s.contextVersionsMu.Unlock()
			}
		}
	}

	if len(ctxVersions) > 0 {
		relMap := make(map[string]provider.VersionInfo, len(ctxVersions)*2)
		for _, cv := range ctxVersions {
			relMap[cv.Version] = cv
			relMap[strings.TrimPrefix(cv.Version, "v")] = cv
			if tagPrefix != "" {
				relMap[strings.TrimPrefix(cv.Version, tagPrefix)] = cv
			}
		}

		for i := range versions {
			v := &versions[i]
			cleanV := strings.TrimPrefix(v.Version, "v")
			match, ok := relMap[v.Version]
			if !ok {
				match, ok = relMap[cleanV]
			}
			if !ok && tagPrefix != "" {
				match, ok = relMap[tagPrefix+v.Version]
			}
			if ok {
				if v.Description == "" {
					v.Description = match.Description
				}
				if v.Changelog == "" {
					v.Changelog = match.Changelog
				}
				if v.ReleasedAt == nil {
					v.ReleasedAt = match.ReleasedAt
				}
				v.Prerelease = v.Prerelease || match.Prerelease
			}
		}
	}
}

func (s *Server) handleReleasePRs(w http.ResponseWriter, r *http.Request, serviceName string, svcCfg *config.ServiceConfig) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.releasePRsMu.Lock()
	if cached, ok := s.releasePRsCache[serviceName]; ok {
		s.releasePRsMu.Unlock()
		writeJSON(w, http.StatusOK, cached)
		return
	}
	s.releasePRsMu.Unlock()

	prov := provider.NewProvider(svcCfg.Source)
	versions, err := prov.GetVersions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string][]int{})
		return
	}
	if len(versions) > 0 {
		s.enrichVersionsFromContext(r.Context(), versions, svcCfg)
		versions = provider.DeduplicateVersionsByDigest(versions)
	}

	result := make(map[string][]int)
	repoURL := strings.TrimSpace(svcCfg.Context.Repo)
	var repoDir string
	if repoURL != "" {
		repoDir = s.indexer.GetRepoPath(repoURL)
	}

	// Limit to top 25 versions to keep processing quick
	maxVers := len(versions)
	if maxVers > 25 {
		maxVers = 25
	}

	for i := 0; i < maxVers; i++ {
		v := versions[i]
		prs := git.ExtractPRsFromText(v.Changelog)
		if len(prs) == 0 && repoDir != "" && i+1 < len(versions) {
			diff, err := git.GetVersionDiff(r.Context(), repoDir, versions[i+1].Version, v.Version, svcCfg.Context.GetTagPrefix())
			if err == nil && diff != nil {
				seen := make(map[int]bool)
				for _, c := range diff.Commits {
					if c.PRNumber > 0 && !seen[c.PRNumber] {
						seen[c.PRNumber] = true
						prs = append(prs, c.PRNumber)
					}
				}
				sort.Sort(sort.Reverse(sort.IntSlice(prs)))
			}
		}
		if len(prs) > 0 {
			result[v.Version] = prs
		}
	}

	s.releasePRsMu.Lock()
	if s.releasePRsCache == nil {
		s.releasePRsCache = make(map[string]map[string][]int)
	}
	s.releasePRsCache[serviceName] = result
	s.releasePRsMu.Unlock()

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request, serviceName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	history := s.store.GetHistory(serviceName)
	if history == nil {
		history = make(map[string][]store.DeploymentEvent)
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request, serviceName string, svcCfg *config.ServiceConfig) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fromRef := r.URL.Query().Get("from")
	toRef := r.URL.Query().Get("to")

	cleanFrom := git.CleanVersionRef(fromRef)
	cleanTo := git.CleanVersionRef(toRef)
	if cleanFrom != "" && (cleanFrom == cleanTo || strings.TrimPrefix(cleanFrom, "v") == strings.TrimPrefix(cleanTo, "v")) {
		writeJSON(w, http.StatusOK, &git.ChangelogDiff{
			FromVersion: fromRef,
			ToVersion:   toRef,
			Commits:     []git.DiffEntry{},
			DiffStat:    "0 files changed",
		})
		return
	}

	repoURL := strings.TrimSpace(svcCfg.Context.Repo)
	if repoURL == "" {
		writeJSON(w, http.StatusOK, &git.ChangelogDiff{
			FromVersion: fromRef,
			ToVersion:   toRef,
			Commits:     []git.DiffEntry{},
			DiffStat:    "0 files changed",
		})
		return
	}

	cloneURL := repoURL
	if !strings.HasPrefix(cloneURL, "https://") && !strings.HasPrefix(cloneURL, "http://") && !strings.HasPrefix(cloneURL, "git@") {
		cloneURL = "https://github.com/" + strings.TrimSuffix(cloneURL, ".git") + ".git"
	}

	repoDir := s.indexer.GetRepoPath(repoURL)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		_ = git.CloneOrFetch(r.Context(), cloneURL, "", repoDir, true)
	}

	diff, err := git.GetVersionDiff(r.Context(), repoDir, fromRef, toRef, svcCfg.Context.GetTagPrefix())
	if err != nil || diff == nil {
		diff = &git.ChangelogDiff{
			FromVersion: fromRef,
			ToVersion:   toRef,
			Commits:     []git.DiffEntry{},
			DiffStat:    "0 files changed",
		}
	}

	writeJSON(w, http.StatusOK, diff)
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req deployer.PromotionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	if req.Service == "" || req.Environment == "" || req.TargetVersion == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service, environment, and target_version are required"})
		return
	}

	user := s.authMgr.GetUserFromDeploymentRequest(r)

	// Permission enforcement
	svcCfg, exists := s.cfg.Services[req.Service]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("service '%s' not found", req.Service)})
		return
	}
	envCfg := svcCfg.Environments[req.Environment]

	allowed, reason := auth.EvaluatePermission(user, svcCfg, envCfg)
	if !allowed {
		log.Printf("[promote] permission denied for user %v on %s/%s: %s", user, req.Service, req.Environment, reason)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden: " + reason})
		return
	}

	if user != nil && req.User == "" {
		if user.Name != "" {
			req.User = user.Name
		} else {
			req.User = user.Username
		}
	} else if userHeader := r.Header.Get("X-Forwarded-User"); userHeader != "" && req.User == "" {
		req.User = userHeader
	}
	if user != nil {
		if req.Email == "" && user.Email != "" {
			req.Email = user.Email
		}
		if req.GitHubToken == "" && user.Token != "" {
			req.GitHubToken = user.Token
		}
	}

	// Dispatch deployment_requested event
	s.dispatcher.Dispatch(webhook.EventDeploymentRequested, webhook.DeploymentRequestedEvent{
		Event:         webhook.EventDeploymentRequested,
		Service:       req.Service,
		Environment:   req.Environment,
		TargetVersion: req.TargetVersion,
		User:          req.User,
		Timestamp:     time.Now(),
	}, req.Service)

	result, err := s.deployer.Promote(r.Context(), req)
	if err != nil {
		log.Printf("[promote] error promoting %s to %s (%s): %v", req.Service, req.Environment, req.TargetVersion, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// If promotion was via pull request, record pending PR in store
	if result.Mode == "pull_request" {
		s.store.SetPendingPR(req.Service, req.Environment, &store.PendingPR{
			Service:       req.Service,
			Environment:   req.Environment,
			TargetVersion: req.TargetVersion,
			Branch:        result.Branch,
			PRNumber:      result.PRNumber,
			PRURL:         result.PRURL,
			User:          req.User,
			CreatedAt:     time.Now(),
		})
	}

	// Dispatch deployment_completed event
	s.dispatcher.Dispatch(webhook.EventDeploymentCompleted, webhook.DeploymentCompletedEvent{
		Event:         webhook.EventDeploymentCompleted,
		Service:       req.Service,
		Environment:   req.Environment,
		TargetVersion: req.TargetVersion,
		User:          req.User,
		CommitSHA:     result.CommitSHA,
		PRNumber:      result.PRNumber,
		PRURL:         result.PRURL,
		Timestamp:     time.Now(),
	}, req.Service)

	// Programmatic watch support for CI/CD callers
	if (r.URL.Query().Get("watch") == "true" || r.URL.Query().Get("wait") == "true") && result.Mode == "direct" && s.clusterChecker != nil && s.clusterChecker.IsAvailable() {
		timeoutSec := 300
		if tStr := r.URL.Query().Get("timeout"); tStr != "" {
			if val, err := strconv.Atoi(tStr); err == nil && val > 0 {
				timeoutSec = val
			}
		}
		if timeoutSec > 900 {
			timeoutSec = 900
		}

		finalStatus, err := s.watchDeployment(r.Context(), req.Service, req.Environment, req.TargetVersion, svcCfg, envCfg, timeoutSec, 2*time.Second, nil)
		if err != nil {
			statusHTTP := http.StatusExpectationFailed
			if strings.Contains(err.Error(), "timed out") {
				statusHTTP = http.StatusGatewayTimeout
			}
			writeJSON(w, statusHTTP, map[string]interface{}{
				"status":     "failed",
				"promotion":  result,
				"deployment": finalStatus,
				"error":      err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "ready",
			"promotion":  result,
			"deployment": finalStatus,
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body: " + err.Error()})
		return
	}

	ghEvent := r.Header.Get("X-GitHub-Event")

	// 1. GitHub release event
	if ghEvent == "release" {
		rel, err := webhook.ParseReleaseRequest(r, body)
		if err == nil && rel.Service != "" {
			s.dispatcher.Dispatch(webhook.EventNewVersionAvailable, webhook.NewVersionAvailableEvent{
				Event:       webhook.EventNewVersionAvailable,
				Service:     rel.Service,
				Version:     rel.Version,
				Description: rel.Description,
				Changelog:   rel.Changelog,
				Timestamp:   time.Now(),
			}, rel.Service)

			go func() {
				log.Printf("[webhook] New release %s for service %s processed", rel.Version, rel.Service)
				_ = s.indexer.ReindexAll(context.Background())
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "ok",
				"message": fmt.Sprintf("release %s for service %s accepted", rel.Version, rel.Service),
				"service": rel.Service,
				"version": rel.Version,
			})
			return
		}
	}

	// 2. GitHub PR event
	if ghEvent == "pull_request" {
		pr, err := webhook.ParsePRMergedRequest(r, body)
		if err == nil {
			s.clearPendingPRFromWebhook(pr)
			s.dispatcher.Dispatch(webhook.EventPRMerged, webhook.PRMergedEvent{
				Event:       webhook.EventPRMerged,
				Service:     pr.Service,
				Environment: pr.Environment,
				PRNumber:    pr.PRNumber,
				CommitSHA:   pr.CommitSHA,
				Branch:      pr.Branch,
				MergedBy:    pr.MergedBy,
				Timestamp:   time.Now(),
			}, pr.Service)

			go func() {
				log.Printf("[webhook] PR #%d closed or merged (service: %s, env: %s), triggering reindex", pr.PRNumber, pr.Service, pr.Environment)
				_ = s.indexer.ReindexAll(context.Background())
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":    "ok",
				"message":   fmt.Sprintf("pr #%d event accepted", pr.PRNumber),
				"pr_number": pr.PRNumber,
			})
			return
		}
	}

	// 3. Inspect JSON body for release or PR events
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err == nil {
		if raw["event"] == "release" || (raw["service"] != nil && raw["version"] != nil && raw["target_version"] == nil) {
			rel, err := webhook.ParseReleaseRequest(r, body)
			if err == nil && rel.Service != "" {
				s.dispatcher.Dispatch(webhook.EventNewVersionAvailable, webhook.NewVersionAvailableEvent{
					Event:       webhook.EventNewVersionAvailable,
					Service:     rel.Service,
					Version:     rel.Version,
					Description: rel.Description,
					Changelog:   rel.Changelog,
					Timestamp:   time.Now(),
				}, rel.Service)

				go func() {
					log.Printf("[webhook] New release %s for service %s processed", rel.Version, rel.Service)
					_ = s.indexer.ReindexAll(context.Background())
				}()

				writeJSON(w, http.StatusOK, map[string]interface{}{
					"status":  "ok",
					"message": fmt.Sprintf("release %s for service %s accepted", rel.Version, rel.Service),
					"service": rel.Service,
					"version": rel.Version,
				})
				return
			}
		}

		if raw["event"] == "pr_merged" || raw["pull_request"] != nil {
			pr, err := webhook.ParsePRMergedRequest(r, body)
			if err == nil {
				s.clearPendingPRFromWebhook(pr)
				s.dispatcher.Dispatch(webhook.EventPRMerged, webhook.PRMergedEvent{
					Event:       webhook.EventPRMerged,
					Service:     pr.Service,
					Environment: pr.Environment,
					PRNumber:    pr.PRNumber,
					CommitSHA:   pr.CommitSHA,
					Branch:      pr.Branch,
					MergedBy:    pr.MergedBy,
					Timestamp:   time.Now(),
				}, pr.Service)

				go func() {
					log.Printf("[webhook] PR #%d closed or merged (service: %s, env: %s), triggering reindex", pr.PRNumber, pr.Service, pr.Environment)
					_ = s.indexer.ReindexAll(context.Background())
				}()

				writeJSON(w, http.StatusOK, map[string]interface{}{
					"status":    "ok",
					"message":   fmt.Sprintf("pr #%d event accepted", pr.PRNumber),
					"pr_number": pr.PRNumber,
				})
				return
			}
		}
	}

	// 4. Fallback Git push event: reindex all services
	go func() {
		log.Println("[webhook] Received Git push event, reindexing all services...")
		_ = s.indexer.ReindexAll(context.Background())
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "re-index triggered",
	})
}

func (s *Server) handleWebhookRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body: " + err.Error()})
		return
	}

	rel, err := webhook.ParseReleaseRequest(r, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.dispatcher.Dispatch(webhook.EventNewVersionAvailable, webhook.NewVersionAvailableEvent{
		Event:       webhook.EventNewVersionAvailable,
		Service:     rel.Service,
		Version:     rel.Version,
		Description: rel.Description,
		Changelog:   rel.Changelog,
		Timestamp:   time.Now(),
	}, rel.Service)

	go func() {
		log.Printf("[webhook] New release %s for service %s processed", rel.Version, rel.Service)
		_ = s.indexer.ReindexAll(context.Background())
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("release %s for service %s accepted", rel.Version, rel.Service),
		"service": rel.Service,
		"version": rel.Version,
	})
}

func (s *Server) handleWebhookPR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body: " + err.Error()})
		return
	}

	if pr, err := webhook.ParsePRMergedRequest(r, body); err == nil {
		s.clearPendingPRFromWebhook(pr)

		s.dispatcher.Dispatch(webhook.EventPRMerged, webhook.PRMergedEvent{
			Event:       webhook.EventPRMerged,
			Service:     pr.Service,
			Environment: pr.Environment,
			PRNumber:    pr.PRNumber,
			CommitSHA:   pr.CommitSHA,
			Branch:      pr.Branch,
			MergedBy:    pr.MergedBy,
			Timestamp:   time.Now(),
		}, pr.Service)

		go func() {
			log.Printf("[webhook] PR #%d closed or merged for service %s, triggering reindex", pr.PRNumber, pr.Service)
			_ = s.indexer.ReindexAll(context.Background())
		}()

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":    "ok",
			"message":   fmt.Sprintf("pr #%d event accepted", pr.PRNumber),
			"pr_number": pr.PRNumber,
			"service":   pr.Service,
		})
		return
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
}

func (s *Server) clearPendingPRFromWebhook(pr *webhook.InboundPRMergedPayload) {
	if pr == nil {
		return
	}
	clearedSvc, clearedEnv := "", ""
	if pr.PRNumber > 0 {
		clearedSvc, clearedEnv = s.store.ClearPendingPRByNumber(pr.PRNumber)
	}
	if clearedSvc == "" && pr.Branch != "" {
		clearedSvc, clearedEnv = s.store.ClearPendingPRByBranch(pr.Branch)
	}
	if clearedSvc == "" && pr.Service != "" && pr.Environment != "" {
		s.store.ClearPendingPR(pr.Service, pr.Environment)
		clearedSvc = pr.Service
		clearedEnv = pr.Environment
	}
	if clearedSvc != "" {
		pr.Service = clearedSvc
	}
	if clearedEnv != "" {
		pr.Environment = clearedEnv
	}
}

func isPRClosed(ctx context.Context, repoURL string, prNumber int) bool {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || prNumber <= 0 || repoURL == "" {
		return false
	}
	cleanRepo := strings.TrimPrefix(repoURL, "git@github.com:")
	cleanRepo = strings.TrimPrefix(cleanRepo, "https://github.com/")
	cleanRepo = strings.TrimSuffix(cleanRepo, ".git")

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", cleanRepo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return true
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var ghPR struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghPR); err == nil {
		return ghPR.State == "closed"
	}
	return false
}

func (s *Server) handleWebhooksList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": s.cfg.Webhooks,
		"history":    s.dispatcher.GetHistory(),
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
