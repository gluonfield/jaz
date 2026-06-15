

---

## Board tile mode

This widget is a **board tile**, not an inline chat artifact. The whole guide above applies — Jaz design tokens, `c-*` ramps, dark mode via `color-scheme`, Chart.js theming, the CDN allowlist — with three tile-specific rules:

- **Fill the tile, don't flow to content.** The tile is a fixed-size cell the user sets and resizes; there is no "natural height" to grow into. Make the root element `height: 100%` and lay it out (flex column) so it fills the tile with no dead space at the bottom.
- **One internal scroller.** If the content can exceed the tile, put `overflow: auto` on exactly one region and let it scroll inside the tile. Never let the tile itself overflow, and never crop content with `overflow: hidden`.
- **Read-only, no chrome.** The tile frame already shows the title and freshness. Render only the data the loop tracks — no headings that repeat the title, no captions, no forms, no action buttons, no `sendPrompt()`.
