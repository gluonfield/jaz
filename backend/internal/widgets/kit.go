package widgets

import (
	_ "embed"
	"fmt"
	"html"
	"strings"
)

// The host document around a widget fragment: design-system CSS, Tailwind,
// the message bridge, and the CSP fence.

//go:embed assets/widget-base.css
var BaseCSS string

//go:embed assets/bridge.js
var BridgeJS string

//go:embed assets/tailwind.js
var TailwindJS string

// contentSecurityPolicy is the fence around widget code: no app or Electron
// access (the iframe sandbox handles that), network limited to https fetches
// and https/data images per the any-https decision. 'self' admits only the
// host's own assets (Tailwind) — widget-supplied external scripts stay blocked.
const contentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline' 'self'; img-src https: data:; connect-src https:; font-src data:; media-src https: data:; form-action 'none'; base-uri 'none'"

// TailwindAssetPath is where the host serves the vendored Tailwind v4 browser
// build; same-origin so the CSP 'self' covers it and widgets work offline.
const TailwindAssetPath = "/v1/widgets/assets/tailwind.js"

// tailwindTheme retargets Tailwind at the Jaz design system: preflight loads
// below the jz kit so bare elements behave the way Tailwind-fluent authors
// assume, stock colors are wiped (no slate/red-500 escapes — the palette IS
// the tokens), the radius and text scales are remapped so habitual classes
// land on-brand, and the token vars are exposed as utilities (bg-surface-2,
// text-ink-3, rounded-card…). Values are var() references, so light/dark
// flips with the html.dark class.
const tailwindTheme = `
@import "tailwindcss/theme.css" layer(theme);
@import "tailwindcss/preflight.css" layer(base);
@import "tailwindcss/utilities.css" layer(utilities);
@custom-variant dark (&:where(.dark, .dark *));
@theme {
  --color-*: initial;
  --radius-*: initial;
  --radius-xs: 6px;
  --radius-sm: 8px;
  --radius-md: 10px;
  --radius-lg: 12px;
  --radius-xl: 16px;
  --radius-2xl: 20px;
  --radius-control: 10px;
  --radius-card: 12px;
  --text-xs: 11px;
  --text-xs--line-height: 1.45;
  --text-sm: 12px;
  --text-sm--line-height: 1.5;
  --text-base: 13px;
  --text-base--line-height: 1.5;
  --text-lg: 15px;
  --text-lg--line-height: 1.45;
  --text-xl: 18px;
  --text-xl--line-height: 1.4;
  --text-2xl: 22px;
  --text-2xl--line-height: 1.25;
}
@theme inline {
  --color-white: #fff;
  --color-black: #000;
  --color-bg: var(--color-bg);
  --color-surface: var(--color-surface);
  --color-surface-2: var(--color-surface-2);
  --color-ink: var(--color-ink);
  --color-ink-2: var(--color-ink-2);
  --color-ink-3: var(--color-ink-3);
  --color-primary: var(--color-primary);
  --color-primary-strong: var(--color-primary-strong);
  --color-primary-soft: var(--color-primary-soft);
  --color-on-primary: var(--color-on-primary);
  --color-accent: var(--color-accent);
  --color-accent-soft: var(--color-accent-soft);
  --color-running: var(--color-running);
  --color-ok: var(--color-ok);
  --color-danger: var(--color-danger);
  --color-danger-soft: var(--color-danger-soft);
  --color-border: var(--color-border);
}
`

// RenderDocument wraps a published widget fragment in the host document that
// carries the design-system CSS, Tailwind, the bridge script, and the CSP.
// zoom scales the whole document (the board's font-size control); 0 means 1.
func RenderDocument(title, fragment, theme string, zoom float64) string {
	return RenderDocumentWithOptions(title, fragment, theme, zoom, RenderOptions{})
}

type RenderOptions struct {
	InlineAssets bool
}

func RenderDocumentWithOptions(title, fragment, theme string, zoom float64, opts RenderOptions) string {
	themeClass := ""
	if theme == "dark" {
		themeClass = ` class="dark"`
	}
	// Zoom lives on the #jz-root wrapper. Standardized CSS zoom resolves the
	// wrapper's 100% height in zoomed coordinates, so no height compensation
	// is needed (or wanted — it would leave a dead strip).
	zoomAttr := ""
	if zoom > 0 && zoom != 1 {
		zoomAttr = fmt.Sprintf(` style="zoom: %.2f"`, clampScale(zoom))
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html")
	b.WriteString(themeClass)
	b.WriteString(">\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<meta http-equiv=\"Content-Security-Policy\" content=\"%s\">\n", contentSecurityPolicy)
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(title))
	b.WriteString("<style>\n")
	b.WriteString(BaseCSS)
	b.WriteString("</style>\n")
	if opts.InlineAssets {
		b.WriteString("<script>\n")
		b.WriteString(TailwindJS)
		b.WriteString("\n</script>\n")
	} else {
		fmt.Fprintf(&b, "<script src=\"%s\"></script>\n", TailwindAssetPath)
	}
	b.WriteString("<style type=\"text/tailwindcss\">")
	b.WriteString(tailwindTheme)
	b.WriteString("</style>\n</head>\n<body>\n<div id=\"jz-root\"")
	b.WriteString(zoomAttr)
	b.WriteString(">\n")
	b.WriteString(fragment)
	b.WriteString("\n</div>\n<script>\n")
	b.WriteString(BridgeJS)
	b.WriteString("</script>\n</body>\n</html>\n")
	return b.String()
}
