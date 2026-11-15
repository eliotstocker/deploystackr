package api

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"deploystackr/pkg/config"
)

func (s *Server) handleBadge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawSlug := strings.TrimPrefix(r.URL.Path, "/badge/")
	rawSlug = strings.Trim(rawSlug, "/")
	rawSlug = strings.TrimSuffix(rawSlug, ".svg")
	serviceName, err := url.PathUnescape(rawSlug)
	if err != nil || serviceName == "" {
		s.writeBadgeSVG(w, http.StatusNotFound, badgeSpec{
			leftText:  "deploystackr",
			rightText: "not found",
			dotColor:  "#94a3b8",
			theme:     badgeTheme(r),
		})
		return
	}

	svcCfg, exists := s.cfg.Services[serviceName]
	if !exists {
		s.writeBadgeSVG(w, http.StatusNotFound, badgeSpec{
			leftText:  serviceName,
			rightText: "not found",
			dotColor:  "#94a3b8",
			theme:     badgeTheme(r),
		})
		return
	}

	envParam := strings.TrimSpace(r.URL.Query().Get("env"))
	targetEnv := resolveBadgeEnv(svcCfg, s.cfg, envParam)
	if envParam != "" && targetEnv == "" {
		s.writeBadgeSVG(w, http.StatusNotFound, badgeSpec{
			leftText:  fmt.Sprintf("%s · %s", serviceDisplayName(serviceName, svcCfg, r), envParam),
			rightText: "env not found",
			dotColor:  "#94a3b8",
			theme:     badgeTheme(r),
			href:      servicePageURL(r, serviceName),
		})
		return
	}

	label := serviceDisplayName(serviceName, svcCfg, r)
	leftText := label
	if targetEnv != "" {
		leftText = fmt.Sprintf("%s · %s", label, targetEnv)
	}

	activeVer := "unknown"
	if targetEnv != "" {
		if activeEvt := s.store.GetActiveEvent(serviceName, targetEnv); activeEvt != nil && activeEvt.Version != "" {
			activeVer = activeEvt.Version
		}
	}

	rightText := shortBadgeVersion(activeVer)
	dotColor := "#6366f1"
	if activeVer == "unknown" {
		rightText = "n/a"
		dotColor = "#94a3b8"
	}

	if targetEnv != "" {
		pendingPR := s.store.GetPendingPR(serviceName, targetEnv)
		if pendingPR != nil && activeVer != "unknown" && areVersionsEqual(activeVer, pendingPR.TargetVersion) {
			pendingPR = nil
		}

		cachedVersions := s.store.GetVersions(serviceName)
		versionsBehind := 0
		isUpToDate := false

		if activeVer != "unknown" && len(cachedVersions) > 0 {
			latestVer := cachedVersions[0]
			if areVersionsEqual(activeVer, latestVer) {
				isUpToDate = true
			} else {
				foundIdx := -1
				for idx, v := range cachedVersions {
					if areVersionsEqual(v, activeVer) {
						foundIdx = idx
						break
					}
				}
				if foundIdx >= 0 {
					versionsBehind = foundIdx
				} else {
					versionsBehind = len(cachedVersions)
				}
			}
		}

		if pendingPR != nil {
			dotColor = "#3b82f6"
			if pendingPR.PRNumber > 0 {
				rightText = fmt.Sprintf("%s · PR #%d", rightText, pendingPR.PRNumber)
			} else {
				rightText = fmt.Sprintf("%s · PR pending", rightText)
			}
		} else if isUpToDate {
			dotColor = "#10b981"
		} else if versionsBehind > 0 {
			dotColor = "#f59e0b"
			rightText = fmt.Sprintf("%s · %d behind", rightText, versionsBehind)
		}
	}

	s.writeBadgeSVG(w, http.StatusOK, badgeSpec{
		leftText:  leftText,
		rightText: rightText,
		dotColor:  dotColor,
		theme:     badgeTheme(r),
		href:      servicePageURL(r, serviceName),
	})
}

type badgeSpec struct {
	leftText  string
	rightText string
	dotColor  string
	theme     string
	href      string
}

func badgeTheme(r *http.Request) string {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("theme"))) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	default:
		return "auto"
	}
}

func serviceDisplayName(serviceName string, svcCfg *config.ServiceConfig, r *http.Request) string {
	if custom := strings.TrimSpace(r.URL.Query().Get("title")); custom != "" {
		return custom
	}
	if custom := strings.TrimSpace(r.URL.Query().Get("label")); custom != "" {
		return custom
	}
	if svcCfg != nil && svcCfg.DisplayName != "" {
		return svcCfg.DisplayName
	}
	return serviceName
}

func resolveBadgeEnv(svcCfg *config.ServiceConfig, cfg *config.Config, envParam string) string {
	if svcCfg == nil || len(svcCfg.Environments) == 0 {
		return ""
	}
	if envParam != "" {
		for name := range svcCfg.Environments {
			if strings.EqualFold(name, envParam) {
				return name
			}
		}
		return ""
	}

	envPriority := svcCfg.EnvironmentPriority
	if len(envPriority) == 0 && cfg != nil {
		envPriority = cfg.EnvironmentPriority
	}
	for _, p := range envPriority {
		for name := range svcCfg.Environments {
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(p)) {
				return name
			}
		}
	}

	names := make([]string, 0, len(svcCfg.Environments))
	for name := range svcCfg.Environments {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "prod") || strings.Contains(lower, "live") || lower == "main" || lower == "release" {
			return name
		}
	}
	return names[0]
}

func shortBadgeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "unknown" {
		return "n/a"
	}
	if idx := strings.LastIndex(v, ":"); idx != -1 {
		v = v[idx+1:]
	}
	if idx := strings.Index(v, "@"); idx != -1 {
		v = v[:idx]
	}
	return v
}

func servicePageURL(r *http.Request, serviceName string) string {
	slug := "/" + url.PathEscape(serviceName)
	if r.Host == "" {
		return slug
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.Split(proto, ",")[0]
	} else if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", strings.TrimSpace(scheme), r.Host, slug)
}

func estimateTextWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == 'i' || r == 'l' || r == 'I' || r == '!' || r == '.' || r == ',' || r == ':':
			width += 4
		case r == 'f' || r == 'j' || r == 'r' || r == 't' || r == ' ' || r == '-':
			width += 5
		case r == 'm' || r == 'w' || r == 'M' || r == 'W':
			width += 10
		case r >= 'A' && r <= 'Z':
			width += 8
		default:
			width += 7
		}
	}
	return width + 4
}

func estimateMonoWidth(s string) int {
	// Monospace 11.5px weight 600 glyphs are ~7.2px each regardless of character
	return (utf8.RuneCountInString(s)*15 + 1) / 2
}

func (s *Server) writeBadgeSVG(w http.ResponseWriter, statusCode int, spec badgeSpec) {
	const height = 28
	const dotOffset = 14
	const leftPadAfterDot = 10
	const leftTailPad = 14
	const rightPad = 14

	leftTextWidth := estimateTextWidth(spec.leftText)
	rightTextWidth := estimateMonoWidth(spec.rightText)

	leftWidth := dotOffset + leftPadAfterDot + leftTextWidth + leftTailPad
	rightWidth := rightPad + rightTextWidth + rightPad
	totalWidth := leftWidth + rightWidth

	leftTextX := dotOffset + leftPadAfterDot
	rightTextX := leftWidth + rightPad

	leftEsc := html.EscapeString(spec.leftText)
	rightEsc := html.EscapeString(spec.rightText)
	titleEsc := html.EscapeString(spec.leftText + " " + spec.rightText)

	themeCSS := badgeThemeCSS(spec.theme)

	openLink := ""
	closeLink := ""
	if spec.href != "" {
		hrefEsc := html.EscapeString(spec.href)
		openLink = fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">`, hrefEsc)
		closeLink = `</a>`
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">
  <title>%s</title>
  <style>
%s
  </style>
  %s
  <g>
    <rect class="bg" x="0.5" y="0.5" width="%d" height="%d" rx="6"/>
    <path class="right-bg" d="M%d 1h%da5 5 0 0 1 5 5v%da5 5 0 0 1-5 5H%dV1z"/>
    <line class="divider" x1="%d" y1="1" x2="%d" y2="%d"/>
    <circle cx="%d" cy="14" r="4" fill="%s"/>
    <text class="label" x="%d" y="18">%s</text>
    <text class="value" x="%d" y="18">%s</text>
  </g>
  %s
</svg>`,
		totalWidth, height, totalWidth, height, titleEsc,
		titleEsc,
		themeCSS,
		openLink,
		totalWidth-1, height-1,
		leftWidth, rightWidth-6, height-12, leftWidth,
		leftWidth, leftWidth, height-1,
		dotOffset, spec.dotColor,
		leftTextX, leftEsc,
		rightTextX, rightEsc,
		closeLink,
	)

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(svg))
}

func badgeThemeCSS(theme string) string {
	lightRules := `    .bg { fill: #ffffff; stroke: #cbd5e1; stroke-width: 1; }
    .right-bg { fill: #f8fafc; }
    .divider { stroke: #e2e8f0; stroke-width: 1; }
    .label { fill: #0f172a; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11.5px; font-weight: 600; }
    .value { fill: #334155; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11.5px; font-weight: 600; }
    a { text-decoration: none; cursor: pointer; }`

	darkRules := `    .bg { fill: #0f172a; stroke: #334155; stroke-width: 1; }
    .right-bg { fill: #1e293b; }
    .divider { stroke: #334155; stroke-width: 1; }
    .label { fill: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11.5px; font-weight: 600; }
    .value { fill: #e2e8f0; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11.5px; font-weight: 600; }
    a { text-decoration: none; cursor: pointer; }`

	if theme == "dark" {
		return darkRules
	}
	if theme == "light" {
		return lightRules
	}
	return lightRules + "\n    @media (prefers-color-scheme: dark) {\n" +
		"      .bg { fill: #0f172a; stroke: #334155; }\n" +
		"      .right-bg { fill: #1e293b; }\n" +
		"      .divider { stroke: #334155; }\n" +
		"      .label { fill: #f8fafc; }\n" +
		"      .value { fill: #e2e8f0; }\n" +
		"    }"
}
