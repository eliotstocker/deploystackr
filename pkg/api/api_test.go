package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"deploystackr/pkg/auth"
	"deploystackr/pkg/cluster"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/store"
)

func setupTestServer(t *testing.T) (*Server, *store.MemoryStore) {
	cfg := &config.Config{
		Version: "1",
		GitOps: config.GitOpsConfig{
			Repo:   "https://github.com/org/gitops.git",
			Branch: "main",
		},
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				DisplayName: "Billing Service",
				Source: config.SourceConfig{
					Type:       "git_tags",
					Repository: "https://github.com/org/billing-service.git",
					Filter:     "^v[0-9]+",
				},
				Context: config.ContextConfig{
					Repo: "https://github.com/org/billing-service.git",
				},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {
						Branch: "main",
						Target: config.TargetConfig{
							File: "apps/staging/billing/values.yaml",
							Path: "$.image.tag",
						},
						WriteStrategy: config.WriteStrategy{
							Mode:          "direct",
							CommitMessage: "chore: promote to {{ .TargetVersion }}",
						},
					},
					"production": {
						Branch: "main",
						Target: config.TargetConfig{
							File: "apps/prod/billing/deployment.json",
							Path: "$.spec.template.spec.containers[0].image",
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

	// Seed store with initial events
	now := time.Now()
	memStore.SetEvents("billing-service", "staging", []store.DeploymentEvent{
		{
			Service:        "billing-service",
			Environment:    "staging",
			Version:        "v1.4.0",
			CommitSHA:      "a1b2c3d4e5f6",
			CommitMessage:  "chore(staging): promote billing-service to v1.4.0 (#102)",
			AuthorName:     "Alice Smith",
			AuthorEmail:    "alice@example.com",
			PRNumber:       102,
			DeployedAt:     now.Add(-2 * time.Hour),
			IsActive:       true,
			ActiveDuration: "2h",
		},
		{
			Service:        "billing-service",
			Environment:    "staging",
			Version:        "v1.3.0",
			CommitSHA:      "f6e5d4c3b2a1",
			CommitMessage:  "chore(staging): promote billing-service to v1.3.0 (#98)",
			AuthorName:     "Bob Jones",
			AuthorEmail:    "bob@example.com",
			PRNumber:       98,
			DeployedAt:     now.Add(-72 * time.Hour),
			RetiredAt:      &now,
			IsActive:       false,
			ActiveDuration: "2d 22h",
		},
	})

	scratchDir := filepath.Join(os.TempDir(), "deploystackr_test")
	idx, err := indexer.NewIndexer(cfg, memStore, scratchDir)
	if err != nil {
		t.Fatalf("NewIndexer failed: %v", err)
	}

	dep := deployer.NewManager(cfg, memStore, idx, scratchDir)
	srv := NewServer(cfg, memStore, idx, dep, nil)

	return srv, memStore
}

func TestGetServicesEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/services", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var services []ServiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &services); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}

	if services[0].Service != "billing-service" {
		t.Fatalf("expected billing-service, got %s", services[0].Service)
	}

	if services[0].DisplayName != "Billing Service" {
		t.Fatalf("expected DisplayName 'Billing Service', got %s", services[0].DisplayName)
	}

	stagingEnv, exists := services[0].Environments["staging"]
	if !exists {
		t.Fatalf("staging environment missing")
	}

	if stagingEnv.ActiveVersion != "v1.4.0" {
		t.Fatalf("expected active version v1.4.0, got %s", stagingEnv.ActiveVersion)
	}
	if stagingEnv.PRNumber != 102 {
		t.Fatalf("expected PR 102, got %d", stagingEnv.PRNumber)
	}
}

func TestGetServicesDeterministicOrder(t *testing.T) {
	srv, _ := setupTestServer(t)

	srv.cfg.Services["zeta"] = &config.ServiceConfig{DisplayName: "Zeta"}
	srv.cfg.Services["alpha"] = &config.ServiceConfig{DisplayName: "Alpha"}
	srv.cfg.Services["gamma"] = &config.ServiceConfig{DisplayName: "Gamma"}
	srv.cfg.ServiceOrder = []string{"zeta", "billing-service", "gamma", "alpha"}

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/services", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		var services []ServiceResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &services); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		expected := []string{"zeta", "billing-service", "gamma", "alpha"}
		if len(services) != len(expected) {
			t.Fatalf("iteration %d: expected %d services, got %d", i, len(expected), len(services))
		}
		for j, want := range expected {
			if services[j].Service != want {
				t.Fatalf("iteration %d, index %d: expected %s, got %s", i, j, want, services[j].Service)
			}
		}
	}
}

func TestGetServiceHistoryEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/services/billing-service/history", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var history map[string][]store.DeploymentEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	stagingEvts, exists := history["staging"]
	if !exists || len(stagingEvts) != 2 {
		t.Fatalf("expected 2 staging events, got %v", stagingEvts)
	}

	if stagingEvts[0].Version != "v1.4.0" || !stagingEvts[0].IsActive {
		t.Errorf("expected active v1.4.0, got %+v", stagingEvts[0])
	}
}

func TestGetVersionsEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/services/billing-service/versions", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var versions []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &versions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(versions) == 0 {
		t.Fatalf("expected candidate versions")
	}
}

func TestGetDiffEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/services/billing-service/diff?from=v1.3.0&to=v1.4.0", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &diff); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if diff["from_version"] != "v1.3.0" || diff["to_version"] != "v1.4.0" {
		t.Fatalf("unexpected diff range: %v", diff)
	}
}

func TestGetDiffEndpoint_MissingContext(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Add service without context repo configured
	srv.cfg.Services["no-context-service"] = &config.ServiceConfig{
		DisplayName: "No Context Service",
		Source: config.SourceConfig{
			Type:       "git_tags",
			Repository: "https://github.com/org/no-context.git",
		},
		Context: config.ContextConfig{
			Repo: "",
		},
		Environments: map[string]*config.EnvironmentConfig{
			"staging": {
				Branch: "main",
			},
		},
	}

	req := httptest.NewRequest("GET", "/api/services/no-context-service/diff?from=v1.0.0&to=v2.0.0", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &diff); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if diff["from_version"] != "v1.0.0" || diff["to_version"] != "v2.0.0" {
		t.Fatalf("unexpected diff range: %v", diff)
	}

	commits, ok := diff["commits"].([]interface{})
	if !ok || len(commits) != 0 {
		t.Fatalf("expected empty commits when context repo is missing, got %v", commits)
	}
}

func TestWebhookEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/webhook", bytes.NewBufferString(`{"ref":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestWebappServing(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!doctype html><html><body>DeployStackr Portal</body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
	}

	cfg := &config.Config{Version: "1"}
	memStore := store.NewMemoryStore()
	scratchDir := filepath.Join(os.TempDir(), "deploystackr_webapp_test")
	idx, _ := indexer.NewIndexer(cfg, memStore, scratchDir)
	dep := deployer.NewManager(cfg, memStore, idx, scratchDir)
	srv := NewServer(cfg, memStore, idx, dep, mockFS)

	// Test 1: root "/" serves index.html
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "DeployStackr Portal") {
		t.Fatalf("expected index.html content, got: %s", rec.Body.String())
	}

	// Test 2: client-side SPA routing (e.g. /timeline) falls back to index.html
	reqSpa := httptest.NewRequest("GET", "/timeline", nil)
	recSpa := httptest.NewRecorder()
	srv.ServeHTTP(recSpa, reqSpa)
	if recSpa.Code != http.StatusOK {
		t.Fatalf("expected 200 for /timeline SPA fallback, got %d", recSpa.Code)
	}
	if !strings.Contains(recSpa.Body.String(), "DeployStackr Portal") {
		t.Fatalf("expected index.html content for SPA fallback, got: %s", recSpa.Body.String())
	}

	// Test 3: static asset /assets/app.js
	reqAsset := httptest.NewRequest("GET", "/assets/app.js", nil)
	recAsset := httptest.NewRecorder()
	srv.ServeHTTP(recAsset, reqAsset)
	if recAsset.Code != http.StatusOK {
		t.Fatalf("expected 200 for /assets/app.js, got %d", recAsset.Code)
	}
}

func TestWebhookEndpoints(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Release webhook
	relBody := `{"service":"billing-service","version":"v1.9.0","description":"Hotfix release"}`
	reqRel := httptest.NewRequest("POST", "/api/webhooks/release", strings.NewReader(relBody))
	reqRel.Header.Set("Content-Type", "application/json")
	recRel := httptest.NewRecorder()
	srv.ServeHTTP(recRel, reqRel)

	if recRel.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/webhooks/release, got %d: %s", recRel.Code, recRel.Body.String())
	}
	var relResp map[string]interface{}
	if err := json.Unmarshal(recRel.Body.Bytes(), &relResp); err != nil {
		t.Fatalf("failed to decode release response: %v", err)
	}
	if relResp["status"] != "ok" || relResp["version"] != "v1.9.0" {
		t.Errorf("unexpected release response: %+v", relResp)
	}

	// 2. PR merged webhook
	prBody := `{"service":"billing-service","environment":"production","pr_number":99,"commit_sha":"deadbeef"}`
	reqPR := httptest.NewRequest("POST", "/api/webhooks/pr", strings.NewReader(prBody))
	reqPR.Header.Set("Content-Type", "application/json")
	recPR := httptest.NewRecorder()
	srv.ServeHTTP(recPR, reqPR)

	if recPR.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/webhooks/pr, got %d: %s", recPR.Code, recPR.Body.String())
	}
	var prResp map[string]interface{}
	if err := json.Unmarshal(recPR.Body.Bytes(), &prResp); err != nil {
		t.Fatalf("failed to decode pr response: %v", err)
	}
	if prResp["status"] != "ok" {
		t.Errorf("unexpected pr response: %+v", prResp)
	}

	// 3. Generic multiplexer /api/webhook
	pushReq := httptest.NewRequest("POST", "/api/webhook", strings.NewReader(`{"ref":"refs/heads/main"}`))
	recPush := httptest.NewRecorder()
	srv.ServeHTTP(recPush, pushReq)
	if recPush.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/webhook push, got %d", recPush.Code)
	}

	// 4. Webhooks list /api/webhooks
	reqList := httptest.NewRequest("GET", "/api/webhooks", nil)
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/webhooks, got %d", recList.Code)
	}
	var listResp map[string]interface{}
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse webhooks list: %v", err)
	}
	if _, ok := listResp["history"]; !ok {
		t.Errorf("expected history field in /api/webhooks response")
	}
}

func TestClusterStatusEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"spec": { "replicas": 1 },
			"status": {
				"replicas": 1,
				"updatedReplicas": 1,
				"readyReplicas": 1,
				"availableReplicas": 1
			}
		}`))
	}))
	defer ts.Close()

	srv, _ := setupTestServer(t)
	srv.cfg.Cluster = &config.ClusterConfig{
		Enabled:          true,
		DefaultNamespace: "default",
		KubeAPIServer:    ts.URL,
		Token:            "test-token",
	}
	srv.clusterChecker = cluster.NewChecker(srv.cfg.Cluster)

	// 1. Check /api/services returns 200 without blocking on cluster status
	req := httptest.NewRequest("GET", "/api/services", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var services []ServiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &services); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(services) == 0 {
		t.Fatalf("expected at least 1 service")
	}

	// 2. Check dedicated /api/services/billing-service/status endpoint
	reqStatus := httptest.NewRequest("GET", "/api/services/billing-service/status", nil)
	recStatus := httptest.NewRecorder()
	srv.ServeHTTP(recStatus, reqStatus)

	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 from status subroute, got %d", recStatus.Code)
	}

	var statusMap map[string]*cluster.Status
	if err := json.Unmarshal(recStatus.Body.Bytes(), &statusMap); err != nil {
		t.Fatalf("unmarshal subroute error: %v", err)
	}
	if statusMap["staging"] == nil || statusMap["staging"].Status != "ready" {
		t.Errorf("expected staging status ready, got %+v", statusMap["staging"])
	}
}

func TestProgrammaticDeployment_AuthAndWatch(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Verify /api/auth/token endpoint is removed (token login from UI is disallowed)
	reqToken := httptest.NewRequest("POST", "/api/auth/token", bytes.NewReader([]byte(`{"token":"ghp_test"}`)))
	recToken := httptest.NewRecorder()
	srv.ServeHTTP(recToken, reqToken)
	if recToken.Code != http.StatusNotFound {
		t.Errorf("expected 404 for removed /api/auth/token endpoint, got %d", recToken.Code)
	}

	// 2. Setup mock cluster server for watch endpoint tests
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": 2,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name":  "billing",
								"image": "billing:v1.4.0",
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"replicas":        2,
				"readyReplicas":   2,
				"updatedReplicas": 2,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	srv.cfg.Cluster = &config.ClusterConfig{
		Enabled:          true,
		DefaultNamespace: "default",
		KubeAPIServer:    ts.URL,
		Token:            "test-token",
	}
	srv.clusterChecker = cluster.NewChecker(srv.cfg.Cluster)

	// 3. Unauthenticated watch request should be rejected with 403
	reqUnauth := httptest.NewRequest("GET", "/api/services/billing-service/watch?env=staging&version=v1.4.0&timeout=5", nil)
	recUnauth := httptest.NewRecorder()
	srv.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unauthenticated watch request, got %d", recUnauth.Code)
	}

	// 4. Register programmatic CI token
	ciToken := "ghp_mock_ci_token_12345"
	srv.authMgr.CacheUserToken(ciToken, &auth.User{
		ID:       "ci-bot",
		Username: "github-actions[bot]",
		Name:     "GitHub Actions",
		Provider: "github",
	}, time.Hour)

	// Verify Bearer token does NOT grant web UI session on /api/auth/me
	reqMe := httptest.NewRequest("GET", "/api/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+ciToken)
	recMe := httptest.NewRecorder()
	srv.ServeHTTP(recMe, reqMe)
	if recMe.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/auth/me, got %d", recMe.Code)
	}
	var meResp AuthMeResponse
	if err := json.Unmarshal(recMe.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("failed to decode meResp: %v", err)
	}
	if meResp.Authenticated {
		t.Errorf("expected Bearer token not to authenticate web session via /api/auth/me")
	}

	// 5. Test GET /api/services/billing-service/watch with CI token returns 200 with deployment status
	reqWatch := httptest.NewRequest("GET", "/api/services/billing-service/watch?env=staging&version=v1.4.0&timeout=5", nil)
	reqWatch.Header.Set("Authorization", "Bearer "+ciToken)
	recWatch := httptest.NewRecorder()
	srv.ServeHTTP(recWatch, reqWatch)

	if recWatch.Code != http.StatusOK {
		t.Fatalf("expected 200 from watch endpoint with Bearer token, got %d: %s", recWatch.Code, recWatch.Body.String())
	}

	var watchResp map[string]interface{}
	if err := json.Unmarshal(recWatch.Body.Bytes(), &watchResp); err != nil {
		t.Fatalf("failed to decode watch response: %v", err)
	}
	if watchResp["status"] != "ready" {
		t.Errorf("expected watch status ready, got %v", watchResp["status"])
	}

	// 6. Test GET /api/services/billing-service/watch with stream=true and CI token
	reqStream := httptest.NewRequest("GET", "/api/services/billing-service/watch?env=staging&version=v1.4.0&stream=true&timeout=5", nil)
	reqStream.Header.Set("Authorization", "Bearer "+ciToken)
	recStream := httptest.NewRecorder()
	srv.ServeHTTP(recStream, reqStream)

	if recStream.Code != http.StatusOK {
		t.Fatalf("expected 200 from stream watch, got %d", recStream.Code)
	}
	contentType := recStream.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("expected text/event-stream Content-Type, got %s", contentType)
	}
	if !strings.Contains(recStream.Body.String(), "data:") {
		t.Errorf("expected SSE data in stream response, got: %s", recStream.Body.String())
	}
}

func TestComputeTopEnvStatus_RepoPrefixMatching(t *testing.T) {
	memStore := store.NewMemoryStore()
	cachedVersions := []string{
		"v1.5.0", "v1.4.30", "v1.4.29", "v1.4.28", "v1.4.27",
		"v1.4.26", "v1.4.25", "v1.4.24", "v1.4.23", "v1.4.22",
	}
	memStore.SetVersions("deploystackr", cachedVersions)

	envViews := map[string]store.EnvironmentView{
		"pi": {
			ActiveVersion: "eliotstocker/deploystackr:v1.5.0",
		},
	}

	status := computeTopEnvStatus("deploystackr", &config.ServiceConfig{}, envViews, []string{"pi"}, memStore)
	if status == nil {
		t.Fatalf("expected non-nil status")
	}

	if !status.IsUpToDate {
		t.Errorf("expected IsUpToDate to be true for eliotstocker/deploystackr:v1.5.0 against latest v1.5.0")
	}
	if status.VersionsBehind != 0 {
		t.Errorf("expected 0 versions behind, got %d", status.VersionsBehind)
	}
	if status.Environment != "pi" {
		t.Errorf("expected environment 'pi', got %s", status.Environment)
	}
}

func TestComputeTopEnvStatus_NoCachedVersionsDoesNotFalselyClaimUpToDate(t *testing.T) {
	memStore := store.NewMemoryStore()
	envViews := map[string]store.EnvironmentView{
		"production": {
			ActiveVersion: "v1.4.0",
		},
		"staging": {
			ActiveVersion: "v1.5.0",
		},
	}

	status := computeTopEnvStatus("billing", &config.ServiceConfig{}, envViews, []string{"production", "staging"}, memStore)
	if status == nil {
		t.Fatalf("expected non-nil status")
	}
	if status.LatestVersion != "" {
		t.Errorf("expected empty LatestVersion when upstream versions are not cached, got %q", status.LatestVersion)
	}
	if status.IsUpToDate {
		t.Errorf("expected IsUpToDate to be false when upstream versions are not cached")
	}
	if status.VersionsBehind != 0 {
		t.Errorf("expected VersionsBehind to be 0 when upstream versions are not cached, got %d", status.VersionsBehind)
	}
}

func TestMCPEndpointOnAPIServer(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!doctype html><html><body>DeployStackr Portal</body></html>"),
		},
	}

	cfg := &config.Config{
		Version: "1",
		Services: map[string]*config.ServiceConfig{
			"billing-service": {
				Context: config.ContextConfig{Repo: "org/billing-service"},
				Environments: map[string]*config.EnvironmentConfig{
					"staging": {Branch: "main"},
				},
			},
		},
	}
	memStore := store.NewMemoryStore()
	memStore.SetEvents("billing-service", "staging", []store.DeploymentEvent{
		{Service: "billing-service", Environment: "staging", Version: "v1.4.0", IsActive: true},
	})
	scratchDir := t.TempDir()
	idx, _ := indexer.NewIndexer(cfg, memStore, scratchDir)
	dep := deployer.NewManager(cfg, memStore, idx, scratchDir)
	srv := NewServer(cfg, memStore, idx, dep, mockFS)

	// 1. CORS preflight OPTIONS /mcp
	optReq := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	optRec := httptest.NewRecorder()
	srv.ServeHTTP(optRec, optReq)
	if optRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on OPTIONS /mcp, got %d", optRec.Code)
	}
	if !strings.Contains(optRec.Header().Get("Access-Control-Allow-Headers"), "MCP-Protocol-Version") {
		t.Errorf("expected MCP headers in Access-Control-Allow-Headers, got %s", optRec.Header().Get("Access-Control-Allow-Headers"))
	}

	// 2. /mcp and /api/mcp both respond to initialize and tools/call
	for _, endpoint := range []string{"/mcp", "/api/mcp"} {
		initReq := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`))
		initRec := httptest.NewRecorder()
		srv.ServeHTTP(initRec, initReq)
		if initRec.Code != http.StatusOK || !strings.Contains(initRec.Body.String(), "deploystackr") {
			t.Fatalf("%s initialize failed: %d %s", endpoint, initRec.Code, initRec.Body.String())
		}

		callReq := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_deployed","arguments":{"context_repo":"org/billing-service"}}}`))
		callRec := httptest.NewRecorder()
		srv.ServeHTTP(callRec, callReq)
		if callRec.Code != http.StatusOK || !strings.Contains(callRec.Body.String(), "v1.4.0") {
			t.Fatalf("%s get_deployed failed: %d %s", endpoint, callRec.Code, callRec.Body.String())
		}
	}

	// 3. SPA fallback does not swallow /mcp/* subpaths
	notFoundReq := httptest.NewRequest(http.MethodGet, "/mcp/unknown", nil)
	notFoundRec := httptest.NewRecorder()
	srv.ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on /mcp/unknown, got %d", notFoundRec.Code)
	}
}




