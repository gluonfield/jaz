package widgets

import (
	"regexp"
	"strings"
)

// The lint pass catches the recurring widget failure modes at publish time so
// the agent can fix them in the same run. Warnings never block a publish —
// they ride back on the tool/ext-method result. Widgets share the inline-
// artifact design system, so these checks cover only the few tile-specific
// failure modes that need validation at publish time.

var (
	fullDocRe      = regexp.MustCompile(`(?i)^\s*(?:<!doctype|<html[\s>]|<head[\s>]|<body[\s>])`)
	localStyleRe   = regexp.MustCompile(`(?i)(<style[\s>]|style\s*=|<svg[\s>])`)
	viewportUnitRe = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?(?:vh|vw|vmin|vmax)\b`)
	fixedPosRe     = regexp.MustCompile(`(?i)position:\s*fixed`)
	cardClassRe    = regexp.MustCompile(`(?i)class\s*=\s*["'][^"']*\bcard\b`)
	paragraphRe    = regexp.MustCompile(`(?i)<p(?:\s|>)`)
)

// LintHTML reports non-fatal quality problems in a widget fragment.
func LintHTML(html string) []string {
	var warnings []string
	if fullDocRe.MatchString(html) {
		warnings = append(warnings, "the widget must be an HTML fragment, not a full document — remove <!doctype>/<html>/<head>/<body>; the host wraps the fragment in the artifact document")
	}
	if m := viewportUnitRe.FindString(html); m != "" {
		warnings = append(warnings, "viewport units ("+strings.TrimSpace(m)+") size against the window, not the tile — the tile is a fixed cell, so 100vh always overflows; use percentages or a flex column that fills the tile")
	}
	if fixedPosRe.MatchString(html) {
		warnings = append(warnings, "position: fixed collapses the sandboxed frame and pins over scrolled content — use a normal-flow layout, or sticky inside the one internal scroller")
	}
	if !localStyleRe.MatchString(html) {
		warnings = append(warnings, "widget has no local styling — build a polished artifact tile with scoped CSS or inline styles instead of bare markup")
	}
	if len(paragraphRe.FindAllString(html, -1)) > 6 || len(cardClassRe.FindAllString(html, -1)) > 8 {
		warnings = append(warnings, "widget reads like a long feed rather than a board tile — summarize the top signals first, use compact rows/chips/metrics, and keep detailed history behind one short scroll region")
	}
	return warnings
}
