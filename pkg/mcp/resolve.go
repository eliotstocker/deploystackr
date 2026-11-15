package mcp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"deploystackr/pkg/config"
)

type ResolvedService struct {
	Name   string
	Config *config.ServiceConfig
}

func NormalizeRepoSlug(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) < 1 {
		return ""
	}

	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return filepath.Clean(s)
	}

	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimRight(s, "/")

	if !strings.Contains(s, "://") {
		//strip scp-style user@host: prefix such as git@github.com:org/repo
		if colonIdx := strings.Index(s, ":"); colonIdx > -1 && strings.Contains(s[:colonIdx], "@") {
			s = s[colonIdx+1:]
		}
	} else if u, err := url.Parse(s); err == nil && u.Path != "" {
		s = u.Path
	}

	s = strings.TrimPrefix(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) >= 3 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		s = strings.Join(parts[1:], "/")
	}

	return strings.ToLower(strings.TrimSpace(s))
}

func OrderedServiceNames(cfg *config.Config) []string {
	if cfg == nil || len(cfg.Services) < 1 {
		return nil
	}

	ordered := make([]string, 0, len(cfg.Services))
	seen := make(map[string]bool, len(cfg.Services))

	for _, name := range cfg.ServiceOrder {
		if _, ok := cfg.Services[name]; ok && !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}

	var remaining []string
	for name := range cfg.Services {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	ordered = append(ordered, remaining...)
	return ordered
}

func OrderedEnvironments(svcCfg *config.ServiceConfig, globalPriority []string) []string {
	if svcCfg == nil || len(svcCfg.Environments) < 1 {
		return nil
	}

	priority := svcCfg.EnvironmentPriority
	if len(priority) < 1 {
		priority = globalPriority
	}

	ordered := make([]string, 0, len(svcCfg.Environments))
	seen := make(map[string]bool, len(svcCfg.Environments))

	var allKeys []string
	for name := range svcCfg.Environments {
		allKeys = append(allKeys, name)
	}
	sort.Strings(allKeys)

	for _, p := range priority {
		target := strings.TrimSpace(p)
		for _, name := range allKeys {
			if !seen[name] && strings.EqualFold(strings.TrimSpace(name), target) {
				ordered = append(ordered, name)
				seen[name] = true
				break
			}
		}
	}

	for _, name := range allKeys {
		if !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}

	return ordered
}

func ResolveServices(cfg *config.Config, service, contextRepo string) ([]ResolvedService, error) {
	service = strings.TrimSpace(service)
	contextRepo = strings.TrimSpace(contextRepo)

	if cfg == nil || len(cfg.Services) < 1 {
		if service != "" || contextRepo != "" {
			return nil, fmt.Errorf("no services are configured")
		}
		return []ResolvedService{}, nil
	}

	ordered := OrderedServiceNames(cfg)

	if service == "" && contextRepo == "" {
		res := make([]ResolvedService, 0, len(ordered))
		for _, name := range ordered {
			res = append(res, ResolvedService{Name: name, Config: cfg.Services[name]})
		}
		return res, nil
	}

	if service != "" {
		matched, matchErr := findServiceByName(cfg, ordered, service)
		if matchErr != nil {
			return nil, matchErr
		}
		if matched != nil {
			if contextRepo != "" {
				repoToCheck := matched.Config.Context.Repo
				if repoToCheck == "" {
					repoToCheck = matched.Config.Source.Repository
				}
				if NormalizeRepoSlug(repoToCheck) != NormalizeRepoSlug(contextRepo) {
					return nil, fmt.Errorf("service '%s' has context_repo '%s', which does not match requested context_repo '%s'", matched.Name, repoToCheck, contextRepo)
				}
			}
			return []ResolvedService{*matched}, nil
		}

		if (strings.Contains(service, "/") || strings.Contains(service, ":")) && contextRepo == "" {
			return resolveByContextRepo(cfg, ordered, service)
		}

		return nil, fmt.Errorf("service '%s' not found; available services: %s", service, strings.Join(ordered, ", "))
	}

	return resolveByContextRepo(cfg, ordered, contextRepo)
}

func findServiceByName(cfg *config.Config, ordered []string, service string) (*ResolvedService, error) {
	if svcCfg, ok := cfg.Services[service]; ok {
		return &ResolvedService{Name: service, Config: svcCfg}, nil
	}

	for _, name := range ordered {
		if strings.EqualFold(name, service) {
			return &ResolvedService{Name: name, Config: cfg.Services[name]}, nil
		}
	}

	var exactDisplay []ResolvedService
	var foldDisplay []ResolvedService
	for _, name := range ordered {
		svcCfg := cfg.Services[name]
		if svcCfg == nil || strings.TrimSpace(svcCfg.DisplayName) == "" {
			continue
		}
		if svcCfg.DisplayName == service {
			exactDisplay = append(exactDisplay, ResolvedService{Name: name, Config: svcCfg})
		} else if strings.EqualFold(svcCfg.DisplayName, service) {
			foldDisplay = append(foldDisplay, ResolvedService{Name: name, Config: svcCfg})
		}
	}

	if len(exactDisplay) == 1 {
		return &exactDisplay[0], nil
	}
	if len(exactDisplay) > 1 {
		names := make([]string, 0, len(exactDisplay))
		for _, m := range exactDisplay {
			names = append(names, m.Name)
		}
		return nil, fmt.Errorf("multiple services share display_name '%s' (%s); specify service slug explicitly", service, strings.Join(names, ", "))
	}
	if len(foldDisplay) == 1 {
		return &foldDisplay[0], nil
	}
	if len(foldDisplay) > 1 {
		names := make([]string, 0, len(foldDisplay))
		for _, m := range foldDisplay {
			names = append(names, m.Name)
		}
		return nil, fmt.Errorf("multiple services share display_name '%s' (%s); specify service slug explicitly", service, strings.Join(names, ", "))
	}

	return nil, nil
}

func resolveByContextRepo(cfg *config.Config, ordered []string, contextRepo string) ([]ResolvedService, error) {
	want := NormalizeRepoSlug(contextRepo)
	if want == "" {
		return nil, fmt.Errorf("context_repo cannot be empty")
	}

	var matches []ResolvedService
	for _, name := range ordered {
		svcCfg := cfg.Services[name]
		if svcCfg != nil && svcCfg.Context.Repo != "" && NormalizeRepoSlug(svcCfg.Context.Repo) == want {
			matches = append(matches, ResolvedService{Name: name, Config: svcCfg})
		}
	}

	if len(matches) < 1 {
		for _, name := range ordered {
			svcCfg := cfg.Services[name]
			if svcCfg != nil && svcCfg.Source.Repository != "" && NormalizeRepoSlug(svcCfg.Source.Repository) == want {
				matches = append(matches, ResolvedService{Name: name, Config: svcCfg})
			}
		}
	}

	if len(matches) < 1 {
		return nil, fmt.Errorf("no service found matching context_repo '%s'", contextRepo)
	}

	return matches, nil
}

func ResolveSingleService(cfg *config.Config, service, contextRepo string) (string, *config.ServiceConfig, error) {
	if strings.TrimSpace(service) == "" && strings.TrimSpace(contextRepo) == "" {
		return "", nil, fmt.Errorf("either 'service' or 'context_repo' is required")
	}

	matches, err := ResolveServices(cfg, service, contextRepo)
	if err != nil {
		return "", nil, err
	}
	if len(matches) < 1 {
		return "", nil, fmt.Errorf("no matching service found")
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.Name)
		}
		return "", nil, fmt.Errorf("multiple services match context_repo '%s' (%s); specify 'service' explicitly", contextRepo, strings.Join(names, ", "))
	}

	return matches[0].Name, matches[0].Config, nil
}

func ResolveEnvironment(svcName string, svcCfg *config.ServiceConfig, env string) (string, *config.EnvironmentConfig, error) {
	if svcCfg == nil || len(svcCfg.Environments) < 1 {
		return "", nil, fmt.Errorf("no environments configured for service '%s'", svcName)
	}

	orderedEnvs := OrderedEnvironments(svcCfg, nil)
	env = strings.TrimSpace(env)

	if env == "" {
		if len(svcCfg.Environments) == 1 {
			sole := orderedEnvs[0]
			return sole, svcCfg.Environments[sole], nil
		}
		return "", nil, fmt.Errorf("'environment' is required for service '%s' (available: %s)", svcName, strings.Join(orderedEnvs, ", "))
	}

	if envCfg, ok := svcCfg.Environments[env]; ok {
		return env, envCfg, nil
	}

	for _, name := range orderedEnvs {
		if strings.EqualFold(name, env) {
			return name, svcCfg.Environments[name], nil
		}
	}

	return "", nil, fmt.Errorf("environment '%s' not found for service '%s' (available: %s)", env, svcName, strings.Join(orderedEnvs, ", "))
}

func ExtractVersionTag(v string) string {
	v = strings.TrimSpace(v)
	if idx := strings.LastIndex(v, ":"); idx > -1 {
		v = v[idx+1:]
	}
	if idx := strings.Index(v, "@"); idx > -1 {
		v = v[:idx]
	}
	return strings.TrimPrefix(v, "v")
}

func AreVersionsEqual(v1, v2 string) bool {
	if v1 == v2 {
		return true
	}
	t1 := ExtractVersionTag(v1)
	t2 := ExtractVersionTag(v2)
	if t1 != "" && t1 == t2 {
		return true
	}
	if (v1 != "" && strings.HasSuffix(v1, ":"+v2)) || (v2 != "" && strings.HasSuffix(v2, ":"+v1)) {
		return true
	}
	return false
}
