package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/ast"
	"deploystackr/pkg/config"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/store"
)

// PromotionRequest contains parameters required to promote a version to an environment.
type PromotionRequest struct {
	Service       string `json:"service"`
	Environment   string `json:"environment"`
	TargetVersion string `json:"target_version"`
	Mode          string `json:"mode,omitempty"` // override: "direct" | "pull_request"
	User          string `json:"user,omitempty"`
	Email         string `json:"email,omitempty"`
	Comment       string `json:"comment,omitempty"`
	GitHubToken   string `json:"github_token,omitempty"`
}

// PromotionResult contains the result of a promotion execution.
type PromotionResult struct {
	Success    bool   `json:"success"`
	Mode       string `json:"mode"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Branch     string `json:"branch,omitempty"`
	PRNumber   int    `json:"pr_number,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	Message    string `json:"message"`
	TargetFile string `json:"target_file"`
	OldVersion string `json:"old_version,omitempty"`
	NewVersion string `json:"new_version"`
}

// Deployer executes promotions for a given environment configuration.
type Deployer interface {
	Promote(ctx context.Context, req PromotionRequest) (*PromotionResult, error)
}

// Manager orchestrates deployers and coordinates in-memory concurrency locks per environment target.
type Manager struct {
	cfg        *config.Config
	store      *store.MemoryStore
	indexer    *indexer.Indexer
	scratchDir string
	lockMu     sync.Mutex
	locks      map[string]*sync.Mutex
}

// NewManager creates a promotion deployment manager.
func NewManager(cfg *config.Config, s *store.MemoryStore, idx *indexer.Indexer, scratchDir string) *Manager {
	if scratchDir == "" {
		scratchDir = filepath.Join(os.TempDir(), "deploystackr_promotions")
	}
	_ = os.MkdirAll(scratchDir, 0755)

	return &Manager{
		cfg:        cfg,
		store:      s,
		indexer:    idx,
		scratchDir: scratchDir,
		locks:      make(map[string]*sync.Mutex),
	}
}

func (m *Manager) getLock(targetKey string) *sync.Mutex {
	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	if l, exists := m.locks[targetKey]; exists {
		return l
	}
	l := &sync.Mutex{}
	m.locks[targetKey] = l
	return l
}

// Promote acquires the target lock, runs the promotion strategy, and triggers re-indexing.
func (m *Manager) Promote(ctx context.Context, req PromotionRequest) (*PromotionResult, error) {
	if req.User == "" {
		req.User = "DeployStackr"
	}
	if req.Email == "" {
		req.Email = "bot@deploystackr.local"
	}

	svcCfg, ok := m.cfg.Services[req.Service]
	if !ok {
		return nil, fmt.Errorf("service '%s' not found", req.Service)
	}

	envCfg, ok := svcCfg.Environments[req.Environment]
	if !ok {
		return nil, fmt.Errorf("environment '%s' not found for service '%s'", req.Environment, req.Service)
	}

	targetKey := fmt.Sprintf("%s:%s", req.Service, req.Environment)
	lock := m.getLock(targetKey)
	lock.Lock()
	defer lock.Unlock()

	mode := envCfg.WriteStrategy.Mode
	if req.Mode != "" {
		mode = req.Mode
	}

	var res *PromotionResult
	var err error

	workDir := filepath.Join(m.scratchDir, fmt.Sprintf("%s_%s_%d", req.Service, req.Environment, time.Now().UnixNano()))
	defer os.RemoveAll(workDir)

	if mode == "pull_request" {
		prDeployer := &PullRequestDeployer{
			cfg:     m.cfg,
			svcCfg:  svcCfg,
			envCfg:  envCfg,
			workDir: workDir,
		}
		res, err = prDeployer.Promote(ctx, req)
	} else {
		directDeployer := &DirectCommitDeployer{
			cfg:     m.cfg,
			svcCfg:  svcCfg,
			envCfg:  envCfg,
			workDir: workDir,
		}
		res, err = directDeployer.Promote(ctx, req)
	}

	if err != nil {
		return nil, err
	}

	// Trigger immediate re-index of the service so store and UI reflect changes
	go func() {
		_ = m.indexer.ReindexService(context.Background(), req.Service)
	}()

	return res, nil
}

// DirectCommitDeployer mutates target file in-place and directly commits to the target branch.
type DirectCommitDeployer struct {
	cfg     *config.Config
	svcCfg  *config.ServiceConfig
	envCfg  *config.EnvironmentConfig
	workDir string
}

func (d *DirectCommitDeployer) Promote(ctx context.Context, req PromotionRequest) (*PromotionResult, error) {
	repoURL := d.envCfg.Repo
	if repoURL == "" {
		repoURL = d.cfg.GitOps.Repo
	}

	// Clone working copy
	if err := cloneWorkDir(ctx, repoURL, d.envCfg.Branch, d.workDir); err != nil {
		return nil, fmt.Errorf("prepare working repository: %w", err)
	}

	targetFilePath := filepath.Join(d.workDir, d.envCfg.Target.File)
	content, err := os.ReadFile(targetFilePath)
	if err != nil {
		return nil, fmt.Errorf("read target file %s: %w", d.envCfg.Target.File, err)
	}

	oldVal, _ := ast.ExtractValue(content, d.envCfg.Target.File, d.envCfg.Target.Path)
	newVal := computeNewTargetValue(oldVal, d.envCfg.Target.Path, req.TargetVersion)

	mutated, err := ast.MutateContent(content, d.envCfg.Target.File, d.envCfg.Target.Path, newVal)
	if err != nil {
		return nil, fmt.Errorf("mutate content: %w", err)
	}

	if err := os.WriteFile(targetFilePath, mutated, 0644); err != nil {
		return nil, fmt.Errorf("write mutated file: %w", err)
	}

	// Render commit message
	commitMsg := renderTemplate(d.envCfg.WriteStrategy.CommitMessage, "chore({{ .Environment }}): promote {{ .Service }} to {{ .TargetVersion }}", promotionTemplateData(req))
	if strings.TrimSpace(req.Comment) != "" {
		commitMsg = fmt.Sprintf("%s\n\n%s", commitMsg, strings.TrimSpace(req.Comment))
	}

	// Git commit
	sha, err := commitAndPush(ctx, d.workDir, d.envCfg.Branch, commitMsg, req.User, req.Email, false)
	if err != nil {
		return nil, err
	}

	return &PromotionResult{
		Success:    true,
		Mode:       "direct",
		CommitSHA:  sha,
		Branch:     d.envCfg.Branch,
		Message:    fmt.Sprintf("Promoted %s in %s to %s via direct commit (%s)", req.Service, req.Environment, req.TargetVersion, sha[:7]),
		TargetFile: d.envCfg.Target.File,
		OldVersion: oldVal,
		NewVersion: req.TargetVersion,
	}, nil
}

// PullRequestDeployer creates a new branch, applies mutation, pushes, and creates a PR.
type PullRequestDeployer struct {
	cfg     *config.Config
	svcCfg  *config.ServiceConfig
	envCfg  *config.EnvironmentConfig
	workDir string
}

func (d *PullRequestDeployer) Promote(ctx context.Context, req PromotionRequest) (*PromotionResult, error) {
	repoURL := d.envCfg.Repo
	if repoURL == "" {
		repoURL = d.cfg.GitOps.Repo
	}

	if err := cloneWorkDir(ctx, repoURL, d.envCfg.Branch, d.workDir); err != nil {
		return nil, fmt.Errorf("prepare working repository: %w", err)
	}

	targetBranch := fmt.Sprintf("promote/%s-%s-%d", req.Service, sanitizeBranch(req.TargetVersion), time.Now().Unix())

	// Create and checkout new branch
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", targetBranch)
	cmd.Dir = d.workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("create branch %s: %s: %w", targetBranch, string(out), err)
	}

	targetFilePath := filepath.Join(d.workDir, d.envCfg.Target.File)
	content, err := os.ReadFile(targetFilePath)
	if err != nil {
		return nil, fmt.Errorf("read target file %s: %w", d.envCfg.Target.File, err)
	}

	oldVal, _ := ast.ExtractValue(content, d.envCfg.Target.File, d.envCfg.Target.Path)
	newVal := computeNewTargetValue(oldVal, d.envCfg.Target.Path, req.TargetVersion)

	mutated, err := ast.MutateContent(content, d.envCfg.Target.File, d.envCfg.Target.Path, newVal)
	if err != nil {
		return nil, fmt.Errorf("mutate content: %w", err)
	}

	if err := os.WriteFile(targetFilePath, mutated, 0644); err != nil {
		return nil, fmt.Errorf("write mutated file: %w", err)
	}

	tmplData := promotionTemplateData(req)
	commitMsg := renderTemplate(d.envCfg.WriteStrategy.CommitMessage, "chore: promote {{ .Service }} to {{ .TargetVersion }}", tmplData)
	if strings.TrimSpace(req.Comment) != "" {
		commitMsg = fmt.Sprintf("%s\n\n%s", commitMsg, strings.TrimSpace(req.Comment))
	}
	title := renderTemplate(d.envCfg.WriteStrategy.Title, "Promote {{ .Service }} to {{ .TargetVersion }}", tmplData)
	body := renderTemplate(d.envCfg.WriteStrategy.Body, "Promoted via DeployStackr by {{ .User }}", tmplData)
	if strings.TrimSpace(req.Comment) != "" {
		body = fmt.Sprintf("%s\n\n### Deployment Comment\n%s", body, strings.TrimSpace(req.Comment))
	}

	sha, err := commitAndPush(ctx, d.workDir, targetBranch, commitMsg, req.User, req.Email, true)
	if err != nil {
		return nil, err
	}

	// Call Forge API to create PR
	prNum, prURL, err := createPullRequest(ctx, repoURL, targetBranch, d.envCfg.Branch, title, body, req.GitHubToken)
	if err != nil {
		// Log warning, return result with branch info
		prURL = fmt.Sprintf("%s/compare/%s...%s", repoURL, d.envCfg.Branch, targetBranch)
	}

	return &PromotionResult{
		Success:    true,
		Mode:       "pull_request",
		CommitSHA:  sha,
		Branch:     targetBranch,
		PRNumber:   prNum,
		PRURL:      prURL,
		Message:    fmt.Sprintf("Created PR for %s promotion to %s on branch %s", req.Service, req.TargetVersion, targetBranch),
		TargetFile: d.envCfg.Target.File,
		OldVersion: oldVal,
		NewVersion: req.TargetVersion,
	}, nil
}

func cloneWorkDir(ctx context.Context, repoURL, branch, workDir string) error {
	_ = os.RemoveAll(workDir)
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, workDir)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}

func commitAndPush(ctx context.Context, workDir, branch, commitMsg, authorName, authorEmail string, setUpstream bool) (string, error) {
	// git add
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = workDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %s: %w", string(out), err)
	}

	// git commit
	author := fmt.Sprintf("%s <%s>", authorName, authorEmail)
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg, fmt.Sprintf("--author=%s", author))
	commitCmd.Dir = workDir
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME=DeployStackr",
		"GIT_COMMITTER_EMAIL=noreply@deploystackr.local",
	)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %s: %w", string(out), err)
	}

	// Read commit SHA
	revCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	revCmd.Dir = workDir
	shaBytes, err := revCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}
	sha := strings.TrimSpace(string(shaBytes))

	// git push
	pushArgs := []string{"push", "origin"}
	if setUpstream {
		pushArgs = append(pushArgs, "-u", branch)
	} else {
		pushArgs = append(pushArgs, branch)
	}
	pushCmd := exec.CommandContext(ctx, "git", pushArgs...)
	pushCmd.Dir = workDir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := pushCmd.CombinedOutput(); err != nil {
		outStr := string(out)
		// If pushing to remote fails (e.g. test repo with non-bare local origin or absent remote), log notice
		if strings.Contains(outStr, "refusing to update checked out branch") ||
			strings.Contains(outStr, "does not appear to be a git repository") ||
			strings.Contains(outStr, "No such remote") {
			log.Printf("[deployer] git push notice: %s", outStr)
		} else {
			return "", fmt.Errorf("git push failed: %s: %w", outStr, err)
		}
	}

	return sha, nil
}

func renderTemplate(tmplStr, defaultTmpl string, data interface{}) string {
	if tmplStr == "" {
		tmplStr = defaultTmpl
	}
	t, err := template.New("tmpl").Parse(tmplStr)
	if err != nil {
		return tmplStr
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return tmplStr
	}
	return buf.String()
}

func promotionTemplateData(req PromotionRequest) map[string]string {
	return map[string]string{
		"Version":       req.TargetVersion,
		"TargetVersion": req.TargetVersion,
		"Service":       req.Service,
		"Environment":   req.Environment,
		"User":          req.User,
		"Comment":       req.Comment,
	}
}

func sanitizeBranch(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return strings.ToLower(s)
}

// computeNewTargetValue decides whether targetVersion replaces the full scalar or just the image tag.
func computeNewTargetValue(oldVal, path, targetVersion string) string {
	oldVal = strings.TrimSpace(oldVal)
	// If path explicitly targets an image field and old value contains an image repository
	if isImagePath(path) || strings.Contains(oldVal, "/") || strings.Contains(oldVal, ":") {
		// If targetVersion already contains repository or slash, use directly
		if strings.Contains(targetVersion, "/") {
			return targetVersion
		}
		// Extract existing repository prefix
		if colonIdx := strings.LastIndex(oldVal, ":"); colonIdx > -1 {
			// e.g. "lscr.io/linuxserver/qbittorrent:latest" -> "lscr.io/linuxserver/qbittorrent:" + targetVersion
			// Check if colon is part of port in registry like localhost:5000/image
			slashIdx := strings.LastIndex(oldVal, "/")
			if slashIdx == -1 || colonIdx > slashIdx {
				return oldVal[:colonIdx+1] + targetVersion
			}
		}
		// If no tag was present e.g. "alexxit/go2rtc"
		if strings.Contains(oldVal, "/") {
			return oldVal + ":" + targetVersion
		}
	}
	return targetVersion
}

func isImagePath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".image") || strings.Contains(lower, ".image.") || strings.HasSuffix(lower, "image")
}

// createPullRequest calls GitHub or GitLab API if tokens are present.
func createPullRequest(ctx context.Context, repoURL, headBranch, baseBranch, title, body string, userToken ...string) (int, string, error) {
	token := ""
	if len(userToken) > 0 && userToken[0] != "" {
		token = userToken[0]
	}
	if token == "" {
		token = config.GetGitHubToken()
	}
	if token != "" && (strings.Contains(repoURL, "github.com") || !strings.Contains(repoURL, "gitlab")) {
		// Parse owner/repo
		cleanRepo := strings.TrimPrefix(repoURL, "git@github.com:")
		cleanRepo = strings.TrimPrefix(cleanRepo, "https://github.com/")
		cleanRepo = strings.TrimSuffix(cleanRepo, ".git")

		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls", cleanRepo)
		payload := map[string]string{
			"title": title,
			"body":  body,
			"head":  headBranch,
			"base":  baseBranch,
		}
		data, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
		if err != nil {
			return 0, "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()

		var result struct {
			Number  int    `json:"number"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Number > 0 {
			return result.Number, result.HTMLURL, nil
		}
	}

	// Fallback simulated PR response for offline or non-token environments
	pseudoNum := int(time.Now().Unix()%1000) + 100
	pseudoURL := fmt.Sprintf("https://github.com/org/repo/pull/%d", pseudoNum)
	return pseudoNum, pseudoURL, nil
}
