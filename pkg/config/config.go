package config

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the root declarative configuration for DeployStackr.
type Config struct {
	Version             string                    `yaml:"version" json:"version"`
	GitOps              GitOpsConfig              `yaml:"gitops" json:"gitops"`
	Auth                *AuthConfig               `yaml:"auth,omitempty" json:"auth,omitempty"`
	Cluster             *ClusterConfig            `yaml:"cluster,omitempty" json:"cluster,omitempty"`
	EnvironmentPriority []string                  `yaml:"environment_priority,omitempty" json:"environment_priority,omitempty"`
	Webhooks            []WebhookConfig           `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
	Services            map[string]*ServiceConfig `yaml:"services" json:"services"`
	ServiceOrder        []string                  `yaml:"-" json:"-"`
	inRepoServices      map[string]bool
}

// ClusterConfig defines cluster status check settings.
type ClusterConfig struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	DefaultNamespace string `yaml:"default_namespace,omitempty" json:"default_namespace,omitempty"`
	KubeAPIServer    string `yaml:"kube_api_server,omitempty" json:"kube_api_server,omitempty"`
	Token            string `yaml:"token,omitempty" json:"token,omitempty"`
	CAFile           string `yaml:"ca_file,omitempty" json:"ca_file,omitempty"`
}

// StatusCheckConfig specifies cluster status lookup rules for a service or environment.
type StatusCheckConfig struct {
	Enabled    *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Namespace  string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Deployment string `yaml:"deployment,omitempty" json:"deployment,omitempty"`
}

// WebhookConfig defines an outgoing webhook subscription with target endpoint and payload template.
type WebhookConfig struct {
	Event    string            `yaml:"event" json:"event"` // e.g. "deployment_requested", "deployment_completed", "new_version_available", "pr_merged", "*"
	Endpoint string            `yaml:"endpoint,omitempty" json:"endpoint,omitempty"` // Target URL
	URL      string            `yaml:"url,omitempty" json:"url,omitempty"`           // Alias for endpoint
	Method   string            `yaml:"method,omitempty" json:"method,omitempty"`     // Default "POST"
	Headers  map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`   // Custom headers
	Payload  string            `yaml:"payload,omitempty" json:"payload,omitempty"`   // Custom body template (Go template syntax)
}

// AuthConfig defines OAuth2 credentials and dev mode toggles.
type AuthConfig struct {
	Provider string               `yaml:"provider,omitempty" json:"provider,omitempty"` // "github", "google", or "dev"
	DevMode  bool                 `yaml:"dev_mode,omitempty" json:"dev_mode,omitempty"`
	GitHub   *OAuthProviderConfig `yaml:"github,omitempty" json:"github,omitempty"`
	Google   *OAuthProviderConfig `yaml:"google,omitempty" json:"google,omitempty"`
}

// OAuthProviderConfig holds client ID and secret for an OAuth provider.
type OAuthProviderConfig struct {
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
}

// ServicePermissions defines role-based and group-based access control rules.
type ServicePermissions struct {
	GitHubTeams       []string `yaml:"github_teams,omitempty" json:"github_teams,omitempty"`
	GoogleGroups      []string `yaml:"google_groups,omitempty" json:"google_groups,omitempty"`
	RequireRepoAccess string   `yaml:"require_repo_access,omitempty" json:"require_repo_access,omitempty"` // "read", "write", "admin"
}

// GitOpsConfig defines the default GitOps repository settings.
type GitOpsConfig struct {
	Repo       string `yaml:"repo" json:"repo"`
	Branch     string `yaml:"branch" json:"branch"`
	ConfigFile string `yaml:"config_file,omitempty" json:"config_file,omitempty"` // In-repo config filename, defaults to .deploystackr.yaml
}

// ServiceConfig defines upstream artifacts, context, and environment targets for a service.
type ServiceConfig struct {
	DisplayName         string                        `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Icon                string                        `yaml:"icon,omitempty" json:"icon,omitempty"`
	Source              SourceConfig                  `yaml:"source" json:"source"`
	Context             ContextConfig                 `yaml:"context" json:"context"`
	Permissions         *ServicePermissions           `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	EnvironmentPriority []string                      `yaml:"environment_priority,omitempty" json:"environment_priority,omitempty"`
	Webhooks            []WebhookConfig               `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
	StatusCheck         *StatusCheckConfig            `yaml:"status_check,omitempty" json:"status_check,omitempty"`
	Environments        map[string]*EnvironmentConfig `yaml:"environments" json:"environments"`
}

// SourceConfig configures discovery of upstream deployable artifacts.
type SourceConfig struct {
	Type       string `yaml:"type" json:"type"`                                     // "oci", "github_releases", "git_tags"
	Repository string `yaml:"repository" json:"repository"`                         // e.g. "ghcr.io/org/billing-service"
	Filter     string `yaml:"filter" json:"filter"`                                 // SemVer regex filter
	TrimPrefix string `yaml:"trim_prefix,omitempty" json:"trim_prefix,omitempty"` // Optional prefix to trim from discovered versions (e.g. "v")
}

// ContextConfig configures source code context for diffing and release notes.
type ContextConfig struct {
	Repo              string `yaml:"repo" json:"repo"`
	ChangelogStrategy string `yaml:"changelog_strategy" json:"changelog_strategy"`
	TagPrefix         string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty"`
	Prefix            string `yaml:"prefix,omitempty" json:"prefix,omitempty"`
}

func (c ContextConfig) GetTagPrefix() string {
	if c.TagPrefix != "" {
		return c.TagPrefix
	}
	return c.Prefix
}

// EnvironmentConfig defines how an environment is targeted and mutated.
type EnvironmentConfig struct {
	Repo          string              `yaml:"repo,omitempty" json:"repo,omitempty"` // Multi-repo override
	Branch        string              `yaml:"branch" json:"branch"`
	Target        TargetConfig        `yaml:"target" json:"target"`
	WriteStrategy WriteStrategy       `yaml:"write_strategy" json:"write_strategy"`
	Permissions   *ServicePermissions `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	StatusCheck   *StatusCheckConfig  `yaml:"status_check,omitempty" json:"status_check,omitempty"`
}

// TargetConfig specifies the file and path selector to mutate.
type TargetConfig struct {
	File string `yaml:"file" json:"file"`
	Path string `yaml:"path" json:"path"` // e.g. "$.image.tag" or "$.spec.template.spec.containers[0].image"
}

// WriteStrategy specifies whether promotions are direct commits or Pull Requests.
type WriteStrategy struct {
	Mode          string `yaml:"mode" json:"mode"` // "direct" | "pull_request"
	CommitMessage string `yaml:"commit_message,omitempty" json:"commit_message,omitempty"`
	Title         string `yaml:"title,omitempty" json:"title,omitempty"`
	Body          string `yaml:"body,omitempty" json:"body,omitempty"`
}

// Load loads and parses the configuration file from a local path or HTTP(S) URL.
func Load(pathOrURL string) (*Config, error) {
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		return LoadRemote(pathOrURL)
	}

	data, err := os.ReadFile(pathOrURL)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	return Parse(data)
}

// HTTPClient is the client used by LoadRemote (can be mocked in tests).
var HTTPClient = &http.Client{Timeout: 15 * time.Second}

// LoadRemote fetches and parses configuration from an HTTP(S) URL (e.g. GitHub raw URL).
func LoadRemote(url string) (*Config, error) {
	client := HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request for config: %w", err)
	}

	// Support GITHUB_TOKEN or GIT_TOKEN for private repos
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("GIT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch remote config from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch remote config failed with status: %d %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read remote config body: %w", err)
	}

	return Parse(data)
}

// extractServiceOrder inspects the raw YAML mapping to retain original service declaration order.
func extractServiceOrder(data []byte) []string {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil
	}
	root := &node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "services" {
			svcMap := root.Content[i+1]
			if svcMap.Kind == yaml.MappingNode {
				order := make([]string, 0, len(svcMap.Content)/2)
				for j := 0; j < len(svcMap.Content)-1; j += 2 {
					order = append(order, svcMap.Content[j].Value)
				}
				return order
			}
		}
	}
	return nil
}

// Parse decodes raw YAML bytes and applies environment overrides and defaults.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}

	if cfg.Services == nil {
		cfg.Services = make(map[string]*ServiceConfig)
	}

	cfg.ServiceOrder = extractServiceOrder(data)
	seen := make(map[string]bool)
	for _, name := range cfg.ServiceOrder {
		seen[name] = true
	}
	var extra []string
	for name := range cfg.Services {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	cfg.ServiceOrder = append(cfg.ServiceOrder, extra...)

	if cfg.Auth == nil {
		cfg.Auth = &AuthConfig{}
	}

	// Environment variable overrides for OAuth credentials
	if ghID := os.Getenv("GITHUB_CLIENT_ID"); ghID != "" {
		if cfg.Auth.GitHub == nil {
			cfg.Auth.GitHub = &OAuthProviderConfig{}
		}
		cfg.Auth.GitHub.ClientID = ghID
	}
	if ghSecret := os.Getenv("GITHUB_CLIENT_SECRET"); ghSecret != "" {
		if cfg.Auth.GitHub == nil {
			cfg.Auth.GitHub = &OAuthProviderConfig{}
		}
		cfg.Auth.GitHub.ClientSecret = ghSecret
	}

	if gID := os.Getenv("GOOGLE_CLIENT_ID"); gID != "" {
		if cfg.Auth.Google == nil {
			cfg.Auth.Google = &OAuthProviderConfig{}
		}
		cfg.Auth.Google.ClientID = gID
	}
	if gSecret := os.Getenv("GOOGLE_CLIENT_SECRET"); gSecret != "" {
		if cfg.Auth.Google == nil {
			cfg.Auth.Google = &OAuthProviderConfig{}
		}
		cfg.Auth.Google.ClientSecret = gSecret
	}

	if prov := os.Getenv("AUTH_PROVIDER"); prov != "" {
		cfg.Auth.Provider = prov
	}

	if os.Getenv("AUTH_DEV_MODE") == "true" || os.Getenv("DEV_MODE") == "true" {
		cfg.Auth.DevMode = true
	}

	if cfg.Cluster != nil {
		if os.Getenv("CLUSTER_STATUS_ENABLED") == "true" {
			cfg.Cluster.Enabled = true
		} else if os.Getenv("CLUSTER_STATUS_ENABLED") == "false" {
			cfg.Cluster.Enabled = false
		}
		if cfg.Cluster.DefaultNamespace == "" {
			cfg.Cluster.DefaultNamespace = "default"
		}
		if cfg.Cluster.KubeAPIServer == "" {
			if apiServer := os.Getenv("KUBERNETES_API_SERVER"); apiServer != "" {
				cfg.Cluster.KubeAPIServer = apiServer
			}
		}
	} else if os.Getenv("CLUSTER_STATUS_ENABLED") == "true" {
		cfg.Cluster = &ClusterConfig{
			Enabled:          true,
			DefaultNamespace: "default",
			KubeAPIServer:    os.Getenv("KUBERNETES_API_SERVER"),
		}
	}

	normalizeWebhook := func(w *WebhookConfig) {
		if w.Endpoint == "" && w.URL != "" {
			w.Endpoint = w.URL
		}
		if w.Method == "" {
			w.Method = "POST"
		}
	}
	for i := range cfg.Webhooks {
		normalizeWebhook(&cfg.Webhooks[i])
	}

	// Apply default GitOps repo/branch to environments if not specified
	for _, svc := range cfg.Services {
		for i := range svc.Webhooks {
			normalizeWebhook(&svc.Webhooks[i])
		}
		for _, env := range svc.Environments {
			if env.Repo == "" {
				env.Repo = cfg.GitOps.Repo
			}
			if env.Branch == "" {
				env.Branch = cfg.GitOps.Branch
			}
			if env.Branch == "" {
				env.Branch = "main"
			}
			if env.WriteStrategy.Mode == "" {
				env.WriteStrategy.Mode = "direct"
			}
		}
	}

	return &cfg, nil
}

// MergeServices overlays newly discovered services into the active config while preserving ordering.
func (c *Config) MergeServices(newServices map[string]*ServiceConfig, order ...string) {
	if c.Services == nil {
		c.Services = make(map[string]*ServiceConfig)
	}

	normalizeEnv := func(svc *ServiceConfig) {
		for _, env := range svc.Environments {
			if env.Repo == "" {
				env.Repo = c.GitOps.Repo
			}
			if env.Branch == "" {
				env.Branch = c.GitOps.Branch
			}
			if env.Branch == "" {
				env.Branch = "main"
			}
			if env.WriteStrategy.Mode == "" {
				env.WriteStrategy.Mode = "direct"
			}
		}
	}

	for _, name := range order {
		if svc, ok := newServices[name]; ok {
			normalizeEnv(svc)
			c.Services[name] = svc
			c.appendServiceOrder(name)
		}
	}

	var remaining []string
	for name := range newServices {
		found := false
		for _, ord := range order {
			if ord == name {
				found = true
				break
			}
		}
		if !found {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	for _, name := range remaining {
		svc := newServices[name]
		normalizeEnv(svc)
		c.Services[name] = svc
		c.appendServiceOrder(name)
	}
}

// SyncInRepoServices updates services discovered from an in-repo config, overlaying new/updated
// definitions and returning the names of any previously discovered in-repo services that were removed.
func (c *Config) SyncInRepoServices(newServices map[string]*ServiceConfig, order ...string) []string {
	if c.Services == nil {
		c.Services = make(map[string]*ServiceConfig)
	}
	if c.inRepoServices == nil {
		c.inRepoServices = make(map[string]bool)
	}

	var removed []string
	for prev := range c.inRepoServices {
		if _, exists := newServices[prev]; !exists {
			delete(c.Services, prev)
			c.removeServiceOrder(prev)
			removed = append(removed, prev)
		}
	}

	c.inRepoServices = make(map[string]bool, len(newServices))
	for name := range newServices {
		c.inRepoServices[name] = true
	}

	c.MergeServices(newServices, order...)
	return removed
}

func (c *Config) removeServiceOrder(name string) {
	newOrder := make([]string, 0, len(c.ServiceOrder))
	for _, existing := range c.ServiceOrder {
		if existing != name {
			newOrder = append(newOrder, existing)
		}
	}
	c.ServiceOrder = newOrder
}

func (c *Config) appendServiceOrder(name string) {
	for _, existing := range c.ServiceOrder {
		if existing == name {
			return
		}
	}
	c.ServiceOrder = append(c.ServiceOrder, name)
}
