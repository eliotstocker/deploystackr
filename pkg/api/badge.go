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

	primaryState := s.buildEnvBadgeState(serviceName, targetEnv)

	var hoverRows []envBadgeRow
	if envParam == "" {
		orderedEnvs := sortBadgeEnvs(svcCfg, s.cfg)
		if len(orderedEnvs) > 1 {
			hoverRows = make([]envBadgeRow, 0, len(orderedEnvs)-1)
			for _, envName := range orderedEnvs[1:] {
				st := s.buildEnvBadgeState(serviceName, envName)
				hoverRows = append(hoverRows, envBadgeRow{
					envName:   envName,
					rightText: st.rightText,
					dotColor:  st.dotColor,
					outdated:  st.outdated,
				})
			}
		}
	}

	s.writeBadgeSVG(w, http.StatusOK, badgeSpec{
		leftText:  leftText,
		rightText: primaryState.rightText,
		dotColor:  primaryState.dotColor,
		outdated:  primaryState.outdated,
		hoverRows: hoverRows,
		theme:     badgeTheme(r),
		href:      servicePageURL(r, serviceName),
	})
}

type envBadgeRow struct {
	envName   string
	rightText string
	dotColor  string
	outdated  bool
}

type badgeSpec struct {
	leftText  string
	rightText string
	dotColor  string
	outdated  bool
	hoverRows []envBadgeRow
	theme     string
	href      string
}

func (s *Server) buildEnvBadgeState(serviceName, envName string) envBadgeRow {
	activeVer := "unknown"
	if envName != "" {
		if activeEvt := s.store.GetActiveEvent(serviceName, envName); activeEvt != nil && activeEvt.Version != "" {
			activeVer = activeEvt.Version
		}
	}

	rightText := shortBadgeVersion(activeVer)
	dotColor := "#6366f1"
	if activeVer == "unknown" {
		rightText = "n/a"
		dotColor = "#94a3b8"
	}

	isOutdated := false
	if envName != "" {
		pendingPR := s.store.GetPendingPR(serviceName, envName)
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
			isOutdated = true
			rightText = fmt.Sprintf("%s · %d behind", rightText, versionsBehind)
		}
	}

	return envBadgeRow{
		envName:   envName,
		rightText: rightText,
		dotColor:  dotColor,
		outdated:  isOutdated,
	}
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

func sortBadgeEnvs(svcCfg *config.ServiceConfig, cfg *config.Config) []string {
	if svcCfg == nil || len(svcCfg.Environments) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(svcCfg.Environments))
	ordered := make([]string, 0, len(svcCfg.Environments))

	envPriority := svcCfg.EnvironmentPriority
	if len(envPriority) == 0 && cfg != nil {
		envPriority = cfg.EnvironmentPriority
	}
	for _, p := range envPriority {
		for name := range svcCfg.Environments {
			if !seen[name] && strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(p)) {
				ordered = append(ordered, name)
				seen[name] = true
			}
		}
	}

	var remaining []string
	for name := range svcCfg.Environments {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)

	// Put prod-like environments ahead of other unprioritized envs
	var prodLike, others []string
	for _, name := range remaining {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "prod") || strings.Contains(lower, "live") || lower == "main" || lower == "release" {
			prodLike = append(prodLike, name)
		} else {
			others = append(others, name)
		}
	}
	ordered = append(ordered, prodLike...)
	ordered = append(ordered, others...)
	return ordered
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
	ordered := sortBadgeEnvs(svcCfg, cfg)
	if len(ordered) == 0 {
		return ""
	}
	return ordered[0]
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

	rightBgClass := "right-bg"
	valueClass := "value"
	if spec.outdated {
		rightBgClass = "right-bg outdated"
		valueClass = "value outdated"
	}

	svgWidth := totalWidth
	svgHeight := height

	drawerSVG := ""
	if len(spec.hoverRows) > 0 {
		const rowHeight = 24
		const drawerGap = 4
		const drawerPadY = 4

		maxEnvCol := 0
		maxValCol := 0
		for _, row := range spec.hoverRows {
			if w := estimateTextWidth(row.envName); w > maxEnvCol {
				maxEnvCol = w
			}
			if w := estimateMonoWidth(row.rightText); w > maxValCol {
				maxValCol = w
			}
		}
		drawerWidth := dotOffset + leftPadAfterDot + maxEnvCol + 20 + maxValCol + rightPad
		if drawerWidth > svgWidth {
			svgWidth = drawerWidth
		}
		if totalWidth > drawerWidth {
			drawerWidth = totalWidth
		}

		drawerY := height + drawerGap
		drawerHeight := drawerPadY*2 + len(spec.hoverRows)*rowHeight
		svgHeight = drawerY + drawerHeight + 2

		var rowsBuilder strings.Builder
		valX := drawerWidth - rightPad - maxValCol
		for i, row := range spec.hoverRows {
			ry := drawerY + drawerPadY + i*rowHeight
			if i > 0 {
				fmt.Fprintf(&rowsBuilder, `      <line class="drawer-sep" x1="8" y1="%d" x2="%d" y2="%d"/>`+"\n", ry, drawerWidth-8, ry)
			}
			rowValClass := "value"
			if row.outdated {
				rowValClass = "value outdated"
				fmt.Fprintf(&rowsBuilder, `      <rect class="right-bg outdated" x="%d" y="%d" width="%d" height="%d" rx="4"/>`+"\n", valX-8, ry+3, maxValCol+16, rowHeight-6)
			}
			fmt.Fprintf(&rowsBuilder, `      <circle cx="%d" cy="%d" r="3.5" fill="%s"/>`+"\n", dotOffset, ry+12, row.dotColor)
			fmt.Fprintf(&rowsBuilder, `      <text class="label" x="%d" y="%d">%s</text>`+"\n", leftTextX, ry+16, html.EscapeString(row.envName))
			fmt.Fprintf(&rowsBuilder, `      <text class="%s" x="%d" y="%d">%s</text>`+"\n", rowValClass, valX, ry+16, html.EscapeString(row.rightText))
		}

		drawerSVG = fmt.Sprintf(`
    <g class="env-drawer">
      <rect class="bg" x="0.5" y="%d.5" width="%d" height="%d" rx="6"/>
%s    </g>`, drawerY, drawerWidth-1, drawerHeight-1, rowsBuilder.String())
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">
  <title>%s</title>
  <style>
%s
  </style>
  %s
  <g class="badge-root">
    <rect class="bg" x="0.5" y="0.5" width="%d" height="%d" rx="6"/>
    <path class="%s" d="M%d 1h%da5 5 0 0 1 5 5v%da5 5 0 0 1-5 5H%dV1z"/>
    <line class="divider" x1="%d" y1="1" x2="%d" y2="%d"/>
    <circle cx="%d" cy="14" r="4" fill="%s"/>
    <text class="label" x="%d" y="18">%s</text>
    <text class="%s" x="%d" y="18">%s</text>%s
  </g>
  %s
</svg>`,
		svgWidth, svgHeight, svgWidth, svgHeight, titleEsc,
		titleEsc,
		themeCSS,
		openLink,
		totalWidth-1, height-1,
		rightBgClass, leftWidth, rightWidth-6, height-12, leftWidth,
		leftWidth, leftWidth, height-1,
		dotOffset, spec.dotColor,
		leftTextX, leftEsc,
		valueClass, rightTextX, rightEsc,
		drawerSVG,
		closeLink,
	)

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(svg))
}

func badgeThemeCSS(theme string) string {
	hoverRules := `    .env-drawer { opacity: 0; visibility: hidden; pointer-events: none; transform: translateY(-3px); transition: opacity 0.15s ease, transform 0.15s ease; }
    .badge-root:hover .env-drawer, .badge-root:focus-within .env-drawer { opacity: 1; visibility: visible; pointer-events: auto; transform: translateY(0); }
    a { text-decoration: none; cursor: pointer; }`

	lightRules := `    .bg { fill: #ffffff; stroke: #cbd5e1; stroke-width: 1; }
    .right-bg { fill: #f8fafc; }
    .right-bg.outdated { fill: #fef3c7; }
    .divider, .drawer-sep { stroke: #e2e8f0; stroke-width: 1; }
    .label { fill: #0f172a; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11.5px; font-weight: 600; }
    .value { fill: #334155; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11.5px; font-weight: 600; }
    .value.outdated { fill: #92400e; }
` + hoverRules

	darkRules := `    .bg { fill: #0f172a; stroke: #334155; stroke-width: 1; }
    .right-bg { fill: #1e293b; }
    .right-bg.outdated { fill: #422006; }
    .divider, .drawer-sep { stroke: #334155; stroke-width: 1; }
    .label { fill: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11.5px; font-weight: 600; }
    .value { fill: #e2e8f0; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11.5px; font-weight: 600; }
    .value.outdated { fill: #fcd34d; }
` + hoverRules

	if theme == "dark" {
		return darkRules
	}
	if theme == "light" {
		return lightRules
	}
	return lightRules + "\n    @media (prefers-color-scheme: dark) {\n" +
		"      .bg { fill: #0f172a; stroke: #334155; }\n" +
		"      .right-bg { fill: #1e293b; }\n" +
		"      .right-bg.outdated { fill: #422006; }\n" +
		"      .divider, .drawer-sep { stroke: #334155; }\n" +
		"      .label { fill: #f8fafc; }\n" +
		"      .value { fill: #e2e8f0; }\n" +
		"      .value.outdated { fill: #fcd34d; }\n" +
		"    }"
}
