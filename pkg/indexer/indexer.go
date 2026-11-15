package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/ast"
	"deploystackr/pkg/config"
	"deploystackr/pkg/git"
	"deploystackr/pkg/store"
)

// Indexer manages ephemeral clones and ingests Git history into the in-memory store.
type Indexer struct {
	cfg        *config.Config
	store      *store.MemoryStore
	scratchDir string
	mu         sync.Mutex
}

// NewIndexer creates an Indexer instance with a designated scratch directory for bare clones.
func NewIndexer(cfg *config.Config, s *store.MemoryStore, scratchDir string) (*Indexer, error) {
	if scratchDir == "" {
		scratchDir = filepath.Join(os.TempDir(), "deploystackr_clones")
	}
	if err := os.MkdirAll(scratchDir, 0755); err != nil {
		return nil, fmt.Errorf("create scratch directory: %w", err)
	}

	return &Indexer{
		cfg:        cfg,
		store:      s,
		scratchDir: scratchDir,
	}, nil
}

// ReindexAll scans all services and environments defined in config and updates the in-memory store.
func (idx *Indexer) ReindexAll(ctx context.Context) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Check if the GitOps repository contains in-repo configuration (.deploystackr.yaml)
	idx.discoverInRepoConfig(ctx)

	for svcName, svcCfg := range idx.cfg.Services {
		for envName, envCfg := range svcCfg.Environments {
			if err := idx.indexEnvironment(ctx, svcName, envName, envCfg); err != nil {
				log.Printf("[indexer] Warning: failed to index %s/%s: %v", svcName, envName, err)
			}
		}
	}

	// Discover and reconcile open promotion PRs across repos
	idx.discoverOpenPRs(ctx)

	return nil
}

// discoverInRepoConfig inspects the root of the GitOps repository for .deploystackr.yaml and overlays services.
func (idx *Indexer) discoverInRepoConfig(ctx context.Context) {
	repoURL := idx.cfg.GitOps.Repo
	if repoURL == "" {
		return
	}
	branch := idx.cfg.GitOps.Branch
	if branch == "" {
		branch = "main"
	}

	repoDir := idx.GetRepoPath(repoURL)
	if err := git.CloneOrFetch(ctx, repoURL, branch, repoDir, true); err != nil {
		log.Printf("[indexer] Warning: failed to fetch GitOps repo for in-repo config discovery: %v", err)
		return
	}

	candidates := []string{
		".deploystackr.yaml",
		".deploystackr.yml",
		"deploystackr.yaml",
		"deploystackr.yml",
	}
	if idx.cfg.GitOps.ConfigFile != "" {
		candidates = append([]string{idx.cfg.GitOps.ConfigFile}, candidates...)
	}

	var configData []byte
	var foundFile string

	// 1. If repoURL is a local directory, try reading directly from working tree
	if fi, err := os.Stat(repoURL); err == nil && fi.IsDir() {
		for _, file := range candidates {
			p := filepath.Join(repoURL, file)
			if data, err := os.ReadFile(p); err == nil {
				configData = data
				foundFile = file
				break
			}
		}
	}

	// 2. If not found locally, query git repository branch
	if configData == nil {
		for _, file := range candidates {
			data, err := git.ShowFileAtCommit(ctx, repoDir, branch, file)
			if err == nil && len(data) > 0 {
				configData = data
				foundFile = file
				break
			}
		}
	}

	if configData == nil {
		return
	}

	inRepoCfg, err := config.Parse(configData)
	if err != nil {
		log.Printf("[indexer] Warning: found in-repo config %s but failed to parse: %v", foundFile, err)
		return
	}

	removed := idx.cfg.SyncInRepoServices(inRepoCfg.Services, inRepoCfg.ServiceOrder...)
	for _, svc := range removed {
		idx.store.DeleteService(svc)
		log.Printf("[indexer] Pruned removed service '%s' from catalog", svc)
	}

	if len(inRepoCfg.Services) > 0 {
		log.Printf("[indexer] Discovered and synced %d service(s) from in-repo GitOps config (%s)", len(inRepoCfg.Services), foundFile)
	}

	if len(inRepoCfg.Webhooks) > 0 {
		idx.cfg.Webhooks = inRepoCfg.Webhooks
	}

	if len(inRepoCfg.EnvironmentPriority) > 0 {
		idx.cfg.EnvironmentPriority = inRepoCfg.EnvironmentPriority
	}
}

// ReindexService re-indexes all environments for a single service.
func (idx *Indexer) ReindexService(ctx context.Context, serviceName string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	svcCfg, ok := idx.cfg.Services[serviceName]
	if !ok {
		return fmt.Errorf("service not found in config: %s", serviceName)
	}

	for envName, envCfg := range svcCfg.Environments {
		if err := idx.indexEnvironment(ctx, serviceName, envName, envCfg); err != nil {
			log.Printf("[indexer] Warning: failed to index %s/%s: %v", serviceName, envName, err)
		}
	}
	return nil
}

// GetRepoPath returns the local path to the bare clone for a given repository URL.
func (idx *Indexer) GetRepoPath(repoURL string) string {
	h := sha256.Sum256([]byte(repoURL))
	repoHash := hex.EncodeToString(h[:8])
	base := filepath.Base(repoURL)
	return filepath.Join(idx.scratchDir, fmt.Sprintf("%s-%s.git", base, repoHash))
}

func (idx *Indexer) indexEnvironment(ctx context.Context, serviceName, envName string, envCfg *config.EnvironmentConfig) error {
	repoURL := envCfg.Repo
	if repoURL == "" {
		repoURL = idx.cfg.GitOps.Repo
	}
	if repoURL == "" {
		return fmt.Errorf("no repo configured for %s/%s", serviceName, envName)
	}

	repoDir := idx.GetRepoPath(repoURL)

	// Clone or fetch the bare repository
	if err := git.CloneOrFetch(ctx, repoURL, envCfg.Branch, repoDir, true); err != nil {
		return fmt.Errorf("clone or fetch %s: %w", repoURL, err)
	}

	// Retrieve commits modifying target file
	commits, err := git.LogFileCommits(ctx, repoDir, envCfg.Branch, envCfg.Target.File)
	if err != nil {
		return fmt.Errorf("log file commits: %w", err)
	}

	if len(commits) == 0 {
		return nil
	}

	// Commits are newest to oldest. Process oldest to newest to detect changes.
	type rawEvent struct {
		version string
		commit  git.CommitInfo
	}

	var rawEvents []rawEvent
	lastVal := ""

	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		content, err := git.ShowFileAtCommit(ctx, repoDir, c.SHA, envCfg.Target.File)
		if err != nil {
			continue
		}

		val, err := ast.ExtractValue(content, envCfg.Target.File, envCfg.Target.Path)
		if err != nil {
			continue
		}

		val = sanitizeVersion(val)
		if val != "" && val != lastVal {
			rawEvents = append(rawEvents, rawEvent{
				version: val,
				commit:  c,
			})
			lastVal = val
		}
	}

	if len(rawEvents) == 0 {
		return nil
	}

	// Build final DeploymentEvent list in reverse chronological order (newest first)
	total := len(rawEvents)
	events := make([]store.DeploymentEvent, 0, total)

	for i := total - 1; i >= 0; i-- {
		rev := rawEvents[i]
		event := store.DeploymentEvent{
			Service:       serviceName,
			Environment:   envName,
			Version:       rev.version,
			CommitSHA:     rev.commit.SHA,
			CommitMessage: rev.commit.Subject,
			AuthorName:    rev.commit.AuthorName,
			AuthorEmail:   rev.commit.AuthorEmail,
			PRNumber:      rev.commit.PRNumber,
			DeployedAt:    rev.commit.Timestamp,
		}

		if i == total-1 {
			// Latest active version
			event.IsActive = true
			event.RetiredAt = nil
			event.ActiveDuration = formatDuration(time.Since(event.DeployedAt))
		} else {
			// Superseded by newer event (which was at i+1 in rawEvents)
			supersededBy := rawEvents[i+1]
			retTime := supersededBy.commit.Timestamp
			event.RetiredAt = &retTime
			event.IsActive = false
			event.ActiveDuration = formatDuration(retTime.Sub(event.DeployedAt))
		}

		events = append(events, event)
	}

	idx.store.SetEvents(serviceName, envName, events)
	return nil
}

// formatDuration formats a duration into human-readable shorthand e.g. "3d 4h" or "45m".
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "0m"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "< 1m"
}

// sanitizeVersion trims quotes or whitespace from extracted strings.
func sanitizeVersion(v string) string {
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		return v[1 : len(v)-1]
	}
	return v
}

// discoverOpenPRs queries GitOps and deployment repositories for open pull requests and recovers/reconciles them in the store.
func (idx *Indexer) discoverOpenPRs(ctx context.Context) {
	token := config.GetGitHubToken()
	if token == "" {
		return
	}

	repoMap := make(map[string]bool)
	if idx.cfg.GitOps.Repo != "" {
		repoMap[idx.cfg.GitOps.Repo] = true
	}
	for _, svcCfg := range idx.cfg.Services {
		for _, envCfg := range svcCfg.Environments {
			if envCfg.Repo != "" {
				repoMap[envCfg.Repo] = true
			}
		}
	}

	openPRsByNumber := make(map[int]bool)

	for repoURL := range repoMap {
		if !strings.Contains(repoURL, "github.com") {
			continue
		}
		cleanRepo := strings.TrimPrefix(repoURL, "git@github.com:")
		cleanRepo = strings.TrimPrefix(cleanRepo, "https://github.com/")
		cleanRepo = strings.TrimSuffix(cleanRepo, ".git")

		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=open&per_page=50", cleanRepo)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[indexer] Warning: failed to fetch open PRs from %s: %v", cleanRepo, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var ghPRs []struct {
			Number    int       `json:"number"`
			HTMLURL   string    `json:"html_url"`
			Title     string    `json:"title"`
			CreatedAt time.Time `json:"created_at"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
			Head struct {
				Ref string `json:"ref"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ghPRs); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, ghPR := range ghPRs {
			headRef := ghPR.Head.Ref
			if !strings.HasPrefix(headRef, "promote/") {
				continue
			}
			openPRsByNumber[ghPR.Number] = true

			// Match to service
			var matchedSvc string
			var targetVersion string
			for svcName := range idx.cfg.Services {
				prefix := "promote/" + svcName + "-"
				if strings.HasPrefix(headRef, prefix) {
					matchedSvc = svcName
					rest := strings.TrimPrefix(headRef, prefix)
					if lastDash := strings.LastIndex(rest, "-"); lastDash > 0 {
						targetVersion = rest[:lastDash]
					} else {
						targetVersion = rest
					}
					break
				}
			}

			if matchedSvc == "" {
				continue
			}

			svcCfg := idx.cfg.Services[matchedSvc]
			if svcCfg == nil || len(svcCfg.Environments) == 0 {
				continue
			}

			// Find matching environment by branch
			var matchedEnv string
			for envName, envCfg := range svcCfg.Environments {
				if envCfg.Branch == ghPR.Base.Ref {
					matchedEnv = envName
					break
				}
			}
			if matchedEnv == "" {
				for envName := range svcCfg.Environments {
					matchedEnv = envName
					break
				}
			}

			if matchedEnv == "" {
				continue
			}

			user := ghPR.User.Login
			if user == "" {
				user = "DeployStackr"
			}

			idx.store.SetPendingPR(matchedSvc, matchedEnv, &store.PendingPR{
				Service:       matchedSvc,
				Environment:   matchedEnv,
				TargetVersion: targetVersion,
				Branch:        headRef,
				PRNumber:      ghPR.Number,
				PRURL:         ghPR.HTMLURL,
				User:          user,
				CreatedAt:     ghPR.CreatedAt,
			})
		}
	}

	// Reconcile store pending PRs: if any PR recorded in store is no longer open on GitHub, clear it
	allPending := idx.store.GetAllPendingPRs()
	for _, p := range allPending {
		if p.PRNumber > 0 && !openPRsByNumber[p.PRNumber] {
			log.Printf("[indexer] Clearing stale pending PR #%d for %s/%s (no longer open on GitHub)", p.PRNumber, p.Service, p.Environment)
			idx.store.ClearPendingPR(p.Service, p.Environment)
		}
	}
}
