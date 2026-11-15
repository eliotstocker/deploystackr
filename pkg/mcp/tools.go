package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"deploystackr/pkg/auth"
	"deploystackr/pkg/cluster"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/git"
	"deploystackr/pkg/provider"
	"deploystackr/pkg/store"
	"deploystackr/pkg/webhook"
)

func boolPtr(b bool) *bool {
	return &b
}

func (s *Server) registerTools() {
	s.tools = []Tool{
		{
			Name:        "list_services",
			Description: "List configured services in DeployStackr with their service slug, display name, context repository, upstream source, environments, and promotion permissions. Use this for lightweight service discovery before inspecting or promoting a target.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"context_repo": map[string]interface{}{
						"type":        "string",
						"description": "Optional source context repository slug or URL to filter matching services (e.g. 'org/billing-service', HTTPS URL, or SSH remote).",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Optional case-insensitive substring filter matched against service slug, display_name, context_repo, or source repository.",
					},
				},
			},
			Annotations: &ToolAnnotations{
				Title:           "List Services",
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
			},
		},
		{
			Name:        "get_deployed",
			Description: "Inspect active GitOps-declared versions, environment promotion state, audit metadata, pending pull requests, and optional live cluster rollout status. Filter by service name or context repository, or omit both to inspect all services.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service": map[string]interface{}{
						"type":        "string",
						"description": "Service slug or display name (e.g. 'billing-service'). Optional if context_repo is provided, or omit both to list all services.",
					},
					"context_repo": map[string]interface{}{
						"type":        "string",
						"description": "Source context repository slug or URL (e.g. 'org/billing-service', 'https://github.com/org/billing-service.git', or 'git@github.com:org/billing-service.git').",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Optional environment filter (e.g. 'staging', 'production').",
					},
					"include_cluster_status": map[string]interface{}{
						"type":        "boolean",
						"description": "Include live Kubernetes cluster rollout status when cluster checking is enabled. Defaults to true when filtering by service or context_repo, false when listing all services.",
					},
					"include_history": map[string]interface{}{
						"type":        "boolean",
						"description": "Include recent deployment history events (up to 10 per environment). Defaults to false.",
					},
				},
			},
			Annotations: &ToolAnnotations{
				Title:           "Get Deployed Versions",
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
			},
		},
		{
			Name:        "get_deploy_queue",
			Description: "Inspect the GitOps promotion queue across services or for a specific service/context repository: open promotion pull requests awaiting merge, in-flight cluster rollouts, and unpromoted upstream candidate versions ahead of each environment's active version.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service": map[string]interface{}{
						"type":        "string",
						"description": "Service slug or display name. Optional if context_repo is provided, or omit both to inspect all services.",
					},
					"context_repo": map[string]interface{}{
						"type":        "string",
						"description": "Source context repository slug or URL (e.g. 'org/billing-service', HTTPS URL, or SSH remote).",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Optional environment filter (e.g. 'staging', 'production').",
					},
					"only_queued": map[string]interface{}{
						"type":        "boolean",
						"description": "When true, omit environments and services that are up to date with no pending promotion PR or active rollout.",
					},
				},
			},
			Annotations: &ToolAnnotations{
				Title:           "Get Deploy Queue",
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
			},
		},
		{
			Name:        "get_version_diff",
			Description: "Compare Git commits and diff summary between two versions of a service (or between an environment's active version and a target version). Select the service by service name or context repository.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service": map[string]interface{}{
						"type":        "string",
						"description": "Service slug or display name. Required unless context_repo is provided.",
					},
					"context_repo": map[string]interface{}{
						"type":        "string",
						"description": "Source context repository slug or URL. Required unless service is provided.",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Optional environment used to infer from_version when from_version is omitted.",
					},
					"from_version": map[string]interface{}{
						"type":        "string",
						"description": "Base version/tag to compare from. Defaults to the active version in environment (or the highest-priority environment).",
					},
					"to_version": map[string]interface{}{
						"type":        "string",
						"description": "Target version/tag to compare to. Defaults to 'latest' upstream candidate version.",
					},
				},
			},
			Annotations: &ToolAnnotations{
				Title:           "Get Version Diff",
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
			},
		},
		{
			Name:        "promote",
			Description: "Trigger a GitOps version promotion for a service to an environment (via direct commit or Pull Request). Select the service by service name or context repository.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"target_version"},
				"properties": map[string]interface{}{
					"service": map[string]interface{}{
						"type":        "string",
						"description": "Service slug or display name. Required unless context_repo is provided.",
					},
					"context_repo": map[string]interface{}{
						"type":        "string",
						"description": "Source context repository slug or URL. Required unless service is provided.",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Target environment (e.g. 'staging', 'production'). Optional only when the service defines a single environment.",
					},
					"target_version": map[string]interface{}{
						"type":        "string",
						"description": "Exact version/tag to promote (e.g. 'v1.4.2'), or 'latest' to resolve the newest upstream candidate version.",
					},
					"mode": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"direct", "pull_request"},
						"description": "Optional write strategy override ('direct' commit or 'pull_request'). Defaults to the environment's configured write_strategy.mode.",
					},
					"comment": map[string]interface{}{
						"type":        "string",
						"description": "Optional promotion comment appended to the commit message or pull request body.",
					},
					"watch": map[string]interface{}{
						"type":        "boolean",
						"description": "When true for a direct promotion and cluster status is enabled, wait until the Kubernetes rollout reaches ready or fails.",
					},
					"timeout_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Timeout in seconds when watch is true (default 300, max 900).",
					},
				},
			},
			Annotations: &ToolAnnotations{
				Title:           "Promote Service Version",
				ReadOnlyHint:    boolPtr(false),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(false),
			},
		},
		{
			Name:        "get_history",
			Description: "Inspect paginated Git-derived promotion history for a specific service, ordered newest-first, with optional environment and date/time filters. Select the service by service name or context repository.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service": map[string]interface{}{
						"type":        "string",
						"description": "Service slug or display name. Required unless context_repo is provided.",
					},
					"context_repo": map[string]interface{}{
						"type":        "string",
						"description": "Source context repository slug or URL. Required unless service is provided.",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Optional environment filter (e.g. 'staging', 'production').",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of events to return (default 20, max 100).",
					},
					"offset": map[string]interface{}{
						"type":        "integer",
						"description": "Number of newest events to skip for pagination (default 0).",
					},
					"since": map[string]interface{}{
						"type":        "string",
						"description": "Optional inclusive start timestamp (RFC3339 or 'YYYY-MM-DD').",
					},
					"until": map[string]interface{}{
						"type":        "string",
						"description": "Optional inclusive end timestamp (RFC3339 or 'YYYY-MM-DD', treated as end of UTC day).",
					},
				},
			},
			Annotations: &ToolAnnotations{
				Title:           "Get Service History",
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
			},
		},
	}

	s.handlers["list_services"] = s.handleListServices
	s.handlers["get_deployed"] = s.handleGetDeployed
	s.handlers["get_deploy_queue"] = s.handleGetDeployQueue
	s.handlers["get_version_diff"] = s.handleGetVersionDiff
	s.handlers["get_history"] = s.handleGetHistory
	s.handlers["promote"] = s.handlePromote
}

type serviceEnvSummary struct {
	Environment      string `json:"environment"`
	WriteStrategy    string `json:"write_strategy"`
	CanDeploy        bool   `json:"can_deploy"`
	PermissionReason string `json:"permission_reason,omitempty"`
}

type serviceCatalogEntry struct {
	Service      string              `json:"service"`
	DisplayName  string              `json:"display_name,omitempty"`
	ContextRepo  string              `json:"context_repo,omitempty"`
	Source       config.SourceConfig `json:"source"`
	Environments []serviceEnvSummary `json:"environments"`
}

func (s *Server) handleListServices(ctx context.Context, raw json.RawMessage) CallToolResult {
	var args struct {
		ContextRepo string `json:"context_repo"`
		Query       string `json:"query"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: " + err.Error())
	}

	matches, err := ResolveServices(s.deps.Config, "", args.ContextRepo)
	if err != nil {
		return toolError(err.Error())
	}

	query := strings.ToLower(strings.TrimSpace(args.Query))
	ra := authFromContext(ctx)
	var globalPriority []string
	if s.deps.Config != nil {
		globalPriority = s.deps.Config.EnvironmentPriority
	}

	services := make([]serviceCatalogEntry, 0, len(matches))
	for _, svc := range matches {
		if query != "" && !serviceMatchesQuery(svc, query) {
			continue
		}

		orderedEnvs := OrderedEnvironments(svc.Config, globalPriority)
		envs := make([]serviceEnvSummary, 0, len(orderedEnvs))
		for _, envName := range orderedEnvs {
			envCfg := svc.Config.Environments[envName]
			if envCfg == nil {
				continue
			}

			canDeploy := false
			permReason := "Authentication required"
			if ra.user != nil {
				canDeploy, permReason = auth.EvaluatePermission(ra.user, svc.Config, envCfg)
			}

			envs = append(envs, serviceEnvSummary{
				Environment:      envName,
				WriteStrategy:    envCfg.WriteStrategy.Mode,
				CanDeploy:        canDeploy,
				PermissionReason: permReason,
			})
		}

		services = append(services, serviceCatalogEntry{
			Service:      svc.Name,
			DisplayName:  svc.Config.DisplayName,
			ContextRepo:  svc.Config.Context.Repo,
			Source:       svc.Config.Source,
			Environments: envs,
		})
	}

	return toolJSON(map[string]interface{}{
		"count":    len(services),
		"services": services,
	})
}

func serviceMatchesQuery(svc ResolvedService, query string) bool {
	if strings.Contains(strings.ToLower(svc.Name), query) {
		return true
	}
	if svc.Config == nil {
		return false
	}
	return strings.Contains(strings.ToLower(svc.Config.DisplayName), query) ||
		strings.Contains(strings.ToLower(svc.Config.Context.Repo), query) ||
		strings.Contains(strings.ToLower(svc.Config.Source.Repository), query)
}

func (s *Server) handleGetHistory(ctx context.Context, raw json.RawMessage) CallToolResult {
	var args struct {
		Service     string `json:"service"`
		ContextRepo string `json:"context_repo"`
		Environment string `json:"environment"`
		Limit       *int   `json:"limit"`
		Offset      int    `json:"offset"`
		Since       string `json:"since"`
		Until       string `json:"until"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: " + err.Error())
	}

	svcName, svcCfg, err := ResolveSingleService(s.deps.Config, args.Service, args.ContextRepo)
	if err != nil {
		return toolError(err.Error())
	}

	var resolvedEnv string
	if strings.TrimSpace(args.Environment) != "" {
		resolvedEnv, _, err = ResolveEnvironment(svcName, svcCfg, args.Environment)
		if err != nil {
			return toolError(err.Error())
		}
	}

	if args.Offset < 0 {
		return toolError("'offset' must be >= 0")
	}
	limit := 20
	if args.Limit != nil {
		if *args.Limit < 1 {
			return toolError("'limit' must be >= 1")
		}
		limit = *args.Limit
		if limit > 100 {
			limit = 100
		}
	}

	var sinceTime, untilTime time.Time
	hasSince := strings.TrimSpace(args.Since) != ""
	hasUntil := strings.TrimSpace(args.Until) != ""
	if hasSince {
		sinceTime, err = parseHistoryTime(args.Since, false)
		if err != nil {
			return toolError("invalid 'since': " + err.Error())
		}
	}
	if hasUntil {
		untilTime, err = parseHistoryTime(args.Until, true)
		if err != nil {
			return toolError("invalid 'until': " + err.Error())
		}
	}
	if hasSince && hasUntil && sinceTime.After(untilTime) {
		return toolError("'since' cannot be after 'until'")
	}

	var rawEvents []store.DeploymentEvent
	if s.deps.Store != nil {
		if resolvedEnv != "" {
			rawEvents = s.deps.Store.GetEnvironmentHistory(svcName, resolvedEnv)
		} else {
			var globalPriority []string
			if s.deps.Config != nil {
				globalPriority = s.deps.Config.EnvironmentPriority
			}
			for _, envName := range OrderedEnvironments(svcCfg, globalPriority) {
				rawEvents = append(rawEvents, s.deps.Store.GetEnvironmentHistory(svcName, envName)...)
			}
		}
	}

	filtered := make([]store.DeploymentEvent, 0, len(rawEvents))
	for _, evt := range rawEvents {
		if hasSince && evt.DeployedAt.Before(sinceTime) {
			continue
		}
		if hasUntil && evt.DeployedAt.After(untilTime) {
			continue
		}
		filtered = append(filtered, evt)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].DeployedAt.After(filtered[j].DeployedAt)
	})

	total := len(filtered)
	page := []store.DeploymentEvent{}
	if args.Offset < total {
		end := args.Offset + limit
		if end > total {
			end = total
		}
		page = filtered[args.Offset:end]
	}

	return toolJSON(map[string]interface{}{
		"service":      svcName,
		"display_name": svcCfg.DisplayName,
		"context_repo": svcCfg.Context.Repo,
		"environment":  resolvedEnv,
		"total":        total,
		"offset":       args.Offset,
		"limit":        limit,
		"has_more":     args.Offset+len(page) < total,
		"events":       page,
	})
}

func parseHistoryTime(raw string, endOfDay bool) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		t = t.UTC()
		if endOfDay {
			return t.Add(24*time.Hour - time.Nanosecond), nil
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 timestamp or YYYY-MM-DD date, got %q", raw)
}

type deployedEnvView struct {
	Repo             string                  `json:"repo,omitempty"`
	Branch           string                  `json:"branch,omitempty"`
	ActiveVersion    string                  `json:"active_version"`
	LastDeployed     *time.Time              `json:"last_deployed,omitempty"`
	DeployedBy       string                  `json:"deployed_by,omitempty"`
	CommitSHA        string                  `json:"commit_sha,omitempty"`
	PRNumber         int                     `json:"pr_number,omitempty"`
	WriteStrategy    string                  `json:"write_strategy"`
	TargetFile       string                  `json:"target_file"`
	TargetPath       string                  `json:"target_path"`
	PendingPR        *store.PendingPR        `json:"pending_pr,omitempty"`
	ClusterStatus    *cluster.Status         `json:"cluster_status,omitempty"`
	CanDeploy        bool                    `json:"can_deploy"`
	PermissionReason string                  `json:"permission_reason,omitempty"`
	RecentEvents     []store.DeploymentEvent `json:"recent_events,omitempty"`
}

type topEnvSummary struct {
	Environment    string           `json:"environment"`
	ActiveVersion  string           `json:"active_version"`
	LatestVersion  string           `json:"latest_version,omitempty"`
	VersionsBehind int              `json:"versions_behind"`
	IsUpToDate     bool             `json:"is_up_to_date"`
	PendingPR      *store.PendingPR `json:"pending_pr,omitempty"`
}

type deployedServiceView struct {
	Service             string                     `json:"service"`
	DisplayName         string                     `json:"display_name,omitempty"`
	ContextRepo         string                     `json:"context_repo,omitempty"`
	GitOpsRepo          string                     `json:"gitops_repo,omitempty"`
	Source              config.SourceConfig        `json:"source"`
	EnvironmentPriority []string                   `json:"environment_priority,omitempty"`
	LatestVersion       string                     `json:"latest_version,omitempty"`
	TopEnvStatus        *topEnvSummary             `json:"top_env_status,omitempty"`
	Environments        map[string]deployedEnvView `json:"environments"`
}

func (s *Server) handleGetDeployed(ctx context.Context, raw json.RawMessage) CallToolResult {
	var args struct {
		Service              string `json:"service"`
		ContextRepo          string `json:"context_repo"`
		Environment          string `json:"environment"`
		IncludeClusterStatus *bool  `json:"include_cluster_status"`
		IncludeHistory       bool   `json:"include_history"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: " + err.Error())
	}

	matches, err := ResolveServices(s.deps.Config, args.Service, args.ContextRepo)
	if err != nil {
		return toolError(err.Error())
	}

	envFilter := strings.TrimSpace(args.Environment)
	if len(matches) == 1 && envFilter != "" {
		if _, _, err := ResolveEnvironment(matches[0].Name, matches[0].Config, envFilter); err != nil {
			return toolError(err.Error())
		}
	}

	includeCluster := strings.TrimSpace(args.Service) != "" || strings.TrimSpace(args.ContextRepo) != ""
	if args.IncludeClusterStatus != nil {
		includeCluster = *args.IncludeClusterStatus
	}

	ra := authFromContext(ctx)
	var globalPriority []string
	var defaultGitOpsRepo string
	if s.deps.Config != nil {
		globalPriority = s.deps.Config.EnvironmentPriority
		defaultGitOpsRepo = s.deps.Config.GitOps.Repo
	}

	services := make([]deployedServiceView, 0, len(matches))
	for _, svc := range matches {
		orderedEnvs := OrderedEnvironments(svc.Config, globalPriority)
		allEnvViews := make(map[string]deployedEnvView, len(orderedEnvs))
		filteredEnvViews := make(map[string]deployedEnvView)

		for _, envName := range orderedEnvs {
			envCfg := svc.Config.Environments[envName]
			if envCfg == nil {
				continue
			}

			var activeEvt *store.DeploymentEvent
			var recentEvts []store.DeploymentEvent
			if s.deps.Store != nil {
				activeEvt = s.deps.Store.GetActiveEvent(svc.Name, envName)
				if args.IncludeHistory {
					recentEvts = s.deps.Store.GetEnvironmentHistory(svc.Name, envName)
					if len(recentEvts) > 10 {
						recentEvts = recentEvts[:10]
					}
				}
			}

			repo := envCfg.Repo
			if repo == "" {
				repo = defaultGitOpsRepo
			}

			view := deployedEnvView{
				Repo:          repo,
				Branch:        envCfg.Branch,
				WriteStrategy: envCfg.WriteStrategy.Mode,
				TargetFile:    envCfg.Target.File,
				TargetPath:    envCfg.Target.Path,
				RecentEvents:  recentEvts,
			}

			if activeEvt != nil {
				view.ActiveVersion = activeEvt.Version
				view.LastDeployed = &activeEvt.DeployedAt
				view.DeployedBy = activeEvt.AuthorName
				view.CommitSHA = activeEvt.CommitSHA
				view.PRNumber = activeEvt.PRNumber
			} else {
				view.ActiveVersion = "unknown"
			}

			view.PendingPR = s.reconciledPendingPR(ctx, svc.Name, envName, envCfg, activeEvt)

			if ra.user != nil {
				canDeploy, reason := auth.EvaluatePermission(ra.user, svc.Config, envCfg)
				view.CanDeploy = canDeploy
				view.PermissionReason = reason
			} else {
				view.CanDeploy = false
				view.PermissionReason = "Authentication required"
			}

			if includeCluster && (envFilter == "" || strings.EqualFold(envName, envFilter)) {
				activeVer := ""
				if activeEvt != nil {
					activeVer = activeEvt.Version
				}
				view.ClusterStatus = s.resolveClusterStatus(ctx, svc.Name, envName, activeVer, svc.Config, envCfg)
			}

			allEnvViews[envName] = view
			if envFilter == "" || strings.EqualFold(envName, envFilter) {
				filteredEnvViews[envName] = view
			}
		}

		if envFilter != "" && len(filteredEnvViews) < 1 {
			continue
		}

		envPriority := svc.Config.EnvironmentPriority
		if len(envPriority) < 1 {
			envPriority = globalPriority
		}

		topStatus := s.computeTopEnvSummary(svc.Name, orderedEnvs, allEnvViews)
		latestVer := ""
		if topStatus != nil {
			latestVer = topStatus.LatestVersion
		}

		services = append(services, deployedServiceView{
			Service:             svc.Name,
			DisplayName:         svc.Config.DisplayName,
			ContextRepo:         svc.Config.Context.Repo,
			GitOpsRepo:          defaultGitOpsRepo,
			Source:              svc.Config.Source,
			EnvironmentPriority: envPriority,
			LatestVersion:       latestVer,
			TopEnvStatus:        topStatus,
			Environments:        filteredEnvViews,
		})
	}

	if envFilter != "" && len(services) < 1 {
		return toolError(fmt.Sprintf("environment '%s' not found on any matching service", envFilter))
	}

	return toolJSON(map[string]interface{}{
		"count":    len(services),
		"services": services,
	})
}

type queuedVersionView struct {
	Version     string     `json:"version"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
	Prerelease  bool       `json:"prerelease,omitempty"`
	Description string     `json:"description,omitempty"`
	Changelog   string     `json:"changelog,omitempty"`
}

type queueEnvView struct {
	ActiveVersion   string              `json:"active_version"`
	LatestVersion   string              `json:"latest_version,omitempty"`
	IsUpToDate      bool                `json:"is_up_to_date"`
	VersionsBehind  int                 `json:"versions_behind"`
	PendingPR       *store.PendingPR    `json:"pending_pr,omitempty"`
	InFlightRollout bool                `json:"in_flight_rollout"`
	ClusterStatus   *cluster.Status     `json:"cluster_status,omitempty"`
	QueuedVersions  []queuedVersionView `json:"queued_versions"`
}

type queueServiceView struct {
	Service       string                  `json:"service"`
	DisplayName   string                  `json:"display_name,omitempty"`
	ContextRepo   string                  `json:"context_repo,omitempty"`
	LatestVersion string                  `json:"latest_version,omitempty"`
	Environments  map[string]queueEnvView `json:"environments"`
}

func (s *Server) handleGetDeployQueue(ctx context.Context, raw json.RawMessage) CallToolResult {
	var args struct {
		Service     string `json:"service"`
		ContextRepo string `json:"context_repo"`
		Environment string `json:"environment"`
		OnlyQueued  bool   `json:"only_queued"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: " + err.Error())
	}

	matches, err := ResolveServices(s.deps.Config, args.Service, args.ContextRepo)
	if err != nil {
		return toolError(err.Error())
	}

	envFilter := strings.TrimSpace(args.Environment)
	if len(matches) == 1 && envFilter != "" {
		if _, _, err := ResolveEnvironment(matches[0].Name, matches[0].Config, envFilter); err != nil {
			return toolError(err.Error())
		}
	}

	var globalPriority []string
	if s.deps.Config != nil {
		globalPriority = s.deps.Config.EnvironmentPriority
	}

	checker := s.getClusterChecker()
	hasEnvMatch := false
	totalPendingPRs := 0
	servicesBehindLatest := 0
	outServices := make([]queueServiceView, 0, len(matches))

	for _, svc := range matches {
		versions := s.fetchServiceVersions(ctx, svc.Name, svc.Config)
		latestVer := ""
		if len(versions) > 0 {
			latestVer = versions[0].Version
		}

		orderedEnvs := OrderedEnvironments(svc.Config, globalPriority)
		envViews := make(map[string]queueEnvView)
		svcBehind := false

		for _, envName := range orderedEnvs {
			if envFilter != "" && !strings.EqualFold(envName, envFilter) {
				continue
			}
			envCfg := svc.Config.Environments[envName]
			if envCfg == nil {
				continue
			}
			hasEnvMatch = true

			var activeEvt *store.DeploymentEvent
			if s.deps.Store != nil {
				activeEvt = s.deps.Store.GetActiveEvent(svc.Name, envName)
			}

			activeVer := "unknown"
			if activeEvt != nil && activeEvt.Version != "" {
				activeVer = activeEvt.Version
			}

			pendingPR := s.reconciledPendingPR(ctx, svc.Name, envName, envCfg, activeEvt)

			var clusterSt *cluster.Status
			inFlight := false
			if checker != nil && checker.IsAvailable() {
				expected := ""
				if activeEvt != nil {
					expected = activeEvt.Version
				}
				clusterSt = s.resolveClusterStatus(ctx, svc.Name, envName, expected, svc.Config, envCfg)
				if clusterSt != nil && (clusterSt.Status == "pending" || clusterSt.Status == "progressing") {
					inFlight = true
				}
			}

			queued, behind, upToDate := computeQueuedVersions(activeVer, latestVer, versions)

			if args.OnlyQueued && pendingPR == nil && !inFlight && behind < 1 {
				continue
			}

			if pendingPR != nil {
				totalPendingPRs++
			}
			if behind > 0 {
				svcBehind = true
			}

			envViews[envName] = queueEnvView{
				ActiveVersion:   activeVer,
				LatestVersion:   latestVer,
				IsUpToDate:      upToDate,
				VersionsBehind:  behind,
				PendingPR:       pendingPR,
				InFlightRollout: inFlight,
				ClusterStatus:   clusterSt,
				QueuedVersions:  queued,
			}
		}

		if len(envViews) < 1 {
			continue
		}
		if svcBehind {
			servicesBehindLatest++
		}

		outServices = append(outServices, queueServiceView{
			Service:       svc.Name,
			DisplayName:   svc.Config.DisplayName,
			ContextRepo:   svc.Config.Context.Repo,
			LatestVersion: latestVer,
			Environments:  envViews,
		})
	}

	if envFilter != "" && !hasEnvMatch {
		return toolError(fmt.Sprintf("environment '%s' not found on any matching service", envFilter))
	}

	return toolJSON(map[string]interface{}{
		"total_services":         len(outServices),
		"total_pending_prs":      totalPendingPRs,
		"services_behind_latest": servicesBehindLatest,
		"services":               outServices,
	})
}

func computeQueuedVersions(activeVer, latestVer string, versions []provider.VersionInfo) ([]queuedVersionView, int, bool) {
	if len(versions) < 1 {
		return make([]queuedVersionView, 0), 0, activeVer != "" && activeVer != "unknown" && AreVersionsEqual(activeVer, latestVer)
	}

	var matched []provider.VersionInfo
	versionsBehind := 0
	isUpToDate := false

	if activeVer != "" && activeVer != "unknown" {
		if AreVersionsEqual(activeVer, latestVer) {
			return make([]queuedVersionView, 0), 0, true
		}

		foundIdx := -1
		for idx, v := range versions {
			if AreVersionsEqual(v.Version, activeVer) {
				foundIdx = idx
				break
			}
		}

		if foundIdx > -1 {
			matched = versions[:foundIdx]
			versionsBehind = foundIdx
			isUpToDate = foundIdx == 0
		} else {
			activeTag := ExtractVersionTag(activeVer)
			for _, v := range versions {
				if provider.CompareVersions(v.Version, activeTag) > 0 {
					matched = append(matched, v)
				}
			}
			versionsBehind = len(matched)
			isUpToDate = len(matched) < 1
		}
	} else {
		matched = versions
		versionsBehind = len(versions)
		isUpToDate = false
	}

	capCount := len(matched)
	if capCount > 20 {
		capCount = 20
	}

	out := make([]queuedVersionView, 0, capCount)
	for i := 0; i < capCount; i++ {
		v := matched[i]
		changelog := v.Changelog
		if len(changelog) > 600 {
			changelog = changelog[:600] + "..."
		}
		out = append(out, queuedVersionView{
			Version:     v.Version,
			ReleasedAt:  v.ReleasedAt,
			Prerelease:  v.Prerelease,
			Description: v.Description,
			Changelog:   changelog,
		})
	}
	return out, versionsBehind, isUpToDate
}

func (s *Server) handleGetVersionDiff(ctx context.Context, raw json.RawMessage) CallToolResult {
	var args struct {
		Service     string `json:"service"`
		ContextRepo string `json:"context_repo"`
		Environment string `json:"environment"`
		FromVersion string `json:"from_version"`
		ToVersion   string `json:"to_version"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: " + err.Error())
	}

	svcName, svcCfg, err := ResolveSingleService(s.deps.Config, args.Service, args.ContextRepo)
	if err != nil {
		return toolError(err.Error())
	}

	fromRef := strings.TrimSpace(args.FromVersion)
	if fromRef == "" {
		var globalPriority []string
		if s.deps.Config != nil {
			globalPriority = s.deps.Config.EnvironmentPriority
		}
		if strings.TrimSpace(args.Environment) != "" {
			envName, _, err := ResolveEnvironment(svcName, svcCfg, args.Environment)
			if err != nil {
				return toolError(err.Error())
			}
			if s.deps.Store != nil {
				if evt := s.deps.Store.GetActiveEvent(svcName, envName); evt != nil {
					fromRef = evt.Version
				}
			}
		} else {
			orderedEnvs := OrderedEnvironments(svcCfg, globalPriority)
			if len(orderedEnvs) > 0 && s.deps.Store != nil {
				if evt := s.deps.Store.GetActiveEvent(svcName, orderedEnvs[0]); evt != nil {
					fromRef = evt.Version
				}
			}
		}
	}
	if fromRef == "unknown" {
		fromRef = ""
	}

	toRef := strings.TrimSpace(args.ToVersion)
	if toRef == "" || strings.EqualFold(toRef, "latest") {
		versions := s.fetchServiceVersions(ctx, svcName, svcCfg)
		if len(versions) < 1 {
			return toolError(fmt.Sprintf("no candidate versions found for service '%s'", svcName))
		}
		toRef = versions[0].Version
	}

	cleanFrom := git.CleanVersionRef(fromRef)
	cleanTo := git.CleanVersionRef(toRef)
	if cleanFrom != "" && (cleanFrom == cleanTo || strings.TrimPrefix(cleanFrom, "v") == strings.TrimPrefix(cleanTo, "v")) {
		return toolJSON(map[string]interface{}{
			"service":      svcName,
			"context_repo": svcCfg.Context.Repo,
			"diff": &git.ChangelogDiff{
				FromVersion: fromRef,
				ToVersion:   toRef,
				Commits:     []git.DiffEntry{},
				DiffStat:    "0 files changed",
			},
		})
	}

	repoURL := strings.TrimSpace(svcCfg.Context.Repo)
	if repoURL == "" {
		return toolJSON(map[string]interface{}{
			"service":      svcName,
			"context_repo": "",
			"diff": &git.ChangelogDiff{
				FromVersion: fromRef,
				ToVersion:   toRef,
				Commits:     []git.DiffEntry{},
				DiffStat:    "0 files changed",
			},
			"note": "no context.repo configured for this service",
		})
	}

	if s.deps.Indexer == nil {
		return toolError("git indexer is not initialized")
	}

	cloneURL := repoURL
	if !strings.HasPrefix(cloneURL, "https://") && !strings.HasPrefix(cloneURL, "http://") && !strings.HasPrefix(cloneURL, "git@") && !strings.HasPrefix(cloneURL, "/") && !strings.HasPrefix(cloneURL, "./") && !strings.HasPrefix(cloneURL, "../") {
		cloneURL = "https://github.com/" + strings.TrimSuffix(cloneURL, ".git") + ".git"
	}

	repoDir := s.deps.Indexer.GetRepoPath(repoURL)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		if err := git.CloneOrFetch(ctx, cloneURL, "", repoDir, true); err != nil {
			return toolError(fmt.Sprintf("failed to fetch context repository '%s': %v", repoURL, err))
		}
	} else {
		_ = git.CloneOrFetch(ctx, cloneURL, "", repoDir, true)
	}

	diff, err := git.GetVersionDiff(ctx, repoDir, fromRef, toRef, svcCfg.Context.GetTagPrefix())
	if err != nil || diff == nil {
		msg := "no diff available"
		if err != nil {
			msg = err.Error()
		}
		return toolError(fmt.Sprintf("failed to compute version diff from '%s' to '%s': %s", fromRef, toRef, msg))
	}

	return toolJSON(map[string]interface{}{
		"service":      svcName,
		"context_repo": svcCfg.Context.Repo,
		"diff":         diff,
	})
}

func (s *Server) handlePromote(ctx context.Context, raw json.RawMessage) CallToolResult {
	var args struct {
		Service        string `json:"service"`
		ContextRepo    string `json:"context_repo"`
		Environment    string `json:"environment"`
		TargetVersion  string `json:"target_version"`
		Mode           string `json:"mode"`
		Comment        string `json:"comment"`
		Watch          bool   `json:"watch"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: " + err.Error())
	}

	targetVersion := strings.TrimSpace(args.TargetVersion)
	if targetVersion == "" {
		return toolError("target_version is required")
	}
	if strings.TrimSpace(args.Service) == "" && strings.TrimSpace(args.ContextRepo) == "" {
		return toolError("either 'service' or 'context_repo' is required")
	}

	mode := strings.TrimSpace(args.Mode)
	if mode != "" && mode != "direct" && mode != "pull_request" {
		return toolError("mode must be 'direct' or 'pull_request'")
	}

	if s.deps.Deployer == nil {
		return toolError("deployer is not initialized")
	}

	svcName, svcCfg, err := ResolveSingleService(s.deps.Config, args.Service, args.ContextRepo)
	if err != nil {
		return toolError(err.Error())
	}

	envName, envCfg, err := ResolveEnvironment(svcName, svcCfg, args.Environment)
	if err != nil {
		return toolError(err.Error())
	}

	ra := authFromContext(ctx)
	allowed, reason := auth.EvaluatePermission(ra.user, svcCfg, envCfg)
	if !allowed {
		return toolError("Forbidden: " + reason)
	}

	if strings.EqualFold(targetVersion, "latest") {
		versions := s.fetchServiceVersions(ctx, svcName, svcCfg)
		if len(versions) < 1 {
			return toolError(fmt.Sprintf("unable to resolve 'latest' version for service '%s': no candidate versions found", svcName))
		}
		targetVersion = versions[0].Version
	}

	req := deployer.PromotionRequest{
		Service:       svcName,
		Environment:   envName,
		TargetVersion: targetVersion,
		Mode:          mode,
		Comment:       strings.TrimSpace(args.Comment),
	}
	if ra.user != nil {
		if ra.user.Name != "" {
			req.User = ra.user.Name
		} else {
			req.User = ra.user.Username
		}
		if ra.user.Email != "" {
			req.Email = ra.user.Email
		}
		if ra.user.Token != "" {
			req.GitHubToken = ra.user.Token
		}
	} else if ra.forwardedUser != "" {
		req.User = ra.forwardedUser
	}

	if s.deps.Dispatcher != nil {
		s.deps.Dispatcher.Dispatch(webhook.EventDeploymentRequested, webhook.DeploymentRequestedEvent{
			Event:         webhook.EventDeploymentRequested,
			Service:       req.Service,
			Environment:   req.Environment,
			TargetVersion: req.TargetVersion,
			User:          req.User,
			Timestamp:     time.Now(),
		}, req.Service)
	}

	result, err := s.deps.Deployer.Promote(ctx, req)
	if err != nil {
		return toolError("promotion failed: " + err.Error())
	}

	if result.Mode == "pull_request" && s.deps.Store != nil {
		s.deps.Store.SetPendingPR(req.Service, req.Environment, &store.PendingPR{
			Service:       req.Service,
			Environment:   req.Environment,
			TargetVersion: req.TargetVersion,
			Branch:        result.Branch,
			PRNumber:      result.PRNumber,
			PRURL:         result.PRURL,
			User:          req.User,
			CreatedAt:     time.Now(),
		})
	} else if result.Mode == "direct" && s.deps.Indexer != nil {
		//keep store immediately synchronized for follow-up inspections
		_ = s.deps.Indexer.ReindexService(ctx, svcName)
	}

	if s.deps.Dispatcher != nil {
		s.deps.Dispatcher.Dispatch(webhook.EventDeploymentCompleted, webhook.DeploymentCompletedEvent{
			Event:         webhook.EventDeploymentCompleted,
			Service:       req.Service,
			Environment:   req.Environment,
			TargetVersion: req.TargetVersion,
			User:          req.User,
			CommitSHA:     result.CommitSHA,
			PRNumber:      result.PRNumber,
			PRURL:         result.PRURL,
			Timestamp:     time.Now(),
		}, req.Service)
	}

	var finalStatus *cluster.Status
	checker := s.getClusterChecker()
	if args.Watch && result.Mode == "direct" && checker != nil && checker.IsAvailable() {
		timeoutSec := args.TimeoutSeconds
		if timeoutSec < 1 {
			timeoutSec = 300
		}
		if timeoutSec > 900 {
			timeoutSec = 900
		}

		var watchErr error
		finalStatus, watchErr = s.watchDeployment(ctx, svcName, envName, targetVersion, svcCfg, envCfg, timeoutSec, 2*time.Second)
		if watchErr != nil {
			return toolJSONWithError(map[string]interface{}{
				"service":      svcName,
				"context_repo": svcCfg.Context.Repo,
				"environment":  envName,
				"promotion":    result,
				"deployment":   finalStatus,
				"watch_error":  watchErr.Error(),
			})
		}
	}

	return toolJSON(map[string]interface{}{
		"service":      svcName,
		"context_repo": svcCfg.Context.Repo,
		"environment":  envName,
		"promotion":    result,
		"deployment":   finalStatus,
	})
}

func (s *Server) computeTopEnvSummary(svcName string, orderedEnvs []string, envViews map[string]deployedEnvView) *topEnvSummary {
	if len(envViews) < 1 || len(orderedEnvs) < 1 {
		return nil
	}

	topEnvName := ""
	for _, name := range orderedEnvs {
		if _, ok := envViews[name]; ok {
			topEnvName = name
			break
		}
	}
	if topEnvName == "" {
		return nil
	}
	topView := envViews[topEnvName]

	var cachedVersions []string
	if s.deps.Store != nil {
		cachedVersions = s.deps.Store.GetVersions(svcName)
	}

	latestVersion := ""
	if len(cachedVersions) > 0 {
		latestVersion = cachedVersions[0]
	}

	versionsBehind := 0
	isUpToDate := false
	activeVer := topView.ActiveVersion

	if activeVer != "" && activeVer != "unknown" && latestVersion != "" {
		if AreVersionsEqual(activeVer, latestVersion) {
			isUpToDate = true
		} else if len(cachedVersions) > 0 {
			foundIdx := -1
			for idx, v := range cachedVersions {
				if AreVersionsEqual(v, activeVer) {
					foundIdx = idx
					break
				}
			}
			if foundIdx > -1 {
				versionsBehind = foundIdx
				isUpToDate = foundIdx == 0
			} else {
				versionsBehind = len(cachedVersions)
			}
		}
	}

	return &topEnvSummary{
		Environment:    topEnvName,
		ActiveVersion:  activeVer,
		LatestVersion:  latestVersion,
		VersionsBehind: versionsBehind,
		IsUpToDate:     isUpToDate,
		PendingPR:      topView.PendingPR,
	}
}

func (s *Server) fetchServiceVersions(ctx context.Context, svcName string, svcCfg *config.ServiceConfig) []provider.VersionInfo {
	if svcCfg == nil {
		return nil
	}

	if strings.TrimSpace(svcCfg.Source.Repository) != "" {
		prov := provider.NewProvider(svcCfg.Source)
		versions, err := prov.GetVersions(ctx)
		if err == nil && len(versions) > 0 {
			s.enrichVersionsFromContext(ctx, versions, svcCfg)
			versions = provider.DeduplicateVersionsByDigest(versions)
			if s.deps.Store != nil {
				verStrs := make([]string, 0, len(versions))
				for _, v := range versions {
					verStrs = append(verStrs, v.Version)
				}
				s.deps.Store.SetVersions(svcName, verStrs)
			}
			return versions
		}
	}

	if s.deps.Store != nil {
		cached := s.deps.Store.GetVersions(svcName)
		if len(cached) > 0 {
			out := make([]provider.VersionInfo, 0, len(cached))
			for _, v := range cached {
				out = append(out, provider.VersionInfo{Version: v})
			}
			return out
		}
	}

	return nil
}

func (s *Server) enrichVersionsFromContext(ctx context.Context, versions []provider.VersionInfo, svcCfg *config.ServiceConfig) {
	if svcCfg == nil || len(versions) < 1 {
		return
	}
	contextRepo := strings.TrimSpace(svcCfg.Context.Repo)
	if !isGitHubContextRepo(contextRepo) {
		return
	}
	tagPrefix := svcCfg.Context.GetTagPrefix()

	s.contextVersionsMu.Lock()
	ctxVersions, found := s.contextVersionsCache[contextRepo]
	s.contextVersionsMu.Unlock()

	if !found {
		enrichCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ctxProv := provider.NewProvider(config.SourceConfig{
			Type:       "github_releases",
			Repository: contextRepo,
			TrimPrefix: tagPrefix,
		})
		fetched, err := ctxProv.GetVersions(enrichCtx)
		cancel()
		if err == nil && len(fetched) > 0 {
			ctxVersions = fetched
			s.contextVersionsMu.Lock()
			s.contextVersionsCache[contextRepo] = ctxVersions
			s.contextVersionsMu.Unlock()
		}
	}

	if len(ctxVersions) < 1 {
		return
	}

	relMap := make(map[string]provider.VersionInfo, len(ctxVersions)*2)
	for _, cv := range ctxVersions {
		relMap[cv.Version] = cv
		relMap[strings.TrimPrefix(cv.Version, "v")] = cv
		if tagPrefix != "" {
			relMap[strings.TrimPrefix(cv.Version, tagPrefix)] = cv
		}
	}

	for i := range versions {
		v := &versions[i]
		cleanV := strings.TrimPrefix(v.Version, "v")
		match, ok := relMap[v.Version]
		if !ok {
			match, ok = relMap[cleanV]
		}
		if !ok && tagPrefix != "" {
			match, ok = relMap[tagPrefix+v.Version]
		}
		if ok {
			if v.Description == "" {
				v.Description = match.Description
			}
			if v.Changelog == "" {
				v.Changelog = match.Changelog
			}
			if v.ReleasedAt == nil {
				v.ReleasedAt = match.ReleasedAt
			}
			v.Prerelease = v.Prerelease || match.Prerelease
		}
	}
}

func isGitHubContextRepo(repo string) bool {
	if repo == "" {
		return false
	}
	if strings.Contains(repo, "github.com") {
		return true
	}
	if strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../") {
		return false
	}
	if strings.Contains(repo, "://") || strings.Contains(repo, ":") || strings.Contains(repo, "gitlab") {
		return false
	}
	return strings.Count(repo, "/") == 1
}

func (s *Server) reconciledPendingPR(ctx context.Context, svcName, envName string, envCfg *config.EnvironmentConfig, activeEvt *store.DeploymentEvent) *store.PendingPR {
	if s.deps.Store == nil {
		return nil
	}
	pendingPR := s.deps.Store.GetPendingPR(svcName, envName)
	if pendingPR == nil {
		return nil
	}

	targetRepo := envCfg.Repo
	if targetRepo == "" && s.deps.Config != nil {
		targetRepo = s.deps.Config.GitOps.Repo
	}

	if activeEvt != nil && AreVersionsEqual(activeEvt.Version, pendingPR.TargetVersion) {
		s.deps.Store.ClearPendingPR(svcName, envName)
		return nil
	}
	if pendingPR.PRNumber > 0 && isPRClosed(ctx, targetRepo, pendingPR.PRNumber) {
		s.deps.Store.ClearPendingPR(svcName, envName)
		return nil
	}
	return pendingPR
}

func isPRClosed(ctx context.Context, repoURL string, prNumber int) bool {
	token := config.GetGitHubToken()
	if token == "" || prNumber <= 0 || repoURL == "" {
		return false
	}
	if !strings.Contains(repoURL, "github.com") {
		return false
	}

	cleanRepo := strings.TrimPrefix(repoURL, "git@github.com:")
	cleanRepo = strings.TrimPrefix(cleanRepo, "https://github.com/")
	cleanRepo = strings.TrimSuffix(cleanRepo, ".git")

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", cleanRepo, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return true
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var ghPR struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghPR); err == nil {
		return ghPR.State == "closed"
	}
	return false
}

func (s *Server) resolveClusterStatus(ctx context.Context, svcName, envName, expectedVersion string, svcCfg *config.ServiceConfig, envCfg *config.EnvironmentConfig) *cluster.Status {
	checker := s.getClusterChecker()
	if checker == nil || !checker.IsAvailable() {
		return nil
	}

	if envCfg.StatusCheck != nil && envCfg.StatusCheck.Enabled != nil && !*envCfg.StatusCheck.Enabled {
		return nil
	}
	if svcCfg.StatusCheck != nil && svcCfg.StatusCheck.Enabled != nil && !*svcCfg.StatusCheck.Enabled {
		return nil
	}

	deployment := ""
	namespace := ""

	if envCfg.StatusCheck != nil {
		deployment = envCfg.StatusCheck.Deployment
		namespace = envCfg.StatusCheck.Namespace
	}
	if deployment == "" && svcCfg.StatusCheck != nil {
		deployment = svcCfg.StatusCheck.Deployment
	}
	if namespace == "" && svcCfg.StatusCheck != nil {
		namespace = svcCfg.StatusCheck.Namespace
	}

	if (deployment == "" || namespace == "") && envCfg.Target.File != "" && s.deps.Indexer != nil {
		repoURL := envCfg.Repo
		if repoURL == "" && s.deps.Config != nil {
			repoURL = s.deps.Config.GitOps.Repo
		}
		repoDir := s.deps.Indexer.GetRepoPath(repoURL)
		if data, err := git.ShowFileAtCommit(ctx, repoDir, envCfg.Branch, envCfg.Target.File); err == nil {
			if dName, dNs, ok := cluster.SniffDeploymentManifest(data); ok {
				if deployment == "" {
					deployment = dName
				}
				if namespace == "" && dNs != "" {
					namespace = dNs
				}
			}
		}
	}

	if deployment == "" {
		deployment = svcName
	}
	if namespace == "" {
		namespace = checker.DefaultNamespace()
	}

	st, err := checker.GetDeploymentStatus(ctx, namespace, deployment, expectedVersion)
	if err != nil {
		return nil
	}
	return st
}

func (s *Server) watchDeployment(ctx context.Context, svcName, envName, targetVer string, svcCfg *config.ServiceConfig, envCfg *config.EnvironmentConfig, timeoutSec int, pollInterval time.Duration) (*cluster.Status, error) {
	watchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var lastStatus *cluster.Status
	for {
		st := s.resolveClusterStatus(watchCtx, svcName, envName, targetVer, svcCfg, envCfg)
		if st != nil {
			lastStatus = st
			if st.Status == "ready" {
				return st, nil
			}
			if st.Status == "degraded" || st.Status == "error" {
				return st, fmt.Errorf("deployment %s: %s", st.Status, st.Message)
			}
		}

		select {
		case <-watchCtx.Done():
			if lastStatus != nil {
				return lastStatus, fmt.Errorf("timed out waiting for deployment to reach ready state (status: %s, message: %s)", lastStatus.Status, lastStatus.Message)
			}
			return nil, fmt.Errorf("timed out waiting for deployment to reach ready state")
		case <-ticker.C:
		}
	}
}

func toolJSON(v interface{}) CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolError("failed to marshal response: " + err.Error())
	}
	return CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(b)}},
	}
}

func toolJSONWithError(v interface{}) CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolError("failed to marshal response: " + err.Error())
	}
	return CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(b)}},
		IsError: true,
	}
}

func toolError(msg string) CallToolResult {
	return CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}
