package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"deploystackr/pkg/store"
)

func TestHandleBadge_SuccessAndEnvResolution(t *testing.T) {
	srv, memStore := setupTestServer(t)

	// Seed production active event and upstream versions so production is up-to-date
	memStore.SetEvents("billing-service", "production", []store.DeploymentEvent{
		{
			Service:     "billing-service",
			Environment: "production",
			Version:     "ghcr.io/org/billing:v1.5.0",
			IsActive:    true,
		},
	})
	memStore.SetVersions("billing-service", []string{"v1.5.0", "v1.4.0", "v1.3.0"})

	req := httptest.NewRequest(http.MethodGet, "http://deploystackr.local/badge/billing-service.svg", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("expected image/svg+xml content type, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Billing Service · production") {
		t.Errorf("expected display name and production env in SVG, got: %s", body)
	}
	if !strings.Contains(body, "v1.5.0") {
		t.Errorf("expected shortened version v1.5.0 in SVG, got: %s", body)
	}
	if !strings.Contains(body, `fill="#10b981"`) {
		t.Errorf("expected green up-to-date dot in SVG, got: %s", body)
	}
	if !strings.Contains(body, `href="http://deploystackr.local/billing-service"`) {
		t.Errorf("expected link to service page in SVG, got: %s", body)
	}
}

func TestHandleBadge_BehindAndThemeAndCustomTitle(t *testing.T) {
	srv, memStore := setupTestServer(t)
	memStore.SetVersions("billing-service", []string{"v1.6.0", "v1.5.0", "v1.4.0"})

	req := httptest.NewRequest(http.MethodGet, "http://deploystackr.local/badge/billing-service?env=staging&theme=dark&title=Billing+%26+Pay", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Billing &amp; Pay · staging") {
		t.Errorf("expected XML-escaped custom title and staging env, got: %s", body)
	}
	if !strings.Contains(body, "v1.4.0 · 2 behind") {
		t.Errorf("expected 'v1.4.0 · 2 behind', got: %s", body)
	}
	if !strings.Contains(body, `fill="#f59e0b"`) {
		t.Errorf("expected amber behind dot, got: %s", body)
	}
	if !strings.Contains(body, "#0f172a") {
		t.Errorf("expected dark theme background, got: %s", body)
	}
}

func TestHandleBadge_PendingPR(t *testing.T) {
	srv, memStore := setupTestServer(t)
	memStore.SetPendingPR("billing-service", "staging", &store.PendingPR{
		PRNumber:      42,
		TargetVersion: "v1.5.0",
	})

	req := httptest.NewRequest(http.MethodGet, "/badge/billing-service.svg?env=staging", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "v1.4.0 · PR #42") {
		t.Errorf("expected pending PR indicator, got: %s", body)
	}
	if !strings.Contains(body, `fill="#3b82f6"`) {
		t.Errorf("expected blue pending PR dot, got: %s", body)
	}
}

func TestHandleBadge_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/badge/nonexistent.svg", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("expected image/svg+xml content type on 404, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Errorf("expected 'not found' in 404 SVG body, got: %s", rec.Body.String())
	}
}
