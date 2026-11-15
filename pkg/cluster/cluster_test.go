package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"deploystackr/pkg/config"
)

func TestSniffDeploymentManifest(t *testing.T) {
	singleDoc := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: torrent
  namespace: custom-apps
spec:
  replicas: 2
`
	name, ns, ok := SniffDeploymentManifest([]byte(singleDoc))
	if !ok || name != "torrent" || ns != "custom-apps" {
		t.Fatalf("expected torrent / custom-apps, got %s / %s (ok=%v)", name, ns, ok)
	}

	multiDoc := `
apiVersion: v1
kind: Namespace
metadata:
  name: ingress-nginx
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingress-nginx-controller
`
	name, ns, ok = SniffDeploymentManifest([]byte(multiDoc))
	if !ok || name != "ingress-nginx-controller" || ns != "" {
		t.Fatalf("expected ingress-nginx-controller, got %s / %s (ok=%v)", name, ns, ok)
	}

	nonDeployment := `
apiVersion: v1
kind: Service
metadata:
  name: torrent-svc
`
	name, ns, ok = SniffDeploymentManifest([]byte(nonDeployment))
	if ok {
		t.Fatalf("expected not ok for non-deployment, got %s / %s", name, ns)
	}
}

func TestGetDeploymentStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/default/deployments/ready-app":
			replicas := 2
			resp := k8sDeploymentResponse{}
			resp.Spec.Replicas = &replicas
			resp.Spec.Template.Spec.Containers = []struct {
				Name  string `json:"name"`
				Image string `json:"image"`
			}{
				{Name: "ready-app", Image: "registry.local/ready-app:v1.0.0"},
			}
			resp.Status.Replicas = 2
			resp.Status.UpdatedReplicas = 2
			resp.Status.ReadyReplicas = 2
			resp.Status.AvailableReplicas = 2
			_ = json.NewEncoder(w).Encode(resp)

		case "/apis/apps/v1/namespaces/default/deployments/progressing-app":
			replicas := 3
			resp := k8sDeploymentResponse{}
			resp.Spec.Replicas = &replicas
			resp.Status.Replicas = 3
			resp.Status.UpdatedReplicas = 1
			resp.Status.ReadyReplicas = 1
			_ = json.NewEncoder(w).Encode(resp)

		case "/apis/apps/v1/namespaces/default/deployments/degraded-app":
			replicas := 1
			resp := k8sDeploymentResponse{}
			resp.Spec.Replicas = &replicas
			resp.Status.Conditions = []struct {
				Type    string `json:"type"`
				Status  string `json:"status"`
				Reason  string `json:"reason"`
				Message string `json:"message"`
			}{
				{
					Type:    "Progressing",
					Status:  "False",
					Reason:  "ProgressDeadlineExceeded",
					Message: "ReplicaSet timed out",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "/apis/apps/v1/namespaces/default/deployments/missing-app":
			http.NotFound(w, r)

		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	checker := NewChecker(&config.ClusterConfig{
		Enabled:          true,
		DefaultNamespace: "default",
		KubeAPIServer:    ts.URL,
		Token:            "test-token",
	})

	if !checker.IsAvailable() {
		t.Fatalf("expected checker to be available")
	}

	ctx := context.Background()

	// 1. Ready
	st, err := checker.GetDeploymentStatus(ctx, "default", "ready-app", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Status != "ready" || st.ReadyReplicas != 2 {
		t.Fatalf("expected ready status with 2 ready replicas, got %+v", st)
	}

	// 1a. Ready with full image repo in expected version
	stFullRepo, err := checker.GetDeploymentStatus(context.Background(), "default", "ready-app", "registry.local/ready-app:v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stFullRepo.Status != "ready" {
		t.Fatalf("expected ready status when expected version has full image repo prefix, got %+v", stFullRepo)
	}

	// 1b. Pending sync (expected version is v2.0.0 but cluster is on v1.0.0)
	stPending, err := checker.GetDeploymentStatus(ctx, "default", "ready-app", "v2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stPending.Status != "pending" {
		t.Fatalf("expected pending status, got %+v", stPending)
	}
	if stPending.ReadyReplicas != 0 || stPending.UpdatedReplicas != 0 {
		t.Fatalf("expected 0 ready and 0 updated replicas in pending status, got %+v", stPending)
	}

	// Test caching: hit ready-app again
	cached, _ := checker.GetDeploymentStatus(ctx, "default", "ready-app", "v1.0.0")
	if cached != st {
		t.Errorf("expected cached pointer instance")
	}

	// 2. Progressing
	stProg, err := checker.GetDeploymentStatus(ctx, "default", "progressing-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stProg.Status != "progressing" {
		t.Fatalf("expected progressing status, got %+v", stProg)
	}

	// 3. Degraded
	stDeg, err := checker.GetDeploymentStatus(ctx, "default", "degraded-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stDeg.Status != "degraded" || stDeg.Message != "ReplicaSet timed out" {
		t.Fatalf("expected degraded status with deadline reason, got %+v", stDeg)
	}

	// 4. Missing
	stMissing, err := checker.GetDeploymentStatus(ctx, "default", "missing-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stMissing.Status != "not_found" {
		t.Fatalf("expected not_found, got %+v", stMissing)
	}
}
