package cluster

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/config"
	"gopkg.in/yaml.v3"
)

// Status represents the runtime status of a Kubernetes deployment.
type Status struct {
	Status          string `json:"status"` // "ready", "progressing", "pending", "degraded", "not_found", "error"
	ReadyReplicas   int    `json:"ready_replicas"`
	UpdatedReplicas int    `json:"updated_replicas"`
	DesiredReplicas int    `json:"desired_replicas"`
	CurrentVersion  string `json:"current_version,omitempty"`
	Message         string `json:"message,omitempty"`
}

type cacheEntry struct {
	status    *Status
	expiresAt time.Time
}

// Checker queries in-cluster Kubernetes API server for workload status.
type Checker struct {
	cfg         *config.ClusterConfig
	httpClient  *http.Client
	baseURL     string
	token       string
	available   bool
	lastFailure time.Time
	cacheMu     sync.RWMutex
	cache       map[string]cacheEntry
	ttl         time.Duration
}

// NewChecker initializes a Kubernetes status checker using in-cluster credentials or config overrides.
func NewChecker(cfg *config.ClusterConfig) *Checker {
	c := &Checker{
		cfg:   cfg,
		cache: make(map[string]cacheEntry),
		ttl:   10 * time.Second,
	}

	if cfg == nil || !cfg.Enabled {
		return c
	}

	token := cfg.Token
	if token == "" {
		token = os.Getenv("KUBERNETES_TOKEN")
	}
	if token == "" {
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
			token = strings.TrimSpace(string(data))
		}
	}

	if token == "" {
		//no credentials available to query cluster
		return c
	}

	baseURL := cfg.KubeAPIServer
	if baseURL == "" {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT")
		if host != "" && port != "" {
			baseURL = fmt.Sprintf("https://%s:%s", host, port)
		} else {
			baseURL = "https://kubernetes.default.svc"
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	tlsConfig := &tls.Config{}
	caPath := cfg.CAFile
	if caPath == "" {
		caPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	}

	if caData, err := os.ReadFile(caPath); err == nil {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(caData) {
			tlsConfig.RootCAs = pool
		}
	} else if strings.HasPrefix(baseURL, "https://127.0.0.1") || strings.HasPrefix(baseURL, "https://localhost") {
		//allow self-signed certs for local testing
		tlsConfig.InsecureSkipVerify = true
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	c.httpClient = &http.Client{
		Transport: transport,
		Timeout:   1200 * time.Millisecond,
	}
	c.baseURL = baseURL
	c.token = token
	c.available = true

	return c
}

// IsAvailable reports whether the cluster checker has active cluster credentials.
func (c *Checker) IsAvailable() bool {
	return c != nil && c.available
}

// DefaultNamespace returns the configured default namespace or "default".
func (c *Checker) DefaultNamespace() string {
	if c != nil && c.cfg != nil && c.cfg.DefaultNamespace != "" {
		return c.cfg.DefaultNamespace
	}
	return "default"
}

type k8sDeploymentResponse struct {
	Spec struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name  string `json:"name"`
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas          int `json:"replicas"`
		UpdatedReplicas   int `json:"updatedReplicas"`
		ReadyReplicas     int `json:"readyReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
		Conditions        []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

// GetDeploymentStatus fetches the runtime status for a deployment, with in-memory caching.
func (c *Checker) GetDeploymentStatus(ctx context.Context, namespace, deployment string, expectedVersion ...string) (*Status, error) {
	if !c.IsAvailable() || namespace == "" || deployment == "" {
		return nil, nil
	}

	expected := ""
	if len(expectedVersion) > 0 {
		expected = expectedVersion[0]
	}

	cacheKey := namespace + "/" + deployment
	if expected != "" {
		cacheKey += "@" + expected
	}

	c.cacheMu.RLock()
	if !c.lastFailure.IsZero() && time.Since(c.lastFailure) < 20*time.Second {
		if entry, ok := c.cache[cacheKey]; ok {
			c.cacheMu.RUnlock()
			return entry.status, nil
		}
		c.cacheMu.RUnlock()
		return &Status{
			Status:  "error",
			Message: "cluster API temporarily unreachable",
		}, nil
	}
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		c.cacheMu.RUnlock()
		return entry.status, nil
	}
	c.cacheMu.RUnlock()

	reqURL := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/deployments/%s", c.baseURL, namespace, deployment)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.cacheMu.Lock()
		c.lastFailure = time.Now()
		c.cacheMu.Unlock()
		st := &Status{
			Status:  "error",
			Message: err.Error(),
		}
		c.setCache(cacheKey, st)
		return st, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		st := &Status{
			Status:  "not_found",
			Message: fmt.Sprintf("deployment '%s' not found in namespace '%s'", deployment, namespace),
		}
		c.setCache(cacheKey, st)
		return st, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		st := &Status{
			Status:  "error",
			Message: fmt.Sprintf("kubernetes api error %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
		c.setCache(cacheKey, st)
		return st, nil
	}

	var dep k8sDeploymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&dep); err != nil {
		return nil, err
	}

	desired := 1
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}

	ready := dep.Status.ReadyReplicas
	updated := dep.Status.UpdatedReplicas

	currentTag := ""
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		img := dep.Spec.Template.Spec.Containers[0].Image
		if idx := strings.LastIndex(img, ":"); idx != -1 {
			currentTag = img[idx+1:]
		}
	}

	st := &Status{
		ReadyReplicas:   ready,
		UpdatedReplicas: updated,
		DesiredReplicas: desired,
		CurrentVersion:  currentTag,
	}

	//check if expected version is ahead of cluster deployment spec
	isPendingSync := false
	if expected != "" && expected != "unknown" && expected != "*" && currentTag != "" {
		expTag := expected
		if idx := strings.LastIndex(expTag, ":"); idx != -1 {
			expTag = expTag[idx+1:]
		}
		expClean := strings.TrimPrefix(expTag, "v")
		tagClean := strings.TrimPrefix(currentTag, "v")
		if expClean != tagClean && expTag != currentTag && expected != currentTag {
			isPendingSync = true
		}
	}

	//check conditions for progress errors
	for _, cond := range dep.Status.Conditions {
		if cond.Type == "Progressing" && cond.Status == "False" {
			st.Status = "degraded"
			st.Message = cond.Message
			if st.Message == "" {
				st.Message = cond.Reason
			}
			c.setCache(cacheKey, st)
			return st, nil
		}
		if cond.Type == "ReplicaFailure" && cond.Status == "True" {
			st.Status = "degraded"
			st.Message = cond.Message
			c.setCache(cacheKey, st)
			return st, nil
		}
	}

	if isPendingSync {
		st.Status = "pending"
		st.ReadyReplicas = 0
		st.UpdatedReplicas = 0
		st.Message = fmt.Sprintf("waiting for gitops sync (running %s, target %s)", currentTag, expected)
	} else if desired == 0 && ready == 0 {
		st.Status = "ready"
		st.Message = "scaled to 0"
	} else if ready >= desired && updated >= desired {
		st.Status = "ready"
		st.Message = fmt.Sprintf("%d/%d replicas ready", ready, desired)
	} else {
		st.Status = "progressing"
		st.Message = fmt.Sprintf("rollout in progress (%d/%d ready, %d/%d updated)", ready, desired, updated, desired)
	}

	c.setCache(cacheKey, st)
	return st, nil
}

func (c *Checker) setCache(key string, status *Status) {
	c.cacheMu.Lock()
	c.cache[key] = cacheEntry{
		status:    status,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.cacheMu.Unlock()
}

// SniffDeploymentManifest inspects YAML content to extract deployment name and namespace if present.
func SniffDeploymentManifest(manifestYAML []byte) (name, namespace string, ok bool) {
	dec := yaml.NewDecoder(bytes.NewReader(manifestYAML))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
		}
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc.Kind == "Deployment" && doc.Metadata.Name != "" {
			return doc.Metadata.Name, doc.Metadata.Namespace, true
		}
	}
	return "", "", false
}
