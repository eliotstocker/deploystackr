package store

import (
	"testing"
	"time"
)

func TestMemoryStore_DeleteService(t *testing.T) {
	s := NewMemoryStore()
	s.SetPendingPR("test-service", "prod", &PendingPR{
		Service:       "test-service",
		Environment:   "prod",
		TargetVersion: "1.0.0",
		CreatedAt:     time.Now(),
	})
	s.SetEvents("test-service", "prod", []DeploymentEvent{
		{
			Service:     "test-service",
			Environment: "prod",
			Version:     "1.0.0",
			DeployedAt:  time.Now(),
		},
	})

	if pr := s.GetPendingPR("test-service", "prod"); pr == nil {
		t.Fatalf("expected pending PR")
	}
	if events := s.GetHistory("test-service"); len(events) == 0 {
		t.Fatalf("expected events")
	}

	s.DeleteService("test-service")

	if pr := s.GetPendingPR("test-service", "prod"); pr != nil {
		t.Errorf("expected pending PR to be cleared after DeleteService")
	}
	if events := s.GetHistory("test-service"); len(events) != 0 {
		t.Errorf("expected history to be empty after DeleteService")
	}
}

func TestMemoryStore_ClearPendingPRByNumberAndBranch(t *testing.T) {
	s := NewMemoryStore()
	s.SetPendingPR("svc-a", "pi", &PendingPR{
		Service:       "svc-a",
		Environment:   "pi",
		TargetVersion: "v1.0.0",
		PRNumber:      42,
		Branch:        "promote/svc-a-v1.0.0",
		CreatedAt:     time.Now(),
	})
	s.SetPendingPR("svc-b", "pi", &PendingPR{
		Service:       "svc-b",
		Environment:   "pi",
		TargetVersion: "v2.0.0",
		Branch:        "promote/svc-b-v2.0.0",
		CreatedAt:     time.Now(),
	})

	// Clear by number
	svc, env := s.ClearPendingPRByNumber(42)
	if svc != "svc-a" || env != "pi" {
		t.Errorf("expected svc-a/pi, got %s/%s", svc, env)
	}
	if pr := s.GetPendingPR("svc-a", "pi"); pr != nil {
		t.Errorf("expected svc-a PR to be cleared")
	}

	// Clear by branch
	svc, env = s.ClearPendingPRByBranch("promote/svc-b-v2.0.0")
	if svc != "svc-b" || env != "pi" {
		t.Errorf("expected svc-b/pi, got %s/%s", svc, env)
	}
	if pr := s.GetPendingPR("svc-b", "pi"); pr != nil {
		t.Errorf("expected svc-b PR to be cleared")
	}
}
