package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deploystackr/pkg/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestParseReleaseRequest(t *testing.T) {
	// 1. Generic release payload
	genericJSON := `{"service":"billing-service","version":"v1.5.0","description":"New feature release"}`
	req := httptest.NewRequest("POST", "/api/webhooks/release", nil)
	payload, err := ParseReleaseRequest(req, []byte(genericJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing generic release: %v", err)
	}
	if payload.Service != "billing-service" || payload.Version != "v1.5.0" {
		t.Errorf("expected billing-service/v1.5.0, got %s/%s", payload.Service, payload.Version)
	}

	// 2. GitHub release payload
	ghJSON := `{
		"action": "published",
		"release": {
			"tag_name": "v2.0.0",
			"name": "Major update",
			"body": "Detailed changelog here"
		},
		"repository": {
			"name": "auth-service",
			"full_name": "org/auth-service"
		},
		"sender": {
			"login": "octocat"
		}
	}`
	reqGH := httptest.NewRequest("POST", "/api/webhook", nil)
	reqGH.Header.Set("X-GitHub-Event", "release")
	ghPayload, err := ParseReleaseRequest(reqGH, []byte(ghJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing GitHub release: %v", err)
	}
	if ghPayload.Service != "auth-service" || ghPayload.Version != "v2.0.0" {
		t.Errorf("expected auth-service/v2.0.0, got %s/%s", ghPayload.Service, ghPayload.Version)
	}
	if ghPayload.Sender != "octocat" {
		t.Errorf("expected sender octocat, got %s", ghPayload.Sender)
	}
}

func TestParsePRMergedRequest(t *testing.T) {
	// 1. Generic PR merged payload
	genericJSON := `{"event":"pr_merged","service":"billing-service","environment":"production","pr_number":42,"commit_sha":"c0ffee"}`
	req := httptest.NewRequest("POST", "/api/webhooks/pr", nil)
	payload, err := ParsePRMergedRequest(req, []byte(genericJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing generic PR: %v", err)
	}
	if payload.Service != "billing-service" || payload.PRNumber != 42 || payload.CommitSHA != "c0ffee" {
		t.Errorf("unexpected payload content: %+v", payload)
	}

	// 2. GitHub PR merged webhook
	ghJSON := `{
		"action": "closed",
		"pull_request": {
			"number": 105,
			"title": "Promote billing-service to v1.4.2",
			"merged": true,
			"merge_commit_sha": "abc1234",
			"base": { "ref": "main" },
			"head": { "ref": "deploy/billing-v1.4.2" },
			"merged_by": { "login": "devops-lead" }
		},
		"repository": {
			"name": "demo-gitops",
			"full_name": "org/demo-gitops"
		}
	}`
	reqGH := httptest.NewRequest("POST", "/api/webhook", nil)
	reqGH.Header.Set("X-GitHub-Event", "pull_request")
	ghPayload, err := ParsePRMergedRequest(reqGH, []byte(ghJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing GitHub PR: %v", err)
	}
	if ghPayload.PRNumber != 105 || ghPayload.CommitSHA != "abc1234" || ghPayload.MergedBy != "devops-lead" {
		t.Errorf("unexpected GitHub PR payload: %+v", ghPayload)
	}
}

func TestDispatcherOutgoingWebhooks(t *testing.T) {
	receivedPayloads := make(chan map[string]interface{}, 2)
	receivedHeaders := make(chan http.Header, 2)

	cfg := &config.Config{
		Webhooks: []config.WebhookConfig{
			{
				Event:    EventDeploymentRequested,
				Endpoint: "http://example.internal/webhook",
				Headers: map[string]string{
					"X-Custom-Auth": "secret123",
				},
				Payload: `{"action":"deploy","svc":"{{ .Service }}","env":"{{ .Environment }}","ver":"{{ .TargetVersion }}","by":"{{ .User }}"}`,
			},
		},
	}

	d := NewDispatcher(cfg)
	// Inject mock transport so no network socket is required
	d.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		var m map[string]interface{}
		_ = json.Unmarshal(body, &m)
		receivedPayloads <- m
		receivedHeaders <- req.Header.Clone()

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})

	eventData := DeploymentRequestedEvent{
		Event:         EventDeploymentRequested,
		Service:       "billing-service",
		Environment:   "staging",
		TargetVersion: "v1.4.5",
		User:          "Alice",
		Timestamp:     time.Now(),
	}

	rec := d.SendDirect(context.Background(), cfg.Webhooks[0], EventDeploymentRequested, eventData, "billing-service")
	if !rec.Success {
		t.Fatalf("expected successful dispatch, got error: %s (status %d)", rec.Error, rec.StatusCode)
	}

	select {
	case payload := <-receivedPayloads:
		if payload["svc"] != "billing-service" || payload["env"] != "staging" || payload["ver"] != "v1.4.5" || payload["by"] != "Alice" {
			t.Errorf("unexpected received payload: %+v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook payload")
	}

	select {
	case headers := <-receivedHeaders:
		if headers.Get("X-Custom-Auth") != "secret123" {
			t.Errorf("expected X-Custom-Auth header secret123, got %s", headers.Get("X-Custom-Auth"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook headers")
	}

	// Verify history
	history := d.GetHistory()
	if len(history) != 1 {
		t.Errorf("expected 1 history record, got %d", len(history))
	} else if history[0].StatusCode != 200 {
		t.Errorf("expected history status 200, got %d", history[0].StatusCode)
	}
}
