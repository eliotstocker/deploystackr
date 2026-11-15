package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deploystackr/pkg/auth"
	"deploystackr/pkg/cluster"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/store"
)

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Committer",
		"GIT_COMMITTER_EMAIL=committer@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %s: %v", args, dir, string(out), err)
	}
	return string(out)
}

func TestNormalizeRepoSlugAndResolveServices(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"org/billing-service", "org/billing-service"},
		{"https://github.com/org/billing-service.git", "org/billing-service"},
		{"http://github.com/Org/Billing-Service/", "org/billing-service"},
		{"git@github.com:org/billing-service.git", "org/billing-service"},
		{"ssh://git@github.com/org/billing-service.git", "org/billing-service"},
		{"github.com/org/billing-service", "org/billing-service"},
		{"ghcr.io/org/billing-service", "org/billing-service"},
		{"/tmp/local-repo/", "/tmp/local-repo"},
		{"./relative/repo/", "relative/repo"},
	}
	for _, tc := range cases {
		if got := NormalizeRepoSlug(tc.in); got != tc.want {
			t.Errorf("NormalizeRepoSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	cfg := &config.Config{
		ServiceOrder: []string{"billing-service", "worker-a", "worker-b", "fallback-svc"},
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				DisplayName: "Billing Service",
				Source:      config.SourceConfig{Type: "oci", Repository: "ghcr.io/org/billing-img"},
				Context:     config.ContextConfig{Repo: "https://github.com/org/billing-service.git"},
				Environments: map[string]*config.EnvironmentConfig{
					"staging":    {Branch: "main"},
					"production": {Branch: "main"},
				},
			},
			"worker-a": {
				DisplayName: "Worker A",
				Source:      config.SourceConfig{Type: "oci", Repository: "ghcr.io/org/worker-a"},
				Context:     config.ContextConfig{Repo: "org/monorepo"},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {Branch: "main"},
				},
			},
			"worker-b": {
				DisplayName: "Worker B",
				Source:      config.SourceConfig{Type: "oci", Repository: "ghcr.io/org/worker-b"},
				Context:     config.ContextConfig{Repo: "git@github.com:org/monorepo.git"},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {Branch: "main"},
				},
			},
			"fallback-svc": {
				DisplayName: "Fallback Service",
				Source:      config.SourceConfig{Type: "github_releases", Repository: "org/fallback-svc"},
				Environments: map[string]*config.EnvironmentConfig{
					"prod": {Branch: "main"},
				},
			},
		},
	}

	// Empty filters return all services in ServiceOrder
	all, err := ResolveServices(cfg, "", "")
	if err != nil {
		t.Fatalf("ResolveServices all failed: %v", err)
	}
	if len(all) != 4 || all[0].Name != "billing-service" || all[3].Name != "fallback-svc" {
		t.Fatalf("unexpected service order: %+v", all)
	}

	// Exact, case-insensitive, and DisplayName matches
	for _, query := range []string{"billing-service", "BILLING-SERVICE", "Billing Service", "billing service"} {
		res, err := ResolveServices(cfg, query, "")
		if err != nil || len(res) != 1 || res[0].Name != "billing-service" {
			t.Fatalf("ResolveServices(%q) = %+v, %v", query, res, err)
		}
	}

	// Context repo in various URL forms
	for _, repo := range []string{"org/billing-service", "git@github.com:org/billing-service.git", "https://github.com/org/billing-service"} {
		name, _, err := ResolveSingleService(cfg, "", repo)
		if err != nil || name != "billing-service" {
			t.Fatalf("ResolveSingleService(%q) = %q, %v", repo, name, err)
		}
	}

	// Repo passed in service parameter falls back to context_repo resolution
	name, _, err := ResolveSingleService(cfg, "git@github.com:org/billing-service.git", "")
	if err != nil || name != "billing-service" {
		t.Fatalf("fallback service->context_repo failed: %q, %v", name, err)
	}

	// Fallback to source.repository when context.repo is unset
	name, _, err = ResolveSingleService(cfg, "", "https://github.com/org/fallback-svc.git")
	if err != nil || name != "fallback-svc" {
		t.Fatalf("source.repository fallback failed: %q, %v", name, err)
	}

	// Monorepo: ResolveServices returns all matching services; ResolveSingleService errors with disambiguation
	mono, err := ResolveServices(cfg, "", "https://github.com/org/monorepo.git")
	if err != nil || len(mono) != 2 {
		t.Fatalf("expected 2 monorepo services, got %d (%v)", len(mono), err)
	}
	if _, _, err := ResolveSingleService(cfg, "", "org/monorepo"); err == nil || !strings.Contains(err.Error(), "multiple services") {
		t.Fatalf("expected multiple services error, got %v", err)
	}
	name, _, err = ResolveSingleService(cfg, "worker-b", "org/monorepo")
	if err != nil || name != "worker-b" {
		t.Fatalf("expected worker-b when disambiguated, got %q, %v", name, err)
	}

	// Conflicting service + context_repo
	if _, _, err := ResolveSingleService(cfg, "billing-service", "org/monorepo"); err == nil {
		t.Fatalf("expected conflict error when service and context_repo disagree")
	}
}

func TestMCPProtocolLifecycleHTTP(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GIT_TOKEN", "")

	cfg := &config.Config{Version: "1", Services: map[string]*config.ServiceConfig{}}
	memStore := store.NewMemoryStore()
	srv := NewServer(Deps{
		Config: cfg,
		Store:  memStore,
		Auth:   auth.NewAuthManager(&config.AuthConfig{DevMode: true}),
	})

	// 1. GET /mcp -> 405
	getReq := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on GET /mcp, got %d", getRec.Code)
	}

	// 2. DELETE /mcp -> 204
	delReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	delRec := httptest.NewRecorder()
	srv.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on DELETE /mcp, got %d", delRec.Code)
	}

	// 3. POST initialize with supported and unsupported protocolVersion
	initCases := []struct {
		requested string
		expected  string
	}{
		{"2025-03-26", "2025-03-26"},
		{"2024-11-05", "2024-11-05"},
		{"1999-01-01", "2025-03-26"},
	}
	for _, tc := range initCases {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + tc.requested + `","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("initialize %s status = %d", tc.requested, rec.Code)
		}
		var resp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"result"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal initialize: %v", err)
		}
		if resp.Result.ProtocolVersion != tc.expected {
			t.Fatalf("initialize(%s) got %s, want %s", tc.requested, resp.Result.ProtocolVersion, tc.expected)
		}
	}

	// 4. Notification returns 202 Accepted with empty body on HTTP
	notifReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	notifRec := httptest.NewRecorder()
	srv.ServeHTTP(notifRec, notifReq)
	if notifRec.Code != http.StatusAccepted || notifRec.Body.Len() != 0 {
		t.Fatalf("expected 202 empty body for notification, got %d (%q)", notifRec.Code, notifRec.Body.String())
	}

	// 5. Batch request over HTTP (initialize + notification + tools/list -> 2 responses)
	batchBody := `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":2,"method":"tools/list"}]`
	batchReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(batchBody))
	batchRec := httptest.NewRecorder()
	srv.ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("batch status = %d", batchRec.Code)
	}
	var batchResp []json.RawMessage
	if err := json.Unmarshal(batchRec.Body.Bytes(), &batchResp); err != nil || len(batchResp) != 2 {
		t.Fatalf("expected 2 responses in batch, got %s (%v)", batchRec.Body.String(), err)
	}

	// 6. Unknown method and unknown tool
	errReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"nonexistent/method"}`))
	errRec := httptest.NewRecorder()
	srv.ServeHTTP(errRec, errReq)
	if !strings.Contains(errRec.Body.String(), "-32601") {
		t.Fatalf("expected -32601 method not found, got %s", errRec.Body.String())
	}

	badToolReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"missing_tool","arguments":{}}}`))
	badToolRec := httptest.NewRecorder()
	srv.ServeHTTP(badToolRec, badToolReq)
	if !strings.Contains(badToolRec.Body.String(), "-32602") {
		t.Fatalf("expected -32602 invalid params for unknown tool, got %s", badToolRec.Body.String())
	}

	// 7. HTTP tools/list returns all 6 tools
	toolsReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":11,"method":"tools/list"}`))
	toolsRec := httptest.NewRecorder()
	srv.ServeHTTP(toolsRec, toolsReq)
	if toolsRec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d", toolsRec.Code)
	}
	var toolsResp struct {
		Result struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(toolsRec.Body.Bytes(), &toolsResp); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if len(toolsResp.Result.Tools) != 6 {
		t.Fatalf("expected 6 MCP tools, got %d", len(toolsResp.Result.Tools))
	}
}

func TestGetDeployedAndQueueTools(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GIT_TOKEN", "")

	// Mock OCI registry serving tags for billing-service and auth-service
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "billing-service") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "org/billing-service",
				"tags": []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "org/auth-service",
			"tags": []string{"v2.0.0", "v2.1.0"},
		})
	}))
	defer regServer.Close()
	regHost := strings.TrimPrefix(regServer.URL, "http://")

	// Mock Kubernetes cluster: progressing for billing-service, ready for auth-service
	kubeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "auth-service") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": 2,
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []map[string]interface{}{{"name": "app", "image": "auth:v2.1.0"}},
						},
					},
				},
				"status": map[string]interface{}{
					"replicas":        2,
					"readyReplicas":   2,
					"updatedReplicas": 2,
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": 2,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{{"name": "app", "image": "billing:v1.1.0"}},
					},
				},
			},
			"status": map[string]interface{}{
				"replicas":        2,
				"readyReplicas":   1,
				"updatedReplicas": 2,
			},
		})
	}))
	defer kubeServer.Close()

	cfg := &config.Config{
		Version: "1",
		GitOps: config.GitOpsConfig{
			Repo:   "https://github.com/org/gitops.git",
			Branch: "main",
		},
		EnvironmentPriority: []string{"production", "staging"},
		Cluster: &config.ClusterConfig{
			Enabled:          true,
			DefaultNamespace: "default",
			KubeAPIServer:    kubeServer.URL,
			Token:            "kube-token",
		},
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				DisplayName: "Billing Service",
				Source:      config.SourceConfig{Type: "oci", Repository: regHost + "/org/billing-service", Filter: `^v[0-9]+\.[0-9]+\.[0-9]+$`},
				Context:     config.ContextConfig{Repo: "https://github.com/org/billing-service.git"},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {
						Branch:        "main",
						Target:        config.TargetConfig{File: "apps/staging/billing/values.yaml", Path: "$.image.tag"},
						WriteStrategy: config.WriteStrategy{Mode: "direct"},
					},
					"production": {
						Branch:        "main",
						Target:        config.TargetConfig{File: "apps/prod/billing/values.yaml", Path: "$.image.tag"},
						WriteStrategy: config.WriteStrategy{Mode: "pull_request"},
					},
				},
			},
			"auth-service": {
				DisplayName: "Auth Service",
				Source:      config.SourceConfig{Type: "oci", Repository: regHost + "/org/auth-service", Filter: `^v[0-9]+\.[0-9]+\.[0-9]+$`},
				Context:     config.ContextConfig{Repo: "git@github.com:org/auth-service.git"},
				Environments: map[string]*config.EnvironmentConfig{
					"production": {
						Branch:        "main",
						Target:        config.TargetConfig{File: "apps/prod/auth/values.yaml", Path: "$.image.tag"},
						WriteStrategy: config.WriteStrategy{Mode: "direct"},
					},
				},
			},
		},
	}

	memStore := store.NewMemoryStore()
	now := time.Now()
	memStore.SetEvents("billing-service", "staging", []store.DeploymentEvent{
		{Service: "billing-service", Environment: "staging", Version: "v1.1.0", CommitSHA: "abc111", AuthorName: "Alice", DeployedAt: now.Add(-time.Hour), IsActive: true},
		{Service: "billing-service", Environment: "staging", Version: "v1.0.0", CommitSHA: "abc000", AuthorName: "Bob", DeployedAt: now.Add(-24 * time.Hour), IsActive: false},
	})
	memStore.SetEvents("billing-service", "production", []store.DeploymentEvent{
		{Service: "billing-service", Environment: "production", Version: "v1.0.0", CommitSHA: "prod00", AuthorName: "Alice", DeployedAt: now.Add(-48 * time.Hour), IsActive: true},
	})
	memStore.SetPendingPR("billing-service", "production", &store.PendingPR{
		Service:       "billing-service",
		Environment:   "production",
		TargetVersion: "v1.2.0",
		Branch:        "promote/billing-service-v1.2.0-1",
		PRNumber:      44,
		PRURL:         "https://github.com/org/gitops/pull/44",
		User:          "Alice",
		CreatedAt:     now.Add(-15 * time.Minute),
	})
	memStore.SetEvents("auth-service", "production", []store.DeploymentEvent{
		{Service: "auth-service", Environment: "production", Version: "v2.1.0", CommitSHA: "auth21", AuthorName: "Alice", DeployedAt: now.Add(-time.Hour), IsActive: true},
	})

	checker := cluster.NewChecker(cfg.Cluster)
	srv := NewServer(Deps{
		Config:         cfg,
		Store:          memStore,
		Auth:           auth.NewAuthManager(&config.AuthConfig{DevMode: true}),
		ClusterChecker: func() *cluster.Checker { return checker },
	})

	// 0. list_services discovery (unfiltered, query filter, and context_repo filter)
	listRes := callToolHTTP(t, srv, "", "list_services", map[string]interface{}{})
	if listRes.IsError {
		t.Fatalf("list_services returned error: %s", listRes.Content[0].Text)
	}
	var listOut struct {
		Count    int                   `json:"count"`
		Services []serviceCatalogEntry `json:"services"`
	}
	if err := json.Unmarshal([]byte(listRes.Content[0].Text), &listOut); err != nil {
		t.Fatalf("decode list_services: %v", err)
	}
	if listOut.Count != 2 || len(listOut.Services) != 2 {
		t.Fatalf("expected 2 services from list_services, got %+v", listOut)
	}
	if listOut.Services[0].Service != "auth-service" || listOut.Services[1].Service != "billing-service" {
		t.Fatalf("unexpected service ordering in list_services: %+v", listOut.Services)
	}

	queryRes := callToolHTTP(t, srv, "", "list_services", map[string]interface{}{"query": "billing"})
	if queryRes.IsError {
		t.Fatalf("list_services query returned error: %s", queryRes.Content[0].Text)
	}
	var queryOut struct {
		Count    int                   `json:"count"`
		Services []serviceCatalogEntry `json:"services"`
	}
	if err := json.Unmarshal([]byte(queryRes.Content[0].Text), &queryOut); err != nil {
		t.Fatalf("decode list_services query: %v", err)
	}
	if queryOut.Count != 1 || queryOut.Services[0].Service != "billing-service" {
		t.Fatalf("unexpected query filter output: %+v", queryOut)
	}

	repoRes := callToolHTTP(t, srv, "", "list_services", map[string]interface{}{"context_repo": "git@github.com:org/auth-service.git"})
	if repoRes.IsError {
		t.Fatalf("list_services context_repo returned error: %s", repoRes.Content[0].Text)
	}
	var repoOut struct {
		Count    int                   `json:"count"`
		Services []serviceCatalogEntry `json:"services"`
	}
	if err := json.Unmarshal([]byte(repoRes.Content[0].Text), &repoOut); err != nil {
		t.Fatalf("decode list_services context_repo: %v", err)
	}
	if repoOut.Count != 1 || repoOut.Services[0].Service != "auth-service" {
		t.Fatalf("unexpected context_repo filter output: %+v", repoOut)
	}

	// 1. get_deployed by context_repo (SSH format matching HTTPS config) + include_history
	res := callToolHTTP(t, srv, "", "get_deployed", map[string]interface{}{
		"context_repo":    "git@github.com:org/billing-service.git",
		"include_history": true,
	})
	if res.IsError {
		t.Fatalf("get_deployed returned error: %s", res.Content[0].Text)
	}
	var deployedOut struct {
		Count    int                   `json:"count"`
		Services []deployedServiceView `json:"services"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &deployedOut); err != nil {
		t.Fatalf("decode get_deployed: %v", err)
	}
	if deployedOut.Count != 1 || deployedOut.Services[0].Service != "billing-service" {
		t.Fatalf("unexpected get_deployed output: %+v", deployedOut)
	}
	stagingView := deployedOut.Services[0].Environments["staging"]
	if stagingView.ActiveVersion != "v1.1.0" || len(stagingView.RecentEvents) != 2 {
		t.Fatalf("unexpected staging view: %+v", stagingView)
	}
	if stagingView.ClusterStatus == nil || stagingView.ClusterStatus.Status != "progressing" {
		t.Fatalf("expected progressing cluster status, got %+v", stagingView.ClusterStatus)
	}
	if deployedOut.Services[0].Environments["production"].PendingPR == nil || deployedOut.Services[0].Environments["production"].PendingPR.PRNumber != 44 {
		t.Fatalf("expected pending PR #44 in production")
	}

	// 2. get_deploy_queue with only_queued=true across all services (auth-service is up to date so omitted)
	qRes := callToolHTTP(t, srv, "", "get_deploy_queue", map[string]interface{}{
		"only_queued": true,
	})
	if qRes.IsError {
		t.Fatalf("get_deploy_queue returned error: %s", qRes.Content[0].Text)
	}
	var queueOut struct {
		TotalServices        int                `json:"total_services"`
		TotalPendingPRs      int                `json:"total_pending_prs"`
		ServicesBehindLatest int                `json:"services_behind_latest"`
		Services             []queueServiceView `json:"services"`
	}
	if err := json.Unmarshal([]byte(qRes.Content[0].Text), &queueOut); err != nil {
		t.Fatalf("decode get_deploy_queue: %v", err)
	}
	if queueOut.TotalServices != 1 || queueOut.TotalPendingPRs != 1 || queueOut.ServicesBehindLatest != 1 {
		t.Fatalf("unexpected queue summary: %+v", queueOut)
	}
	prodQ := queueOut.Services[0].Environments["production"]
	if prodQ.VersionsBehind != 3 || prodQ.PendingPR == nil || prodQ.PendingPR.PRNumber != 44 {
		t.Fatalf("unexpected production queue: %+v", prodQ)
	}
	if len(prodQ.QueuedVersions) != 3 || prodQ.QueuedVersions[0].Version != "v1.3.0" {
		t.Fatalf("expected v1.3.0..v1.1.0 queued, got %+v", prodQ.QueuedVersions)
	}

	// 3. get_deploy_queue filtered by context_repo (HTTPS format matching SSH config) and environment
	qAuth := callToolHTTP(t, srv, "", "get_deploy_queue", map[string]interface{}{
		"context_repo": "https://github.com/org/auth-service.git",
		"environment":  "production",
	})
	if qAuth.IsError {
		t.Fatalf("get_deploy_queue auth-service failed: %s", qAuth.Content[0].Text)
	}
	var authQOut struct {
		Services []queueServiceView `json:"services"`
	}
	if err := json.Unmarshal([]byte(qAuth.Content[0].Text), &authQOut); err != nil {
		t.Fatalf("decode auth queue: %v", err)
	}
	if len(authQOut.Services) != 1 || !authQOut.Services[0].Environments["production"].IsUpToDate {
		t.Fatalf("expected auth-service up to date, got %+v", authQOut)
	}
}

func TestGetHistoryTool(t *testing.T) {
	cfg := &config.Config{
		Version:             "1",
		EnvironmentPriority: []string{"staging", "production"},
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				DisplayName: "Billing Service",
				Context:     config.ContextConfig{Repo: "https://github.com/org/billing-service.git"},
				Environments: map[string]*config.EnvironmentConfig{
					"staging":    {Branch: "main"},
					"production": {Branch: "main"},
				},
			},
		},
	}

	memStore := store.NewMemoryStore()
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 10, 14, 0, 0, 0, time.UTC)
	t4 := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)

	memStore.SetEvents("billing-service", "staging", []store.DeploymentEvent{
		{Service: "billing-service", Environment: "staging", Version: "v1.3.0", DeployedAt: t4, IsActive: true},
		{Service: "billing-service", Environment: "staging", Version: "v1.2.0", DeployedAt: t3},
		{Service: "billing-service", Environment: "staging", Version: "v1.1.0", DeployedAt: t1},
	})
	memStore.SetEvents("billing-service", "production", []store.DeploymentEvent{
		{Service: "billing-service", Environment: "production", Version: "v1.1.0", DeployedAt: t2, IsActive: true},
	})

	srv := NewServer(Deps{Config: cfg, Store: memStore})

	type historyResp struct {
		Service     string                  `json:"service"`
		Environment string                  `json:"environment"`
		Total       int                     `json:"total"`
		Offset      int                     `json:"offset"`
		Limit       int                     `json:"limit"`
		HasMore     bool                    `json:"has_more"`
		Events      []store.DeploymentEvent `json:"events"`
	}

	// 1. Paginated across all environments by context_repo (newest-first: t4, t3, t2, t1)
	res1 := callToolHTTP(t, srv, "", "get_history", map[string]interface{}{
		"context_repo": "org/billing-service",
		"limit":        2,
		"offset":       1,
	})
	if res1.IsError {
		t.Fatalf("get_history failed: %s", res1.Content[0].Text)
	}
	var out1 historyResp
	if err := json.Unmarshal([]byte(res1.Content[0].Text), &out1); err != nil {
		t.Fatalf("decode get_history: %v", err)
	}
	if out1.Total != 4 || out1.Offset != 1 || out1.Limit != 2 || !out1.HasMore || len(out1.Events) != 2 {
		t.Fatalf("unexpected pagination output: %+v", out1)
	}
	if out1.Events[0].Version != "v1.2.0" || out1.Events[1].Environment != "production" {
		t.Fatalf("unexpected event order: %+v", out1.Events)
	}

	// 2. Date range + environment filter (YYYY-MM-DD inclusive bounds)
	res2 := callToolHTTP(t, srv, "", "get_history", map[string]interface{}{
		"service":     "billing-service",
		"environment": "staging",
		"since":       "2026-11-09",
		"until":       "2026-11-11",
	})
	if res2.IsError {
		t.Fatalf("get_history date filter failed: %s", res2.Content[0].Text)
	}
	var out2 historyResp
	if err := json.Unmarshal([]byte(res2.Content[0].Text), &out2); err != nil {
		t.Fatalf("decode get_history date filter: %v", err)
	}
	if out2.Total != 1 || out2.HasMore || len(out2.Events) != 1 || out2.Events[0].Version != "v1.2.0" {
		t.Fatalf("unexpected date-filtered output: %+v", out2)
	}

	// 3. Invalid since/until order returns tool error
	resErr := callToolHTTP(t, srv, "", "get_history", map[string]interface{}{
		"service": "billing-service",
		"since":   "2026-11-12T00:00:00Z",
		"until":   "2026-11-10T00:00:00Z",
	})
	if !resErr.IsError {
		t.Fatalf("expected error when since > until")
	}
}

func TestPromoteToolEndToEnd(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GIT_TOKEN", "")

	tmpDir := t.TempDir()

	// Create bare GitOps remote and seed initial manifests
	gitopsBare := filepath.Join(tmpDir, "gitops-origin.git")
	runGitCmd(t, tmpDir, "init", "--bare", "-b", "main", gitopsBare)

	gitopsSeed := filepath.Join(tmpDir, "gitops-seed")
	runGitCmd(t, tmpDir, "clone", gitopsBare, gitopsSeed)
	if err := os.MkdirAll(filepath.Join(gitopsSeed, "apps/staging/billing"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitopsSeed, "apps/prod/billing"), 0755); err != nil {
		t.Fatal(err)
	}
	stagingFile := filepath.Join(gitopsSeed, "apps/staging/billing/values.yaml")
	prodFile := filepath.Join(gitopsSeed, "apps/prod/billing/values.yaml")
	_ = os.WriteFile(stagingFile, []byte("image:\n  repository: ghcr.io/org/billing-service\n  tag: v1.0.0\n"), 0644)
	_ = os.WriteFile(prodFile, []byte("image:\n  repository: ghcr.io/org/billing-service\n  tag: v1.0.0\n"), 0644)
	runGitCmd(t, gitopsSeed, "add", ".")
	runGitCmd(t, gitopsSeed, "commit", "-m", "chore: initial v1.0.0")
	runGitCmd(t, gitopsSeed, "push", "-u", "origin", "main")

	// Create source context repo with tagged releases v1.0.0, v1.1.0, v1.2.0
	srcRepo := filepath.Join(tmpDir, "billing-service-src")
	if err := os.MkdirAll(srcRepo, 0755); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, srcRepo, "init", "-b", "main")
	_ = os.WriteFile(filepath.Join(srcRepo, "CHANGELOG.md"), []byte("v1.0.0\n"), 0644)
	runGitCmd(t, srcRepo, "add", ".")
	runGitCmd(t, srcRepo, "commit", "-m", "feat: initial release")
	runGitCmd(t, srcRepo, "tag", "v1.0.0")

	_ = os.WriteFile(filepath.Join(srcRepo, "CHANGELOG.md"), []byte("v1.1.0\n"), 0644)
	runGitCmd(t, srcRepo, "add", ".")
	runGitCmd(t, srcRepo, "commit", "-m", "feat: add invoice export (#10)")
	runGitCmd(t, srcRepo, "tag", "v1.1.0")

	_ = os.WriteFile(filepath.Join(srcRepo, "CHANGELOG.md"), []byte("v1.2.0\n"), 0644)
	runGitCmd(t, srcRepo, "add", ".")
	runGitCmd(t, srcRepo, "commit", "-m", "fix: idempotent webhook retry (#11)")
	runGitCmd(t, srcRepo, "tag", "v1.2.0")

	cfg := &config.Config{
		Version: "1",
		GitOps: config.GitOpsConfig{
			Repo:   gitopsBare,
			Branch: "main",
		},
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				DisplayName: "Billing Service",
				Source: config.SourceConfig{
					Type:       "git_tags",
					Repository: srcRepo,
					Filter:     `^v[0-9]+\.[0-9]+\.[0-9]+$`,
				},
				Context: config.ContextConfig{
					Repo: srcRepo,
				},
				Permissions: &config.ServicePermissions{
					GitHubTeams: []string{"org/billing-team"},
				},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {
						Repo:   gitopsBare,
						Branch: "main",
						Target: config.TargetConfig{
							File: "apps/staging/billing/values.yaml",
							Path: "$.image.tag",
						},
						WriteStrategy: config.WriteStrategy{
							Mode:          "direct",
							CommitMessage: "chore(staging): promote {{ .Service }} to {{ .TargetVersion }}",
						},
					},
					"production": {
						Repo:   gitopsBare,
						Branch: "main",
						Target: config.TargetConfig{
							File: "apps/prod/billing/values.yaml",
							Path: "$.image.tag",
						},
						WriteStrategy: config.WriteStrategy{
							Mode: "pull_request",
						},
					},
				},
			},
		},
	}

	memStore := store.NewMemoryStore()
	scratchDir := filepath.Join(tmpDir, "scratch")
	idx, err := indexer.NewIndexer(cfg, memStore, filepath.Join(scratchDir, "clones"))
	if err != nil {
		t.Fatalf("NewIndexer failed: %v", err)
	}
	if err := idx.ReindexAll(context.Background()); err != nil {
		t.Fatalf("ReindexAll failed: %v", err)
	}

	dep := deployer.NewManager(cfg, memStore, idx, filepath.Join(scratchDir, "promotions"))
	authMgr := auth.NewAuthManager(&config.AuthConfig{DevMode: true})

	unauthToken := "tok_unauth_viewer"
	authMgr.CacheUserToken(unauthToken, &auth.User{
		ID:       "user:viewer",
		Username: "viewer",
		Name:     "Viewer",
		Teams:    []string{"org/other-team"},
	}, time.Hour)

	billingToken := "tok_billing_lead"
	authMgr.CacheUserToken(billingToken, &auth.User{
		ID:       "user:sarah",
		Username: "sarah-chen",
		Name:     "Sarah Chen",
		Email:    "sarah@example.com",
		Teams:    []string{"org/billing-team"},
	}, time.Hour)

	srv := NewServer(Deps{
		Config:   cfg,
		Store:    memStore,
		Indexer:  idx,
		Deployer: dep,
		Auth:     authMgr,
	})

	// 1. Unauthenticated promote -> Forbidden
	unauthRes := callToolHTTP(t, srv, "", "promote", map[string]interface{}{
		"service":        "billing-service",
		"environment":    "staging",
		"target_version": "v1.1.0",
	})
	if !unauthRes.IsError || !strings.Contains(unauthRes.Content[0].Text, "Forbidden") {
		t.Fatalf("expected Forbidden for unauthenticated promote, got %+v", unauthRes)
	}

	// 2. Unauthorized team member -> Forbidden
	wrongTeamRes := callToolHTTP(t, srv, unauthToken, "promote", map[string]interface{}{
		"service":        "billing-service",
		"environment":    "staging",
		"target_version": "v1.1.0",
	})
	if !wrongTeamRes.IsError || !strings.Contains(wrongTeamRes.Content[0].Text, "Forbidden") {
		t.Fatalf("expected Forbidden for wrong team, got %+v", wrongTeamRes)
	}

	// 3. Direct promote by service name to v1.1.0
	res1 := callToolHTTP(t, srv, billingToken, "promote", map[string]interface{}{
		"service":        "billing-service",
		"environment":    "staging",
		"target_version": "v1.1.0",
		"comment":        "agent staging rollout",
	})
	if res1.IsError {
		t.Fatalf("promote v1.1.0 failed: %s", res1.Content[0].Text)
	}
	var promoOut1 struct {
		Service   string                   `json:"service"`
		Promotion deployer.PromotionResult `json:"promotion"`
	}
	if err := json.Unmarshal([]byte(res1.Content[0].Text), &promoOut1); err != nil {
		t.Fatalf("unmarshal promote result: %v", err)
	}
	if !promoOut1.Promotion.Success || promoOut1.Promotion.NewVersion != "v1.1.0" || promoOut1.Promotion.Mode != "direct" {
		t.Fatalf("unexpected promotion result: %+v", promoOut1)
	}

	// Confirm gitops bare repo has v1.1.0 committed by Sarah Chen
	yamlOut := runGitCmd(t, gitopsBare, "show", "main:apps/staging/billing/values.yaml")
	if !strings.Contains(yamlOut, "tag: v1.1.0") {
		t.Fatalf("expected tag: v1.1.0 in values.yaml, got:\n%s", yamlOut)
	}
	authorOut := runGitCmd(t, gitopsBare, "log", "-1", "--format=%an <%ae>", "main")
	if strings.TrimSpace(authorOut) != "Sarah Chen <sarah@example.com>" {
		t.Fatalf("expected author Sarah Chen, got %q", authorOut)
	}

	// Confirm get_deployed immediately reflects v1.1.0
	depAfter1 := callToolHTTP(t, srv, billingToken, "get_deployed", map[string]interface{}{
		"context_repo": srcRepo,
	})
	var depOut1 struct {
		Services []deployedServiceView `json:"services"`
	}
	_ = json.Unmarshal([]byte(depAfter1.Content[0].Text), &depOut1)
	if depOut1.Services[0].Environments["staging"].ActiveVersion != "v1.1.0" {
		t.Fatalf("expected staging active v1.1.0, got %s", depOut1.Services[0].Environments["staging"].ActiveVersion)
	}

	// 4. get_version_diff chosen by context_repo comparing staging active (v1.1.0) to latest (v1.2.0)
	diffRes := callToolHTTP(t, srv, billingToken, "get_version_diff", map[string]interface{}{
		"context_repo": srcRepo,
		"environment":  "staging",
	})
	if diffRes.IsError {
		t.Fatalf("get_version_diff failed: %s", diffRes.Content[0].Text)
	}
	if !strings.Contains(diffRes.Content[0].Text, "idempotent webhook retry (#11)") {
		t.Fatalf("expected v1.2.0 commit in diff, got %s", diffRes.Content[0].Text)
	}

	// 5. Promote "latest" to staging chosen by context_repo
	res2 := callToolHTTP(t, srv, billingToken, "promote", map[string]interface{}{
		"context_repo":   srcRepo,
		"environment":    "staging",
		"target_version": "latest",
	})
	if res2.IsError {
		t.Fatalf("promote latest failed: %s", res2.Content[0].Text)
	}
	var promoOut2 struct {
		Promotion deployer.PromotionResult `json:"promotion"`
	}
	_ = json.Unmarshal([]byte(res2.Content[0].Text), &promoOut2)
	if promoOut2.Promotion.NewVersion != "v1.2.0" {
		t.Fatalf("expected latest to resolve to v1.2.0, got %s", promoOut2.Promotion.NewVersion)
	}

	// 6. Promote to production (pull_request mode) by context_repo -> pending_pr visible in get_deploy_queue
	res3 := callToolHTTP(t, srv, billingToken, "promote", map[string]interface{}{
		"context_repo":   srcRepo,
		"environment":    "production",
		"target_version": "v1.2.0",
	})
	if res3.IsError {
		t.Fatalf("promote production PR failed: %s", res3.Content[0].Text)
	}
	var promoOut3 struct {
		Promotion deployer.PromotionResult `json:"promotion"`
	}
	_ = json.Unmarshal([]byte(res3.Content[0].Text), &promoOut3)
	if promoOut3.Promotion.Mode != "pull_request" || promoOut3.Promotion.PRNumber < 1 {
		t.Fatalf("expected pull_request result with PR number, got %+v", promoOut3.Promotion)
	}

	qRes := callToolHTTP(t, srv, billingToken, "get_deploy_queue", map[string]interface{}{
		"context_repo": srcRepo,
		"environment":  "production",
	})
	var qOut struct {
		TotalPendingPRs int                `json:"total_pending_prs"`
		Services        []queueServiceView `json:"services"`
	}
	_ = json.Unmarshal([]byte(qRes.Content[0].Text), &qOut)
	if qOut.TotalPendingPRs != 1 || qOut.Services[0].Environments["production"].PendingPR == nil {
		t.Fatalf("expected pending PR in production queue, got %+v", qOut)
	}
}

func callToolHTTP(t *testing.T, srv *Server, token string, name string, args map[string]interface{}) CallToolResult {
	t.Helper()
	payload, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call %s HTTP status = %d: %s", name, rec.Code, rec.Body.String())
	}
	var rpcResp struct {
		Result CallToolResult `json:"result"`
		Error  *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rpcResp); err != nil {
		t.Fatalf("unmarshal tools/call response: %v (%s)", err, rec.Body.String())
	}
	if rpcResp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error for %s: %+v", name, rpcResp.Error)
	}
	return rpcResp.Result
}
