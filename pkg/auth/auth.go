package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/config"
)

// User represents an authenticated user in DeployStackr.
type User struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Name        string            `json:"name"`
	Email       string            `json:"email"`
	AvatarURL   string            `json:"avatar_url"`
	Provider    string            `json:"provider"` // "github", "google", "dev"
	Teams       []string          `json:"teams"`    // GitHub teams, e.g. ["org/devops", "org/backend"]
	Groups      []string          `json:"groups"`   // Google groups, e.g. ["devs@example.com"]
	Repos       map[string]string `json:"repos"`    // GitHub repo permissions: repo -> "admin" | "write" | "read"
	DeployRepos []string          `json:"deploy_repos,omitempty"` // Repositories written to for deployments
	Token       string            `json:"-"`        // GitHub token used for PR creation & API operations
}

type tokenCacheEntry struct {
	user      *User
	expiresAt time.Time
}

// Provider defines the interface for pluggable authentication providers.
type Provider interface {
	Name() string
	LoginURL(state, redirectURI string) string
	HandleCallback(ctx context.Context, code, redirectURI string) (*User, error)
}

// AuthManager coordinates pluggable authentication and session state.
type AuthManager struct {
	mu              sync.RWMutex
	providers       map[string]Provider
	primaryProvider string
	sessions        map[string]*Session
	tokenCache      map[string]tokenCacheEntry
	secretKey       []byte
}

type Session struct {
	ID        string    `json:"id"`
	User      *User     `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewAuthManager initializes the auth system with enabled providers.
// Only one connected authentication provider is active as the primary auth system.
func NewAuthManager(cfg *config.AuthConfig) *AuthManager {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	am := &AuthManager{
		providers:  make(map[string]Provider),
		sessions:   make(map[string]*Session),
		tokenCache: make(map[string]tokenCacheEntry),
		secretKey:  secret,
	}

	selected := ""
	if cfg != nil {
		selected = strings.ToLower(strings.TrimSpace(cfg.Provider))
	}

	hasGitHub := cfg != nil && cfg.GitHub != nil && cfg.GitHub.ClientID != "" && cfg.GitHub.ClientSecret != ""
	hasGoogle := cfg != nil && cfg.Google != nil && cfg.Google.ClientID != "" && cfg.Google.ClientSecret != ""

	// Determine single primary provider
	var primary string
	switch selected {
	case "github":
		if hasGitHub {
			am.RegisterProvider(NewGitHubProvider(cfg.GitHub.ClientID, cfg.GitHub.ClientSecret))
			primary = "github"
		}
	case "google":
		if hasGoogle {
			am.RegisterProvider(NewGoogleProvider(cfg.Google.ClientID, cfg.Google.ClientSecret))
			primary = "google"
		}
	case "dev":
		am.RegisterProvider(NewDevProvider())
		primary = "dev"
	default:
		// When no provider is explicitly set, choose at most one configured external provider
		if hasGitHub {
			am.RegisterProvider(NewGitHubProvider(cfg.GitHub.ClientID, cfg.GitHub.ClientSecret))
			primary = "github"
		} else if hasGoogle {
			am.RegisterProvider(NewGoogleProvider(cfg.Google.ClientID, cfg.Google.ClientSecret))
			primary = "google"
		}
	}

	// Fallback to dev if no provider was configured
	if primary == "" {
		am.RegisterProvider(NewDevProvider())
		primary = "dev"
	}
	am.primaryProvider = primary

	// In dev mode, ensure dev provider is registered so test users work
	if cfg != nil && cfg.DevMode {
		if _, ok := am.providers["dev"]; !ok {
			am.RegisterProvider(NewDevProvider())
		}
	}

	return am
}

func (am *AuthManager) PrimaryProvider() string {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.primaryProvider
}

func (am *AuthManager) RegisterProvider(p Provider) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.providers[p.Name()] = p
}

func (am *AuthManager) GetProvider(name string) (Provider, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	p, ok := am.providers[name]
	return p, ok
}

func (am *AuthManager) ListProviders() []string {
	am.mu.RLock()
	defer am.mu.RUnlock()
	list := make([]string, 0, len(am.providers))
	for name := range am.providers {
		list = append(list, name)
	}
	return list
}

func (am *AuthManager) CreateSession(u *User) (*Session, string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	b := make([]byte, 24)
	_, _ = rand.Read(b)
	sessionID := hex.EncodeToString(b)

	s := &Session{
		ID:        sessionID,
		User:      u,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	am.sessions[sessionID] = s

	// Sign session ID for tamper-evident cookie
	mac := hmac.New(sha256.New, am.secretKey)
	mac.Write([]byte(sessionID))
	signed := fmt.Sprintf("%s.%s", sessionID, hex.EncodeToString(mac.Sum(nil)))

	return s, signed
}

func (am *AuthManager) GetUserFromCookie(r *http.Request) *User {
	cookie, err := r.Cookie("deploystackr_session")
	if err != nil || cookie.Value == "" {
		return nil
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return nil
	}
	sessionID, signature := parts[0], parts[1]

	mac := hmac.New(sha256.New, am.secretKey)
	mac.Write([]byte(sessionID))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil
	}

	am.mu.RLock()
	defer am.mu.RUnlock()

	s, exists := am.sessions[sessionID]
	if !exists || time.Now().After(s.ExpiresAt) {
		return nil
	}

	return s.User
}

func (am *AuthManager) DeleteSession(r *http.Request) {
	cookie, err := r.Cookie("deploystackr_session")
	if err != nil || cookie.Value == "" {
		return
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) > 0 {
		am.mu.Lock()
		delete(am.sessions, parts[0])
		am.mu.Unlock()
	}
}

// GetUserFromSession resolves authenticated user from session cookie or forwarded header (for web UI).
func (am *AuthManager) GetUserFromSession(r *http.Request) *User {
	if u := am.GetUserFromCookie(r); u != nil {
		return u
	}
	if fwd := r.Header.Get("X-Forwarded-User"); fwd != "" {
		return &User{
			ID:       "forwarded:" + fwd,
			Username: fwd,
			Name:     fwd,
			Provider: "header",
		}
	}
	return nil
}

// GetUserFromDeploymentRequest resolves authenticated user for programmatic deployment and status watch endpoints.
// It prioritizes CI/programmatic GitHub tokens in headers before falling back to browser sessions.
func (am *AuthManager) GetUserFromDeploymentRequest(r *http.Request) *User {
	token := ExtractTokenFromHeader(r)
	if(token != "") {
		am.mu.RLock()
		if entry, ok := am.tokenCache[token]; ok && time.Now().Before(entry.expiresAt) {
			am.mu.RUnlock()
			return entry.user
		}
		am.mu.RUnlock()

		u, err := FetchGitHubUser(r.Context(), token)
		if(err == nil && u != nil) {
			am.mu.Lock()
			am.tokenCache[token] = tokenCacheEntry{
				user:      u,
				expiresAt: time.Now().Add(5 * time.Minute),
			}
			am.mu.Unlock()
			return u
		}
	}

	return am.GetUserFromSession(r)
}

func (am *AuthManager) CacheUserToken(token string, user *User, duration time.Duration) {
	if(am == nil || token == "" || user == nil) {
		return
	}
	am.mu.Lock()
	defer am.mu.Unlock()
	am.tokenCache[token] = tokenCacheEntry{
		user:      user,
		expiresAt: time.Now().Add(duration),
	}
}

// GetUserFromRequest resolves authenticated user from session cookie or forwarded header.
func (am *AuthManager) GetUserFromRequest(r *http.Request) *User {
	return am.GetUserFromSession(r)
}

// ExtractTokenFromHeader inspects Authorization and X-GitHub-Token headers.
func ExtractTokenFromHeader(r *http.Request) string {
	if authHdr := r.Header.Get("Authorization"); authHdr != "" {
		parts := strings.Fields(authHdr)
		if len(parts) == 2 && (strings.EqualFold(parts[0], "bearer") || strings.EqualFold(parts[0], "token")) {
			return parts[1]
		}
	}
	if tok := r.Header.Get("X-GitHub-Token"); tok != "" {
		return strings.TrimSpace(tok)
	}
	return ""
}

// FetchGitHubUser fetches user profile, teams, and repo permissions using a GitHub token.
func FetchGitHubUser(ctx context.Context, token string) (*User, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+token)
	userReq.Header.Set("Accept", "application/vnd.github.v3+json")

	userResp, err := client.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("github user request failed: %w", err)
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned HTTP %d", userResp.StatusCode)
	}

	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decode github user: %w", err)
	}

	teams := fetchGitHubTeams(ctx, client, token)
	repos := fetchGitHubRepos(ctx, client, token)

	email := ghUser.Email
	if email == "" {
		email = fmt.Sprintf("%s@users.noreply.github.com", ghUser.Login)
	}
	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	return &User{
		ID:        fmt.Sprintf("github:%d", ghUser.ID),
		Username:  ghUser.Login,
		Name:      name,
		Email:     email,
		AvatarURL: ghUser.AvatarURL,
		Provider:  "github",
		Teams:     teams,
		Groups:    []string{},
		Repos:     repos,
		Token:     token,
	}, nil
}

func fetchGitHubTeams(ctx context.Context, client *http.Client, token string) []string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/teams?per_page=100", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return []string{}
	}
	defer resp.Body.Close()

	var teamsList []struct {
		Slug         string `json:"slug"`
		Organization struct {
			Login string `json:"login"`
		} `json:"organization"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&teamsList); err != nil {
		return []string{}
	}

	teams := make([]string, 0, len(teamsList))
	for _, t := range teamsList {
		teams = append(teams, fmt.Sprintf("%s/%s", t.Organization.Login, t.Slug))
	}
	return teams
}

func fetchGitHubRepos(ctx context.Context, client *http.Client, token string) map[string]string {
	repos := make(map[string]string)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/repos?per_page=100", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return repos
	}
	defer resp.Body.Close()

	var repoList []struct {
		FullName    string `json:"full_name"`
		Permissions struct {
			Admin bool `json:"admin"`
			Push  bool `json:"push"`
			Pull  bool `json:"pull"`
		} `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoList); err != nil {
		return repos
	}

	for _, r := range repoList {
		if r.Permissions.Admin {
			repos[r.FullName] = "admin"
		} else if r.Permissions.Push {
			repos[r.FullName] = "write"
		} else if r.Permissions.Pull {
			repos[r.FullName] = "read"
		}
	}
	return repos
}

// -------------------------------------------------------------------------
// GitHub OAuth Provider
// -------------------------------------------------------------------------

type GitHubProvider struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewGitHubProvider(clientID, clientSecret string) *GitHubProvider {
	return &GitHubProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GitHubProvider) Name() string {
	return "github"
}

func (g *GitHubProvider) LoginURL(state, redirectURI string) string {
	vals := url.Values{}
	vals.Set("client_id", g.clientID)
	vals.Set("redirect_uri", redirectURI)
	vals.Set("scope", "read:user user:email read:org repo")
	vals.Set("state", state)
	return "https://github.com/login/oauth/authorize?" + vals.Encode()
}

func (g *GitHubProvider) HandleCallback(ctx context.Context, code, redirectURI string) (*User, error) {
	// Exchange code for access token
	tokenVals := url.Values{}
	tokenVals.Set("client_id", g.clientID)
	tokenVals.Set("client_secret", g.clientSecret)
	tokenVals.Set("code", code)
	tokenVals.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(tokenVals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github token request failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode github token response: %w", err)
	}
	if tokenResp.Error != "" || tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("github oauth error: %s", tokenResp.Error)
	}

	return FetchGitHubUser(ctx, tokenResp.AccessToken)
}

// -------------------------------------------------------------------------
// Google OAuth Provider
// -------------------------------------------------------------------------

type GoogleProvider struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewGoogleProvider(clientID, clientSecret string) *GoogleProvider {
	return &GoogleProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (gp *GoogleProvider) Name() string {
	return "google"
}

func (gp *GoogleProvider) LoginURL(state, redirectURI string) string {
	vals := url.Values{}
	vals.Set("client_id", gp.clientID)
	vals.Set("redirect_uri", redirectURI)
	vals.Set("response_type", "code")
	vals.Set("scope", "openid email profile")
	vals.Set("state", state)
	vals.Set("access_type", "online")
	return "https://accounts.google.com/o/oauth2/v2/auth?" + vals.Encode()
}

func (gp *GoogleProvider) HandleCallback(ctx context.Context, code, redirectURI string) (*User, error) {
	tokenVals := url.Values{}
	tokenVals.Set("client_id", gp.clientID)
	tokenVals.Set("client_secret", gp.clientSecret)
	tokenVals.Set("code", code)
	tokenVals.Set("redirect_uri", redirectURI)
	tokenVals.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(tokenVals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := gp.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google token request failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode google token response: %w", err)
	}
	if tokenResp.Error != "" || tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("google oauth error: %s", tokenResp.Error)
	}

	// Fetch Google User Profile
	userReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userResp, err := gp.httpClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("google userinfo request failed: %w", err)
	}
	defer userResp.Body.Close()

	var gUser struct {
		Sub     string `json:"sub"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
		HD      string `json:"hd"` // Hosted domain if Google Workspace
	}
	if err := json.NewDecoder(userResp.Body).Decode(&gUser); err != nil {
		return nil, fmt.Errorf("decode google userinfo: %w", err)
	}

	// Extract groups: domain-level group membership + standard mappings
	groups := make([]string, 0)
	if gUser.HD != "" {
		groups = append(groups, fmt.Sprintf("all@%s", gUser.HD))
	}
	if gUser.Email != "" {
		// Include email domain as fallback group
		parts := strings.Split(gUser.Email, "@")
		if len(parts) == 2 {
			groups = append(groups, fmt.Sprintf("team@%s", parts[1]))
		}
	}

	username := strings.Split(gUser.Email, "@")[0]

	return &User{
		ID:        fmt.Sprintf("google:%s", gUser.Sub),
		Username:  username,
		Name:      gUser.Name,
		Email:     gUser.Email,
		AvatarURL: gUser.Picture,
		Provider:  "google",
		Teams:     []string{},
		Groups:    groups,
		Repos:     make(map[string]string),
	}, nil
}

// -------------------------------------------------------------------------
// Dev / Mock Provider (For instant local dev testing & demonstration)
// -------------------------------------------------------------------------

type DevProvider struct{}

func NewDevProvider() *DevProvider {
	return &DevProvider{}
}

func (d *DevProvider) Name() string {
	return "dev"
}

func (d *DevProvider) LoginURL(state, redirectURI string) string {
	// Dev provider returns direct callback redirect
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	code := state
	if code == "" || code == "state" {
		code = "dev_admin"
	}
	q.Set("code", code)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

func (d *DevProvider) HandleCallback(ctx context.Context, code, redirectURI string) (*User, error) {
	persona := code
	switch persona {
	case "dev_billing":
		return &User{
			ID:        "dev:billing",
			Username:  "sarah-chen",
			Name:      "Sarah Chen",
			Email:     "sarah.chen@example.com",
			AvatarURL: "",
			Provider:  "dev",
			Teams:     []string{"org/billing-team", "org/engineering"},
			Groups:    []string{"billing-engineers@example.com", "engineers@example.com"},
			Repos: map[string]string{
				"org/billing-service": "admin",
				"org/auth-service":    "read",
			},
		}, nil
	case "dev_guest":
		return &User{
			ID:        "dev:guest",
			Username:  "guest-user",
			Name:      "Guest Developer",
			Email:     "guest@example.com",
			AvatarURL: "",
			Provider:  "dev",
			Teams:     []string{},
			Groups:    []string{"viewers@example.com"},
			Repos: map[string]string{
				"org/billing-service": "read",
			},
		}, nil
	default:
		// Full access Admin Persona
		return &User{
			ID:        "dev:admin",
			Username:  "alex-rivera",
			Name:      "Alex Rivera",
			Email:     "alex.rivera@example.com",
			AvatarURL: "",
			Provider:  "dev",
			Teams:     []string{"org/platform-team", "org/devops", "org/billing-team", "org/auth-team"},
			Groups:    []string{"platform-engineers@example.com", "devops@example.com", "engineers@example.com"},
			Repos: map[string]string{
				"org/billing-service":      "admin",
				"org/auth-service":         "admin",
				"org/notification-service": "admin",
			},
		}, nil
	}
}

// -------------------------------------------------------------------------
// Permissions & Authorization Evaluator
// -------------------------------------------------------------------------

// EvaluatePermission checks if the user is authorized to perform deployments for a service & env.
// Returns (allowed, reason).
func EvaluatePermission(u *User, svcCfg *config.ServiceConfig, envCfg *config.EnvironmentConfig) (bool, string) {
	if u == nil {
		return false, "Authentication required"
	}

	// 1. Determine effective permissions: Environment overrides service-level permissions if defined
	perm := svcCfg.Permissions
	if envCfg != nil && envCfg.Permissions != nil {
		perm = envCfg.Permissions
	}

	// If no permissions restrictions specified, all authenticated users have access
	if perm == nil || (len(perm.GitHubTeams) == 0 && len(perm.GoogleGroups) == 0 && perm.RequireRepoAccess == "") {
		return true, ""
	}

	var matchConditions []string

	// 2. Check GitHub Team Membership
	if len(perm.GitHubTeams) > 0 {
		hasTeam := false
		for _, requiredTeam := range perm.GitHubTeams {
			for _, userTeam := range u.Teams {
				if strings.EqualFold(strings.TrimSpace(userTeam), strings.TrimSpace(requiredTeam)) {
					hasTeam = true
					break
				}
			}
			if hasTeam {
				break
			}
		}
		if hasTeam {
			return true, ""
		}
		matchConditions = append(matchConditions, fmt.Sprintf("GitHub team: %s", strings.Join(perm.GitHubTeams, " or ")))
	}

	// 3. Check Google Group Membership
	if len(perm.GoogleGroups) > 0 {
		hasGroup := false
		for _, requiredGroup := range perm.GoogleGroups {
			for _, userGroup := range u.Groups {
				if strings.EqualFold(strings.TrimSpace(userGroup), strings.TrimSpace(requiredGroup)) {
					hasGroup = true
					break
				}
			}
			if hasGroup {
				break
			}
		}
		if hasGroup {
			return true, ""
		}
		matchConditions = append(matchConditions, fmt.Sprintf("Google group: %s", strings.Join(perm.GoogleGroups, " or ")))
	}

	// 4. Check Source Repo Access
	if perm.RequireRepoAccess != "" {
		targetRepo := svcCfg.Context.Repo
		if targetRepo == "" {
			targetRepo = svcCfg.Source.Repository
		}
		targetRepo = cleanRepoSlug(targetRepo)

		reqLevel := strings.ToLower(perm.RequireRepoAccess)
		userAccess := strings.ToLower(u.Repos[targetRepo])

		allowed := false
		if reqLevel == "read" {
			allowed = userAccess == "read" || userAccess == "write" || userAccess == "admin"
		} else if reqLevel == "write" {
			allowed = userAccess == "write" || userAccess == "admin"
		} else if reqLevel == "admin" {
			allowed = userAccess == "admin"
		}

		if allowed {
			return true, ""
		}
		matchConditions = append(matchConditions, fmt.Sprintf("%s access to repository %s", reqLevel, targetRepo))
	}

	if len(matchConditions) > 0 {
		return false, fmt.Sprintf("Requires %s", strings.Join(matchConditions, " OR "))
	}

	return false, "Insufficient permissions"
}

func cleanRepoSlug(repoURL string) string {
	s := strings.TrimPrefix(repoURL, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")
	s = strings.TrimPrefix(s, "git@github.com:")
	s = strings.TrimSuffix(s, ".git")
	return strings.TrimSpace(s)
}
