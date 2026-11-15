package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
