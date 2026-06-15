package widgets

import (
	"regexp"
	"strings"
)

// The lint pass catches the recurring widget failure modes at publish time so
// the agent can fix them in the same run. Warnings never block a publish —
// they ride back on the tool/ext-method result. Widgets share the inline-
// artifact design system, so these checks cover only the few tile-specific
// failure modes that design guide doesn't enforce on its own.

var (
	fullDocRe      = regexp.MustCompile(`(?i)^\s*(?:<!doctype|<html[\s>]|<head[\s>]|<body[\s>])`)
	viewportUnitRe = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?(?:vh|vw|vmin|vmax)\b`)
	fixedPosRe     = regexp.MustCompile(`(?i)position:\s*fixed`)
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
	return warnings
}
