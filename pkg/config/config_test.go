package config

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	yamlData := `
version: "1"
gitops:
  repo: "git@github.com:org/gitops.git"
  branch: "main"
services:
  test-svc:
    source:
      type: "oci"
      repository: "ghcr.io/org/test"
    environments:
      staging:
        target:
          file: "apps/staging/values.yaml"
          path: "$.image.tag"
`
	cfg, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error parsing yaml: %v", err)
	}

	if cfg.GitOps.Repo != "git@github.com:org/gitops.git" {
		t.Errorf("expected repo git@github.com:org/gitops.git, got %s", cfg.GitOps.Repo)
	}
	if svc, ok := cfg.Services["test-svc"]; !ok {
		t.Fatalf("expected test-svc in services")
	} else if env, ok := svc.Environments["staging"]; !ok {
		t.Fatalf("expected staging environment in test-svc")
	} else if env.Repo != "git@github.com:org/gitops.git" {
		t.Errorf("expected inherited repo in staging env, got %s", env.Repo)
	}
}

func TestContextConfig_TagPrefix(t *testing.T) {
	yamlData := `
version: "1"
gitops:
  repo: "git@github.com:org/gitops.git"
  branch: "main"
services:
  test-svc:
    source:
      type: "oci"
      repository: "ghcr.io/org/test"
    context:
      repo: "org/test"
      tag_prefix: "v"
    environments:
      prod:
        target:
          file: "apps/prod/values.yaml"
          path: "$.image.tag"
`
	cfg, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error parsing yaml: %v", err)
	}
	svc := cfg.Services["test-svc"]
	if svc.Context.GetTagPrefix() != "v" {
		t.Errorf("expected tag_prefix 'v', got %s", svc.Context.GetTagPrefix())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestLoadRemoteConfig(t *testing.T) {
	origClient := HTTPClient
	defer func() { HTTPClient = origClient }()

	yamlContent := `
version: "1"
gitops:
  repo: "https://github.com/remote/gitops.git"
  branch: "main"
services:
  remote-svc:
    source:
      type: "git_tags"
      repository: "https://github.com/remote/svc.git"
    environments:
      prod:
        target:
          file: "prod.yaml"
          path: "$.tag"
`
	HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(yamlContent)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	cfg, err := Load("https://example.com/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error loading remote config: %v", err)
	}

	if _, ok := cfg.Services["remote-svc"]; !ok {
		t.Fatalf("expected remote-svc loaded from remote URL")
	}
}

func TestMergeServices(t *testing.T) {
	cfg := &Config{
		Version: "1",
		GitOps: GitOpsConfig{
			Repo:   "https://github.com/org/gitops.git",
			Branch: "main",
		},
		Services: map[string]*ServiceConfig{
			"existing": {
				DisplayName: "Existing Service",
			},
		},
	}

	newServices := map[string]*ServiceConfig{
		"new-app": {
			DisplayName: "Discovered Service",
			Environments: map[string]*EnvironmentConfig{
				"staging": {
					Target: TargetConfig{
						File: "apps/staging/app/values.yaml",
						Path: "$.image.tag",
					},
				},
			},
		},
	}

	cfg.MergeServices(newServices)

	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services after merge, got %d", len(cfg.Services))
	}

	newApp := cfg.Services["new-app"]
	if newApp == nil {
		t.Fatalf("expected new-app in cfg.Services")
	}
	if newApp.Environments["staging"].Repo != "https://github.com/org/gitops.git" {
		t.Errorf("expected inherited repo in new-app staging env")
	}
}

func TestLoadLocalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "test-config.yaml")
	err := os.WriteFile(confPath, []byte(`
version: "1"
gitops:
  repo: "/local/repo"
  branch: "master"
`), 0644)
	if err != nil {
		t.Fatalf("failed to write tmp config: %v", err)
	}

	cfg, err := Load(confPath)
	if err != nil {
		t.Fatalf("failed to load local config: %v", err)
	}
	if cfg.GitOps.Repo != "/local/repo" || cfg.GitOps.Branch != "master" {
		t.Errorf("unexpected local config loaded: %+v", cfg.GitOps)
	}
}

func TestServiceOrderPreserved(t *testing.T) {
	yamlData := `
version: "1"
services:
  zeta-service:
    display_name: "Zeta"
  alpha-service:
    display_name: "Alpha"
  gamma-service:
    display_name: "Gamma"
  beta-service:
    display_name: "Beta"
`
	cfg, err := Parse([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error parsing yaml: %v", err)
	}

	expected := []string{"zeta-service", "alpha-service", "gamma-service", "beta-service"}
	if len(cfg.ServiceOrder) != len(expected) {
		t.Fatalf("expected %d ordered services, got %d", len(expected), len(cfg.ServiceOrder))
	}
	for i, want := range expected {
		if cfg.ServiceOrder[i] != want {
			t.Errorf("at index %d: expected %s, got %s", i, want, cfg.ServiceOrder[i])
		}
	}
}

func TestSyncInRepoServices_Prune(t *testing.T) {
	cfg := &Config{
		Version: "1",
		GitOps: GitOpsConfig{
			Repo:   "https://github.com/org/gitops.git",
			Branch: "main",
		},
		Services: map[string]*ServiceConfig{
			"base-service": {
				DisplayName: "Base Service",
			},
		},
		ServiceOrder: []string{"base-service"},
	}

	initialInRepo := map[string]*ServiceConfig{
		"nginx-ingress": {
			DisplayName: "NGINX Ingress",
		},
		"cert-manager": {
			DisplayName: "cert-manager",
		},
	}

	removed := cfg.SyncInRepoServices(initialInRepo, "nginx-ingress", "cert-manager")
	if len(removed) != 0 {
		t.Errorf("expected 0 removed on initial sync, got %d", len(removed))
	}
	if len(cfg.Services) != 3 {
		t.Fatalf("expected 3 services, got %d", len(cfg.Services))
	}

	// Next sync: nginx-ingress is removed, envoy-gateway is added
	updatedInRepo := map[string]*ServiceConfig{
		"envoy-gateway": {
			DisplayName: "Envoy Gateway",
		},
		"cert-manager": {
			DisplayName: "cert-manager",
		},
	}

	removed = cfg.SyncInRepoServices(updatedInRepo, "envoy-gateway", "cert-manager")
	if len(removed) != 1 || removed[0] != "nginx-ingress" {
		t.Fatalf("expected ['nginx-ingress'] removed, got %v", removed)
	}
	if _, ok := cfg.Services["nginx-ingress"]; ok {
		t.Errorf("expected nginx-ingress to be pruned from Services")
	}
	if _, ok := cfg.Services["envoy-gateway"]; !ok {
		t.Errorf("expected envoy-gateway in Services")
	}
	if _, ok := cfg.Services["base-service"]; !ok {
		t.Errorf("expected base-service to be preserved")
	}
}
