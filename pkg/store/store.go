package store

import (
	"sync"
	"time"

	"deploystackr/pkg/cluster"
)

// DeploymentEvent represents a single deployment lifecycle event recorded from Git history.
type DeploymentEvent struct {
	Service        string     `json:"service"`
	Environment    string     `json:"environment"`
	Version        string     `json:"version"`
	CommitSHA      string     `json:"commit_sha"`
	CommitMessage  string     `json:"commit_message"`
	AuthorName     string     `json:"author_name"`
	AuthorEmail    string     `json:"author_email"`
	PRNumber       int        `json:"pr_number,omitempty"`
	DeployedAt     time.Time  `json:"deployed_at"`
	RetiredAt      *time.Time `json:"retired_at,omitempty"`
	ActiveDuration string     `json:"active_duration,omitempty"` // Human-readable e.g. "2d 4h" or "active"
	IsActive       bool       `json:"is_active"`
}

// ServiceStatus represents the summary view for the service catalog.
type ServiceStatus struct {
	Service      string                     `json:"service"`
	Environments map[string]EnvironmentView `json:"environments"`
}

// PendingPR represents an open Pull Request for a deployment to an environment.
type PendingPR struct {
	Service       string    `json:"service"`
	Environment   string    `json:"environment"`
	TargetVersion string    `json:"target_version"`
	Branch        string    `json:"branch,omitempty"`
	PRNumber      int       `json:"pr_number,omitempty"`
	PRURL         string    `json:"pr_url,omitempty"`
	User          string    `json:"user,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// EnvironmentView provides the current declared intent for an environment.
type EnvironmentView struct {
	Repo          string            `json:"repo,omitempty"`
	ActiveVersion string            `json:"active_version"`
	LastDeployed  *time.Time        `json:"last_deployed,omitempty"`
	DeployedBy    string            `json:"deployed_by,omitempty"`
	CommitSHA     string            `json:"commit_sha,omitempty"`
	PRNumber      int               `json:"pr_number,omitempty"`
	WriteStrategy string            `json:"write_strategy"`
	TargetFile    string            `json:"target_file"`
	TargetPath    string            `json:"target_path"`
	RecentEvents  []DeploymentEvent `json:"recent_events,omitempty"`
	PendingPR     *PendingPR        `json:"pending_pr,omitempty"`
	ClusterStatus *cluster.Status   `json:"cluster_status,omitempty"`
}

// MemoryStore is the thread-safe in-memory cache for deployment events and active versions.
type MemoryStore struct {
	sync.RWMutex
	// Map: service_name -> environment_name -> slice of events sorted desc by DeployedAt
	events     map[string]map[string][]DeploymentEvent
	pendingPRs map[string]map[string]*PendingPR
	versions   map[string][]string
}

// NewMemoryStore initializes an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events:     make(map[string]map[string][]DeploymentEvent),
		pendingPRs: make(map[string]map[string]*PendingPR),
		versions:   make(map[string][]string),
	}
}

// SetVersions caches discovered candidate versions for a service.
func (s *MemoryStore) SetVersions(service string, versions []string) {
	s.Lock()
	defer s.Unlock()
	s.versions[service] = versions
}

// GetVersions returns cached candidate versions for a service.
func (s *MemoryStore) GetVersions(service string) []string {
	s.RLock()
	defer s.RUnlock()
	if v, ok := s.versions[service]; ok {
		copied := make([]string, len(v))
		copy(copied, v)
		return copied
	}
	return nil
}

// SetPendingPR records an open deployment PR for an environment.
func (s *MemoryStore) SetPendingPR(service, env string, pr *PendingPR) {
	s.Lock()
	defer s.Unlock()

	if _, ok := s.pendingPRs[service]; !ok {
		s.pendingPRs[service] = make(map[string]*PendingPR)
	}
	s.pendingPRs[service][env] = pr
}

// GetPendingPR retrieves an open deployment PR for an environment if any.
func (s *MemoryStore) GetPendingPR(service, env string) *PendingPR {
	s.RLock()
	defer s.RUnlock()

	if envMap, ok := s.pendingPRs[service]; ok {
		if pr, exists := envMap[env]; exists && pr != nil {
			copied := *pr
			return &copied
		}
	}
	return nil
}

// ClearPendingPR removes an open deployment PR record.
func (s *MemoryStore) ClearPendingPR(service, env string) {
	s.Lock()
	defer s.Unlock()

	if envMap, ok := s.pendingPRs[service]; ok {
		delete(envMap, env)
	}
}

// ClearPendingPRByNumber removes any pending PR record matching the PR number.
func (s *MemoryStore) ClearPendingPRByNumber(prNum int) (string, string) {
	if prNum <= 0 {
		return "", ""
	}
	s.Lock()
	defer s.Unlock()

	for svc, envs := range s.pendingPRs {
		for env, pr := range envs {
			if pr != nil && pr.PRNumber == prNum {
				delete(envs, env)
				return svc, env
			}
		}
	}
	return "", ""
}

// ClearPendingPRByBranch removes any pending PR record matching the branch name.
func (s *MemoryStore) ClearPendingPRByBranch(branch string) (string, string) {
	if branch == "" {
		return "", ""
	}
	s.Lock()
	defer s.Unlock()

	for svc, envs := range s.pendingPRs {
		for env, pr := range envs {
			if pr != nil && pr.Branch == branch {
				delete(envs, env)
				return svc, env
			}
		}
	}
	return "", ""
}

// GetAllPendingPRs returns a copy of all currently pending PRs.
func (s *MemoryStore) GetAllPendingPRs() []*PendingPR {
	s.RLock()
	defer s.RUnlock()

	var list []*PendingPR
	for _, envs := range s.pendingPRs {
		for _, pr := range envs {
			if pr != nil {
				copied := *pr
				list = append(list, &copied)
			}
		}
	}
	return list
}

// DeleteService removes cached events and pending PRs for a service.
func (s *MemoryStore) DeleteService(service string) {
	s.Lock()
	defer s.Unlock()

	delete(s.events, service)
	delete(s.pendingPRs, service)
}

// SetEvents safely updates the deployment events for a service environment.
func (s *MemoryStore) SetEvents(service, env string, events []DeploymentEvent) {
	s.Lock()
	defer s.Unlock()

	if _, ok := s.events[service]; !ok {
		s.events[service] = make(map[string][]DeploymentEvent)
	}
	s.events[service][env] = events
}

// GetHistory returns all deployment events for a service grouped by environment.
func (s *MemoryStore) GetHistory(service string) map[string][]DeploymentEvent {
	s.RLock()
	defer s.RUnlock()

	result := make(map[string][]DeploymentEvent)
	envMap, ok := s.events[service]
	if !ok {
		return result
	}

	for env, events := range envMap {
		copied := make([]DeploymentEvent, len(events))
		copy(copied, events)
		result[env] = copied
	}
	return result
}

// GetEnvironmentHistory returns the event list for a specific service and environment.
func (s *MemoryStore) GetEnvironmentHistory(service, env string) []DeploymentEvent {
	s.RLock()
	defer s.RUnlock()

	if envMap, ok := s.events[service]; ok {
		if events, exists := envMap[env]; exists {
			copied := make([]DeploymentEvent, len(events))
			copy(copied, events)
			return copied
		}
	}
	return nil
}

// GetActiveEvent returns the latest (active) deployment event for a service and environment.
func (s *MemoryStore) GetActiveEvent(service, env string) *DeploymentEvent {
	s.RLock()
	defer s.RUnlock()

	if envMap, ok := s.events[service]; ok {
		if events, exists := envMap[env]; exists && len(events) > 0 {
			event := events[0]
			return &event
		}
	}
	return nil
}
