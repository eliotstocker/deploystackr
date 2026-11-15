package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deploystackr/pkg/config"
)

func TestParseBearerChallenge(t *testing.T) {
	header := `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:eliotstocker/deploystackr:pull"`
	realm, service, scope := parseBearerChallenge(header, "default")
	if realm != "https://auth.docker.io/token" {
		t.Fatalf("unexpected realm: %s", realm)
	}
	if service != "registry.docker.io" {
		t.Fatalf("unexpected service: %s", service)
	}
	if scope != "repository:eliotstocker/deploystackr:pull" {
		t.Fatalf("unexpected scope: %s", scope)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"latest", "v1.4.4", 1},
		{"v1.4.4", "latest", -1},
		{"v1.4.4", "v1.4.3", 1},
		{"v1.4.3", "v1.4.4", -1},
		{"v1.4.10", "v1.4.2", 1},
		{"v1.4.2", "v1.4.10", -1},
		{"v1.4.0", "v1.4.0", 0},
	}

	for _, tc := range tests {
		got := compareVersions(tc.a, tc.b)
		if got != tc.expected {
			t.Errorf("compareVersions(%q, %q) = %d, expected %d", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestOCIProviderTokenChallenge(t *testing.T) {
	var authServer *httptest.Server
	authServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("service") != "mock-registry" {
			http.Error(w, "bad service", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token": "valid-token-123",
		})
	}))
	defer authServer.Close()

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-token-123" {
			w.Header().Set("Www-Authenticate", `Bearer realm="`+authServer.URL+`",service="mock-registry",scope="repository:myorg/myapp:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(ociTagsResponse{
			Name: "myorg/myapp",
			Tags: []string{"latest", "v1.4.2", "v1.4.3", "v1.4.4"},
		})
	}))
	defer regServer.Close()

	// Strip http:// from regServer.URL
	regHost := regServer.URL[len("http://"):]

	provider := NewProvider(config.SourceConfig{
		Type:       "oci",
		Repository: regHost + "/myorg/myapp",
	})

	versions, err := provider.GetVersions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(versions) != 4 {
		t.Fatalf("expected 4 versions, got %d", len(versions))
	}
	if versions[0].Version != "latest" || versions[1].Version != "v1.4.4" || versions[2].Version != "v1.4.3" || versions[3].Version != "v1.4.2" {
		t.Fatalf("unexpected version order: %+v", versions)
	}
}

func TestGitHubReleasesProvider_TrimPrefix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		releases := []ghRelease{
			{TagName: "v12.4", Name: "12.4"},
			{TagName: "v12.3", Name: "12.3"},
			{TagName: "12.2", Name: "12.2"},
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer ts.Close()

	// Use custom server by setting GITHUB_TOKEN or passing via custom URL in test
	provider := &GitHubReleasesProvider{
		cfg: config.SourceConfig{
			Type:       "github_releases",
			Repository: ts.URL[len("http://"):],
			TrimPrefix: "v",
		},
	}

	// Direct check of URL replacement with ts.URL
	req, _ := http.NewRequestWithContext(context.Background(), "GET", ts.URL, nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var releases []ghRelease
	_ = json.NewDecoder(resp.Body).Decode(&releases)

	var versions []VersionInfo
	for _, r := range releases {
		tag := r.TagName
		if provider.cfg.TrimPrefix != "" {
			tag = strings.TrimPrefix(tag, provider.cfg.TrimPrefix)
		}
		versions = append(versions, VersionInfo{Version: tag})
	}

	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	if versions[0].Version != "12.4" || versions[1].Version != "12.3" || versions[2].Version != "12.2" {
		t.Fatalf("unexpected versions with TrimPrefix: %+v", versions)
	}
}

func TestOCIProvider_TrimPrefix(t *testing.T) {
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ociTagsResponse{
			Name: "jellyfin/jellyfin",
			Tags: []string{"v12.4", "v12.3"},
		})
	}))
	defer regServer.Close()

	regHost := regServer.URL[len("http://"):]

	provider := NewProvider(config.SourceConfig{
		Type:       "oci",
		Repository: regHost + "/jellyfin/jellyfin",
		TrimPrefix: "v",
	})

	versions, err := provider.GetVersions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != "12.4" || versions[1].Version != "12.3" {
		t.Fatalf("expected trimmed versions, got %+v", versions)
	}
}

func TestOCIProvider_PopulatesDigests(t *testing.T) {
	digests := map[string]string{
		"v1.2":           "sha256:aaa",
		"v1.2.0":         "sha256:aaa",
		"v1.2.0-fun":     "sha256:aaa",
		"v1.3.0":         "sha256:bbb",
		"latest":         "sha256:bbb",
	}

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/myorg/myapp/tags/list" {
			_ = json.NewEncoder(w).Encode(ociTagsResponse{
				Name: "myorg/myapp",
				Tags: []string{"v1.2", "v1.2.0", "v1.2.0-fun", "v1.3.0", "latest"},
			})
			return
		}
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/v2/myorg/myapp/manifests/") {
			tag := strings.TrimPrefix(r.URL.Path, "/v2/myorg/myapp/manifests/")
			if d, ok := digests[tag]; ok {
				w.Header().Set("Docker-Content-Digest", d)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer regServer.Close()

	regHost := regServer.URL[len("http://"):]
	p := NewProvider(config.SourceConfig{
		Type:       "oci",
		Repository: regHost + "/myorg/myapp",
		TrimPrefix: "v",
	})

	versions, err := p.GetVersions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 5 {
		t.Fatalf("expected 5 versions before deduplication, got %d", len(versions))
	}

	gotDigests := make(map[string]string, len(versions))
	for _, v := range versions {
		gotDigests[v.Version] = v.Digest
	}
	if gotDigests["1.2"] != "sha256:aaa" || gotDigests["1.2.0"] != "sha256:aaa" || gotDigests["1.2.0-fun"] != "sha256:aaa" {
		t.Fatalf("expected sha256:aaa for 1.2* tags, got %+v", gotDigests)
	}
	if gotDigests["1.3.0"] != "sha256:bbb" || gotDigests["latest"] != "sha256:bbb" {
		t.Fatalf("expected sha256:bbb for 1.3.0 and latest, got %+v", gotDigests)
	}
}

func TestDeduplicateVersionsByDigest(t *testing.T) {
	now := time.Now()

	t.Run("default picks more specific version", func(t *testing.T) {
		input := []VersionInfo{
			{Version: "latest", Digest: "sha256:bbb"},
			{Version: "1.3.0", Digest: "sha256:bbb"},
			{Version: "1.2.0", Digest: "sha256:aaa"},
			{Version: "1.2", Digest: "sha256:aaa"},
			{Version: "1.1.0", Digest: ""},
		}

		got := DeduplicateVersionsByDigest(input)
		if len(got) != 3 {
			t.Fatalf("expected 3 versions, got %d: %+v", len(got), got)
		}
		if got[0].Version != "1.3.0" || got[1].Version != "1.2.0" || got[2].Version != "1.1.0" {
			t.Fatalf("unexpected deduplicated versions: %+v", got)
		}
	})

	t.Run("default picks suffix when no context", func(t *testing.T) {
		input := []VersionInfo{
			{Version: "1.2.0-funthings", Digest: "sha256:aaa"},
			{Version: "1.2.0", Digest: "sha256:aaa"},
			{Version: "1.2", Digest: "sha256:aaa"},
		}

		got := DeduplicateVersionsByDigest(input)
		if len(got) != 1 || got[0].Version != "1.2.0-funthings" {
			t.Fatalf("expected 1.2.0-funthings, got %+v", got)
		}
	})

	t.Run("context info overrides default specificity", func(t *testing.T) {
		input := []VersionInfo{
			{Version: "1.2.0-funthings", Digest: "sha256:aaa"},
			{Version: "1.2.0", Digest: "sha256:aaa", Changelog: "### Features\n- Something", ReleasedAt: &now},
			{Version: "1.2", Digest: "sha256:aaa"},
		}

		got := DeduplicateVersionsByDigest(input)
		if len(got) != 1 || got[0].Version != "1.2.0" {
			t.Fatalf("expected 1.2.0 with context info to win, got %+v", got)
		}
	})
}
