package widgets

import (
	"fmt"
	"regexp"
	"strings"
)

// The lint pass catches the recurring widget failure modes at publish time so
// the agent can fix them in the same run. Warnings never block a publish —
// they ride back on the tool/ext-method result.

var (
	viewportUnitRe = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?(?:vh|vw|vmin|vmax)\b|\b(?:min-h-screen|h-screen|w-screen)\b`)
	rawColorRe     = regexp.MustCompile(`(?i)(?:#[0-9a-f]{3,8}\b|\brgba?\(|\bhsla?\()`)
	// Stock Tailwind palette classes compile to nothing: the default colors
	// are disabled so widgets copy the Jaz palette.
	stockPaletteRe = regexp.MustCompile(`\b(?:bg|text|border|fill|stroke|ring|divide|from|to|via)-(?:red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|slate|gray|zinc|neutral|stone)-\d{2,3}\b`)
	styleBlockRe   = regexp.MustCompile(`(?is)<style[^>]*>(.*?)</style>`)
	importantRe    = regexp.MustCompile(`!important`)
	fixedPosRe     = regexp.MustCompile(`(?i)position:\s*fixed`)
)

// LintHTML reports non-fatal quality problems in a widget fragment.
func LintHTML(html string) []string {
	var warnings []string
	if m := viewportUnitRe.FindString(html); m != "" {
		warnings = append(warnings, fmt.Sprintf("viewport units (%s) size against the iframe, not the content box — the host pads the document, so 100vh always overflows and conjures a permanent scrollbar; use percentages or flex (jz-fill) instead", strings.TrimSpace(m)))
	}
	if m := rawColorRe.FindString(html); m != "" {
		warnings = append(warnings, fmt.Sprintf("hardcoded color (%s…) breaks light/dark theming — use the Jaz tokens (var(--color-*)) or the token Tailwind utilities (bg-surface-2, text-ink-2, …)", strings.TrimSpace(m)))
	}
	if m := stockPaletteRe.FindString(html); m != "" {
		warnings = append(warnings, fmt.Sprintf("stock Tailwind palette class (%s) compiles to NOTHING here — the default palette is disabled; use the Jaz token utilities (bg-primary, text-ok, bg-danger-soft, …)", m))
	}
	if fixedPosRe.MatchString(html) {
		warnings = append(warnings, "position: fixed pins to the tile viewport and overlaps scrolled content; use sticky inside the scroller, or flex pinning")
	}
	styleLen := 0
	for _, m := range styleBlockRe.FindAllStringSubmatch(html, -1) {
		styleLen += len(m[1])
	}
	if styleLen > 2500 {
		warnings = append(warnings, fmt.Sprintf("%d chars of custom CSS — that much bespoke styling usually fights the kit; prefer Tailwind utilities and jz-* components, keep <style> for the truly custom bits", styleLen))
	}
	if n := len(importantRe.FindAllString(html, -1)); n > 2 {
		warnings = append(warnings, fmt.Sprintf("%d uses of !important — the cascade is already arranged so utilities win; !important fights the host and future runs", n))
	}
	return warnings
}
