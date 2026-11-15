package indexer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"deploystackr/pkg/config"
	"deploystackr/pkg/store"
)

func TestDiscoverInRepoConfig(t *testing.T) {
	// 1. Create a dummy gitops git repo
	gitopsDir := t.TempDir()

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v failed: %s (%v)", name, args, string(out), err)
		}
	}

	runCmd(gitopsDir, "git", "init")
	runCmd(gitopsDir, "git", "config", "user.name", "Tester")
	runCmd(gitopsDir, "git", "config", "user.email", "test@example.com")

	// Create values.yaml
	appDir := filepath.Join(gitopsDir, "apps", "staging")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "values.yaml"), []byte("image:\n  tag: v1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .deploystackr.yaml in root of the GitOps repo
	inRepoYaml := `
version: "1"
services:
  discovered-service:
    display_name: "Discovered Service"
    source:
      type: "git_tags"
      repository: "https://github.com/org/discovered.git"
    environments:
      staging:
        target:
          file: "apps/staging/values.yaml"
          path: "$.image.tag"
`
	if err := os.WriteFile(filepath.Join(gitopsDir, ".deploystackr.yaml"), []byte(inRepoYaml), 0644); err != nil {
		t.Fatal(err)
	}

	runCmd(gitopsDir, "git", "add", "-A")
	runCmd(gitopsDir, "git", "commit", "-m", "chore: initial gitops setup with .deploystackr.yaml")

	// 2. Set up DeployStackr with bootstrap config pointing only to the GitOps repo
	bootstrapCfg := &config.Config{
		Version: "1",
		GitOps: config.GitOpsConfig{
			Repo:   gitopsDir,
			Branch: "master",
		},
		Services: make(map[string]*config.ServiceConfig),
	}

	// Also check branch name (git init can be master or main)
	out, _ := exec.Command("git", "-C", gitopsDir, "branch", "--show-current").Output()
	branch := string(out)
	if branch != "" {
		bootstrapCfg.GitOps.Branch = string([]rune(branch)[:len(branch)-1]) // trim newline
	}

	memStore := store.NewMemoryStore()
	scratchDir := t.TempDir()

	idx, err := NewIndexer(bootstrapCfg, memStore, scratchDir)
	if err != nil {
		t.Fatalf("failed to create indexer: %v", err)
	}

	// 3. Run ReindexAll
	if err := idx.ReindexAll(context.Background()); err != nil {
		t.Fatalf("reindex failed: %v", err)
	}

	// 4. Verify discovered-service was loaded into idx.cfg.Services
	if len(idx.cfg.Services) != 1 {
		t.Fatalf("expected 1 service discovered, got %d", len(idx.cfg.Services))
	}
	svc, ok := idx.cfg.Services["discovered-service"]
	if !ok {
		t.Fatalf("expected discovered-service in idx.cfg.Services")
	}
	if svc.DisplayName != "Discovered Service" {
		t.Errorf("expected DisplayName 'Discovered Service', got '%s'", svc.DisplayName)
	}

	// 5. Verify the indexer indexed the environment version from values.yaml
	evt := memStore.GetActiveEvent("discovered-service", "staging")
	if evt == nil {
		t.Fatalf("expected active deployment event for discovered-service/staging")
	}
	if evt.Version != "v1.0.0" {
		t.Errorf("expected active version v1.0.0, got %s", evt.Version)
	}
}
