package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"deploystackr/pkg/api"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/store"
)

func runCmd(t *testing.T, dir string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Committer",
		"GIT_COMMITTER_EMAIL=committer@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %s: %v", name, args, string(out), err)
	}
	return string(out)
}

func setupLocalGitRepo(t *testing.T) (string, string) {
	tmpDir, err := os.MkdirTemp("", "gitops_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}

	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	runCmd(t, repoDir, "git", "init", "-b", "main")

	// Commit 1: v1.0.0
	stagingDir := filepath.Join(repoDir, "apps/staging/billing")
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	valFile := filepath.Join(stagingDir, "values.yaml")
	content1 := `# Staging configuration
image:
  repository: ghcr.io/org/billing-service
  tag: v1.0.0
`
	if err := os.WriteFile(valFile, []byte(content1), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "Initial deployment v1.0.0")

	// Commit 2: Unrelated commit
	readme := filepath.Join(repoDir, "README.md")
	_ = os.WriteFile(readme, []byte("# GitOps Manifests\n"), 0644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "docs: add readme")

	// Commit 3: v1.1.0 (with PR in message)
	content2 := `# Staging configuration
image:
  repository: ghcr.io/org/billing-service
  tag: v1.1.0
`
	if err := os.WriteFile(valFile, []byte(content2), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "chore(staging): promote billing to v1.1.0 (#42)")

	return tmpDir, repoDir
}

func TestEndToEndIndexerAndPromotion(t *testing.T) {
	tmpDir, repoDir := setupLocalGitRepo(t)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Version: "1",
		GitOps: config.GitOpsConfig{
			Repo:   repoDir,
			Branch: "main",
		},
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				Source: config.SourceConfig{
					Type:       "git_tags",
					Repository: repoDir,
					Filter:     "^v[0-9]+",
				},
				Context: config.ContextConfig{
					Repo: repoDir,
				},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {
						Repo:   repoDir,
						Branch: "main",
						Target: config.TargetConfig{
							File: "apps/staging/billing/values.yaml",
							Path: "$.image.tag",
						},
						WriteStrategy: config.WriteStrategy{
							Mode:          "direct",
							CommitMessage: "chore(staging): promote billing to {{ .TargetVersion }}",
						},
					},
				},
			},
		},
	}

	memStore := store.NewMemoryStore()
	scratchDir := filepath.Join(tmpDir, "scratch")

	idx, err := indexer.NewIndexer(cfg, memStore, scratchDir)
	if err != nil {
		t.Fatalf("NewIndexer failed: %v", err)
	}

	ctx := context.Background()
	if err := idx.ReindexAll(ctx); err != nil {
		t.Fatalf("ReindexAll failed: %v", err)
	}

	history := memStore.GetEnvironmentHistory("billing-service", "staging")
	if len(history) != 2 {
		t.Fatalf("expected 2 deployment events, got %d", len(history))
	}

	// First event should be newest (v1.1.0)
	if history[0].Version != "v1.1.0" {
		t.Errorf("expected newest version v1.1.0, got %s", history[0].Version)
	}
	if !history[0].IsActive {
		t.Errorf("expected v1.1.0 to be active")
	}
	if history[0].PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", history[0].PRNumber)
	}

	// Second event should be superseded (v1.0.0)
	if history[1].Version != "v1.0.0" {
		t.Errorf("expected superseded version v1.0.0, got %s", history[1].Version)
	}
	if history[1].IsActive {
		t.Errorf("expected v1.0.0 to be retired")
	}
	if history[1].RetiredAt == nil {
		t.Errorf("expected RetiredAt to be set for v1.0.0")
	}

	// Test Promotion Engine
	depMgr := deployer.NewManager(cfg, memStore, idx, scratchDir)
	res, err := depMgr.Promote(ctx, deployer.PromotionRequest{
		Service:       "billing-service",
		Environment:   "staging",
		TargetVersion: "v1.2.0",
		User:          "Deploy Engineer",
		Email:         "engineer@example.com",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected promotion success")
	}
	if res.NewVersion != "v1.2.0" {
		t.Errorf("expected v1.2.0, got %s", res.NewVersion)
	}

	// Test API Server
	srv := api.NewServer(cfg, memStore, idx, depMgr, nil)
	req := httptest.NewRequest("GET", "/api/services", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/services, got %d", rec.Code)
	}

	time.Sleep(100 * time.Millisecond)
}
