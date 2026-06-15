# `visualize:read_me` — Complete Reference

All modules loaded simultaneously: `diagram`, `mockup`, `interactive`, `data_viz`, `art`, `chart`, `elicitation`.
This document captures the full verbatim content returned by the tool.

---

## Tool identity

- **Tool name:** `visualize:read_me` (companion to `visualize:show_widget`)
- **Not a skill file** — it is a built-in Anthropic tool, not sourced from `/mnt/skills/`
- **Companion tool:** `visualize:show_widget` renders SVG or HTML inline in the conversation
- **Purpose:** Loads design system documentation and per-module guidance before widget creation

---

## Modules available

| Module | Description |
|--------|-------------|
| `diagram` | SVG flowcharts, structural diagrams, illustrative diagrams |
| `mockup` | UI mockups, forms, cards, dashboards |
| `interactive` | Interactive explainers with controls |
| `chart` | Charts, data analysis, geographic maps (Chart.js, D3 choropleth) |
| `art` | Illustration and generative art |
| `elicitation` | Collecting skill arguments via forms |

---

## Universal header (returned on every call)

The tool always returns this preamble regardless of which modules are requested:

> You create rich visual content — SVG diagrams/illustrations and HTML interactive widgets — that renders inline in conversation. The best output feels like a natural extension of the chat.

### Complexity budget — hard limits (always active)

- Box subtitles: ≤5 words. Detail goes in `sendPrompt()` or prose below — not the box.
- Colors: ≤2 ramps per diagram. If colors encode meaning, add a 1-line legend. Otherwise use one neutral ramp.
- Horizontal tier: ≤4 boxes at full width (~140px each). 5+ boxes → shrink to ≤110px OR wrap to 2 rows OR split into overview + detail diagrams.

### Accessibility (always active)

- HTML widgets: begin with `<h2 class="sr-only">` containing a one-sentence summary for screen readers.
- SVG widgets: use `role="img"` with `<title>` and `<desc>` as first children.

---

## Core Design System (always active across all modules)

### Philosophy

- **Seamless** — users shouldn't notice where claude.ai ends and the widget begins.
- **Flat** — no gradients, mesh backgrounds, noise textures, or decorative effects. Clean flat surfaces.
- **Compact** — show the essential inline. Explain the rest in text.
- **Text goes in response, visuals go in the tool** — all explanatory text, descriptions, introductions, and summaries must be written as normal response text OUTSIDE the tool call. The tool output should contain ONLY the visual element.

### Streaming rules

- **HTML:** `<style>` (short) → content HTML → `<script>` last.
- **SVG:** `<defs>` (markers) → visual elements immediately.
- Prefer inline `style="..."` over `<style>` blocks — inputs/controls must look correct mid-stream.
- Keep `<style>` under ~15 lines (interactive widgets may need more — that's fine, but don't bloat with decorative CSS).
- Gradients, shadows, and blur flash during streaming DOM diffs. Use solid flat fills instead.

### Hard rules

- No `<!-- comments -->` or `/* comments */` — waste tokens, break streaming
- No font-size below 11px
- No emoji. Icons = Tabler **outline** webfont (5800+ icons, already loaded): `<i class="ti ti-home"></i>`. Outline only — never use `-filled` suffixes. Decorative icons get `aria-hidden="true"`; icon-only buttons get `aria-label`.
- Common Tabler icons: `ti-home ti-settings ti-user ti-search ti-x ti-check ti-plus ti-trash ti-edit ti-download ti-upload ti-file ti-folder ti-chart-bar ti-calendar ti-clock ti-arrow-right ti-arrow-left ti-chevron-down ti-external-link ti-copy ti-refresh ti-player-play ti-player-pause ti-heart ti-star ti-bell ti-mail ti-lock ti-eye ti-menu-2`
- No gradients, drop shadows, blur, glow, or neon effects
- No dark/colored backgrounds on outer containers (transparent only — host provides the bg)
- **Typography:** Default font is Anthropic Sans. For the rare editorial/blockquote moment, use `font-family: var(--font-serif)`.
- **Headings:** h1 = 22px, h2 = 18px, h3 = 16px — all `font-weight: 500`. Heading color is pre-set to `var(--color-text-primary)` — don't override it. Body text = 16px, weight 400, `line-height: 1.7`. **Two weights only: 400 regular, 500 bold.** Never use 600 or 700.
- **Sentence case** always. Never Title Case, never ALL CAPS.
- **No mid-sentence bolding.** Entity/class/function names go in `code style` not **bold**.
- The widget container is `display: block; width: 100%`. Start content directly — no wrapper div needed. For breathing room add `padding: 1rem 0` on the first element.
- Never use `position: fixed` — the iframe viewport sizes itself to in-flow content height. For modal/overlay mockups: wrap in a normal-flow `<div style="min-height: 400px; background: rgba(0,0,0,0.45); display: flex; align-items: center; justify-content: center;">`.
- No DOCTYPE, `<html>`, `<head>`, or `<body>` — just content fragments.
- Text on colored background: use the darkest shade from the same color family — never plain black or generic gray.
- **Corners:** `border-radius: var(--border-radius-md)` (or `-lg` for cards) in HTML. In SVG, `rx="4"` default — larger values = pills, use deliberately.
- **No rounded corners on single-sided borders** — `border-left` or `border-top` accents get `border-radius: 0`.
- No titles or prose inside the tool output.
- **Icon sizing:** Tabler `<i class="ti …">` sizes with `font-size` — 16–20px inline, 24px max decorative.
- No tabs, carousels, or `display: none` sections during streaming. Show all content stacked vertically. (Post-streaming JS-driven steppers are fine.)
- No nested scrolling — auto-fit height.
- Scripts execute after streaming — load libraries via `<script src="https://cdnjs.cloudflare.com/ajax/libs/...">` (UMD globals), then use the global in a plain `<script>` that follows.
- **CDN allowlist (CSP-enforced):** `cdnjs.cloudflare.com`, `esm.sh`, `cdn.jsdelivr.net`, `unpkg.com`, `fonts.googleapis.com`, `fonts.gstatic.com`. All other origins are blocked.

### CSS Variables

**Backgrounds:**
- `--color-background-primary` (white)
- `--color-background-secondary` (surfaces)
- `--color-background-tertiary` (page bg)
- `--color-background-info`, `-danger`, `-success`, `-warning`

**Text:**
- `--color-text-primary` (black)
- `--color-text-secondary` (muted)
- `--color-text-tertiary` (hints)
- `--color-text-info`, `-danger`, `-success`, `-warning`

**Borders:**
- `--color-border-tertiary` (0.15α, default)
- `--color-border-secondary` (0.3α, hover)
- `--color-border-primary` (0.4α)
- Semantic: `-info`, `-danger`, `-success`, `-warning`

**Typography:**
- `--font-sans`, `--font-serif`, `--font-mono`

**Layout:**
- `--border-radius-md` (8px)
- `--border-radius-lg` (12px — preferred for most components)
- `--border-radius-xl` (16px)

All auto-adapt to light/dark mode.

### Dark mode (mandatory)

Every color must work in both light and dark modes:

- **In SVG:** use pre-built color classes (`c-blue`, `c-teal`, etc.) — they handle dark mode automatically. Never write `<style>` blocks for colors.
- **In SVG:** every `<text>` element needs a class (`t`, `ts`, `th`) — never omit fill or use `fill="inherit"`.
- **In HTML:** always use CSS variables for text. Never hardcode like `color: #333` — invisible in dark mode.
- Mental test: if the background were near-black, would every text element still be readable?

### `sendPrompt(text)`

A global function that sends a message to chat as if the user typed it. Use when the user's next step benefits from Claude thinking. Handle filtering, sorting, toggling, and calculations in JS instead.

### Links

`<a href="https://...">` just works — clicks are intercepted and open the host's link-confirmation dialog. Or call `openLink(url)` directly.

### Fallback rule

When nothing fits:
- Default to **editorial layout** if the content is explanatory
- Default to **card layout** if the content is a bounded object
- All core design system rules still apply
- Use `sendPrompt()` for any action that benefits from Claude thinking

---

## Color Palette (always active)

9 color ramps, each with 7 stops from lightest to darkest.

| Class | Ramp | 50 | 100 | 200 | 400 | 600 | 800 | 900 |
|-------|------|----|-----|-----|-----|-----|-----|-----|
| `c-purple` | Purple | #EEEDFE | #CECBF6 | #AFA9EC | #7F77DD | #534AB7 | #3C3489 | #26215C |
| `c-teal` | Teal | #E1F5EE | #9FE1CB | #5DCAA5 | #1D9E75 | #0F6E56 | #085041 | #04342C |
| `c-coral` | Coral | #FAECE7 | #F5C4B3 | #F0997B | #D85A30 | #993C1D | #712B13 | #4A1B0C |
| `c-pink` | Pink | #FBEAF0 | #F4C0D1 | #ED93B1 | #D4537E | #993556 | #72243E | #4B1528 |
| `c-gray` | Gray | #F1EFE8 | #D3D1C7 | #B4B2A9 | #888780 | #5F5E5A | #444441 | #2C2C2A |
| `c-blue` | Blue | #E6F1FB | #B5D4F4 | #85B7EB | #378ADD | #185FA5 | #0C447C | #042C53 |
| `c-green` | Green | #EAF3DE | #C0DD97 | #97C459 | #639922 | #3B6D11 | #27500A | #173404 |
| `c-amber` | Amber | #FAEEDA | #FAC775 | #EF9F27 | #BA7517 | #854F0B | #633806 | #412402 |
| `c-red` | Red | #FCEBEB | #F7C1C1 | #F09595 | #E24B4A | #A32D2D | #791F1F | #501313 |

### Color assignment rules

- Color encodes **meaning**, not sequence. Don't cycle like a rainbow.
- Group nodes by **category** — all nodes of the same type share one color.
- For illustrative diagrams, map to **physical properties** — warm ramps for heat/energy, cool for cold/calm, green for organic, gray for structural/inert.
- Use **gray for neutral/structural** nodes (start, end, generic steps).
- Use **2–3 colors per diagram**, not 6+.
- **Prefer purple, teal, coral, pink** for general categories. Reserve blue, green, amber, red for informational/success/warning/error semantics.

### Text on colored backgrounds

Always use the 800 or 900 stop from the same ramp. When a box has title + subtitle, use two different stops — title darker (800), subtitle lighter (600).

### Light/dark mode quick pick

- **Light mode:** 50 fill + 600 stroke + 800 title / 600 subtitle
- **Dark mode:** 800 fill + 200 stroke + 100 title / 200 subtitle
- Apply `c-{ramp}` to `<g>` or directly to `<rect>`/`<circle>`/`<ellipse>`. Never to `<path>`.
- Dark mode is automatic for ramp classes.

---

## SVG Setup (diagram and art modules)

### ViewBox safety checklist

1. Find lowest element: `max(y + height)` across all rects, `max(y)` across text baselines.
2. Set viewBox height = that value + 40px buffer.
3. Find rightmost element: all content must stay within x=0 to x=680.
4. For `text-anchor="end"`, text extends LEFT from x — check it doesn't go negative.
5. Never use negative x or y coordinates. ViewBox starts at 0,0.
6. No unintentional overlaps — check every pair of elements that aren't meant to layer.
7. Flowcharts: for every pair of boxes in the same row, left box's `(x + width)` must be less than right box's `x` by at least 20px.

### SVG root element

```svg
<svg width="100%" viewBox="0 0 680 H" role="img">
  <title>…</title>
  <desc>…</desc>
  …
</svg>
```

- **680px wide** — load-bearing, do not change. With `width="100%"`, this renders 1:1 with CSS pixels.
- H = last element's bottom edge + 40px padding. Don't leave excess space.
- Safe area: x=40 to x=640, y=40 to y=(H-40).
- Background transparent. Do not wrap in a container with a background color.
- One SVG per tool call.

### Pre-built SVG classes

| Class | Description |
|-------|-------------|
| `t` | sans 14px primary text |
| `ts` | sans 12px secondary text |
| `th` | sans 14px medium (500) text |
| `box` | neutral rect (bg-secondary fill, border stroke) |
| `node` | clickable group with hover effect (cursor pointer, slight dim) |
| `arr` | arrow line (1.5px, open chevron head) |
| `leader` | dashed leader line (tertiary stroke, 0.5px, dashed) |
| `c-{ramp}` | colored node — sets fill+stroke on shapes, auto-adjusts child text, dark mode automatic |

Short aliases: `var(--p)`, `var(--s)`, `var(--t)`, `var(--bg2)`, `var(--b)`

### Arrow marker (include in every SVG)

```svg
<defs>
  <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
    <path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
  </marker>
</defs>
```

Use `marker-end="url(#arrow)"` on lines. The head inherits the line's color via `context-stroke`.

### Font size calibration table

| Text | Chars | Weight | Size | Rendered width |
|------|-------|--------|------|----------------|
| Authentication Service | 22 | 500 | 14px | 167px |
| Background Job Processor | 24 | 500 | 14px | 201px |
| Detects and validates incoming tokens | 37 | 400 | 14px | 279px |
| forwards request to | 19 | 400 | 12px | 123px |
| データベースサーバー接続 | 12 | 400 | 14px | 181px |

### SVG rules summary

- Two font sizes only: 14px for node labels (`t` or `th`), 12px for subtitles/descriptions/arrow labels (`ts`)
- No icons or illustrations inside boxes — text only (exception: illustrative diagrams)
- `<text>` never auto-wraps — every line break needs explicit `<tspan x="..." dy="1.2em">`
- Every `<text>` must carry a class — `t`, `ts`, or `th`
- `<path>` connectors need `fill="none"` — SVG defaults to black fill
- Stroke width: 0.5px for diagram borders and edges
- Connector paths that would cross unrelated boxes must use L-shaped `<path>` detour
- No rotated text
- `<defs>` may contain: arrow marker, `<clipPath>`, subtle `<pattern>` fills, one `<linearGradient>` (illustrative only)
- `c-{ramp}` nesting: uses direct-child selectors — put `c-*` on the innermost group holding shapes
- `dominant-baseline="central"` on all text inside boxes, with y set to the center of the slot

---

## Module: Diagram

### Decision tree — which diagram type to use

Route on the **verb**, not the noun:

| User says | Type | What to draw |
|---|---|---|
| "how do LLMs work" | Illustrative | Token row, stacked layer slabs, attention threads |
| "transformer architecture" | Structural | Labelled boxes: embedding, attention heads, FFN, layer norm |
| "how does attention work" | Illustrative | One query token, fan of lines to every key, opacity = weight |
| "how does gradient descent work" | Illustrative | Contour surface, a ball, trail of steps |
| "what are the training steps" | Flowchart | Forward → loss → backward → update |
| "how does TCP work" | Illustrative | Two endpoints, numbered packets in flight, ACK returning |
| "TCP handshake sequence" | Flowchart | SYN → SYN-ACK → ACK |
| "explain Krebs cycle" / "how does the event loop work" | HTML stepper | Click through stages. Never a ring. |
| "how does a hash map work" | Illustrative | Key falling through a funnel into N buckets |
| "draw the database schema" / "ERD" | mermaid.js | `erDiagram` syntax |

Two families:
1. **Reference diagrams** (flowchart, structural) — the user wants a map to point at. Precision matters.
2. **Intuition diagrams** (illustrative) — the user wants to *feel* how something works.

Don't mix families in one diagram. Multiple SVG calls are fine — break complex explanations into a series. Always add prose between diagrams — never stack SVG calls back-to-back.

### Flowchart

For sequential processes, cause-and-effect, decision trees.

**Planning rules:**
- At 14px sans-serif, each character ≈ 8px wide. "Load Balancer" (13 chars) → needs ≥140px rect.
- Special characters (chemical formulas, math notation, subscripts, Unicode) are wider — add 30–50% extra width.
- Spacing: 60px minimum between boxes, 24px padding inside boxes, 12px between text and edges. 10px gap between arrowheads and box edges.
- Two-line boxes: ≥56px height, 22px between lines.
- Vertical text: `dominant-baseline="central"`, y set to the center of the slot. Formula: `x={x+w/2} y={y+h/2} text-anchor="middle"`.
- Single-direction flows preferred (all top-down or all left-right). Max 4–5 nodes per diagram.

**Tier packing:**
- Compute total width BEFORE placing.
- WRONG: x=40,160,260,360 w=160 → overlaps (4×160=640 > 480 available)
- RIGHT: x=50,200,350,500 w=130 gap=20 → fits (4×130 + 3×20 = 580 ≤ 590 safe width)

**Cycles:**
- Never draw cyclic processes as rings — use HTML stepper instead.
- For feedback loops in linear flows: use a small `↻` glyph + text near the cycle point, or restructure as a circle if the cycle IS the point.

**Arrows:**
- A line from A to B must not cross any other box or label.
- If direct path crosses something, use L-bend: `<path d="M x1 y1 L x1 ymid L x2 ymid L x2 y2"/>`

**When prompt is over budget:**
- 6+ components → decompose: (1) stripped overview, (2) one diagram per interesting sub-flow

**Flowchart component patterns:**

Single-line node (44px tall):
```svg
<g class="node c-blue" onclick="sendPrompt('Tell me more about T-cells')">
  <rect x="100" y="20" width="180" height="44" rx="8" stroke-width="0.5"/>
  <text class="th" x="190" y="42" text-anchor="middle" dominant-baseline="central">T-cells</text>
</g>
```

Two-line node (56px tall):
```svg
<g class="node c-blue" onclick="sendPrompt('Tell me more about dendritic cells')">
  <rect x="100" y="20" width="200" height="56" rx="8" stroke-width="0.5"/>
  <text class="th" x="200" y="38" text-anchor="middle" dominant-baseline="central">Dendritic cells</text>
  <text class="ts" x="200" y="56" text-anchor="middle" dominant-baseline="central">Detect foreign antigens</text>
</g>
```

Connector (no label):
```svg
<line x1="200" y1="76" x2="200" y2="120" class="arr" marker-end="url(#arrow)"/>
```

Neutral node: use `class="box"` for auto-themed fill/stroke.

Make all nodes clickable by default with `onclick="sendPrompt('...')"` — hover effect is built in.

### Structural diagram

For concepts where physical or logical **containment** matters — things inside other things.

**Container rules:**
- Outermost: large rounded rect, rx=20-24, lightest fill (50 stop), 0.5px stroke (600 stop). Label at top-left inside, 14px bold.
- Inner regions: medium rounded rects, rx=8-12, next shade fill (100-200 stop). Different ramp if semantically different.
- 20px minimum padding inside every container.
- Max 2–3 nesting levels.

**Layout:**
- Inner regions side by side with 16px+ gap.
- External inputs/outputs sit outside with arrows pointing in/out.
- Short labels for externals — one word or a short phrase.

**Structural container example:**
```svg
<defs>
  <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
    <path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
  </marker>
</defs>
<g class="c-green">
  <rect x="120" y="30" width="560" height="260" rx="20" stroke-width="0.5"/>
  <text class="th" x="400" y="62" text-anchor="middle">Library branch</text>
  <text class="ts" x="400" y="80" text-anchor="middle">Main floor</text>
</g>
<g class="c-teal">
  <rect x="150" y="100" width="220" height="160" rx="12" stroke-width="0.5"/>
  <text class="th" x="260" y="130" text-anchor="middle">Circulation desk</text>
  <text class="ts" x="260" y="148" text-anchor="middle">Checkouts, returns</text>
</g>
<g class="c-amber">
  <rect x="450" y="100" width="210" height="160" rx="12" stroke-width="0.5"/>
  <text class="th" x="555" y="130" text-anchor="middle">Reading room</text>
  <text class="ts" x="555" y="148" text-anchor="middle">Seating, reference</text>
</g>
<text class="ts" x="410" y="175" text-anchor="middle">Books</text>
<line x1="370" y1="185" x2="448" y2="185" class="arr" marker-end="url(#arrow)"/>
<text class="ts" x="40" y="185" text-anchor="middle">New acq.</text>
<line x1="75" y1="185" x2="118" y2="185" class="arr" marker-end="url(#arrow)"/>
```

**Color in structural diagrams:** Nested regions need distinct ramps — same class on parent and child flattens the hierarchy. Pick a related ramp for inner structures, contrasting ramp for functionally different regions.

**ERDs — use mermaid.js, not SVG:**
```
erDiagram
  USERS ||--o{ POSTS : writes
  POSTS ||--o{ COMMENTS : has
  USERS {
    uuid id PK
    string email
    timestamp created_at
  }
```

Import and initialize with:
```html
<script type="module">
import mermaid from 'https://esm.sh/mermaid@11/dist/mermaid.esm.min.mjs';
const dark = matchMedia('(prefers-color-scheme: dark)').matches;
await document.fonts.ready;
mermaid.initialize({
  startOnLoad: false,
  theme: 'base',
  fontFamily: '"Anthropic Sans", sans-serif',
  themeVariables: {
    darkMode: dark,
    fontSize: '13px',
    fontFamily: '"Anthropic Sans", sans-serif',
    lineColor: dark ? '#9c9a92' : '#73726c',
    textColor: dark ? '#c2c0b6' : '#3d3d3a',
  },
});
</script>
```

Same init works for `classDiagram`.

### Illustrative diagram

For building **intuition**. Two flavours:
- **Physical subjects** — draw simplified versions: cross-sections, cutaways, schematics.
- **Abstract subjects** — draw spatial metaphors: transformer as stacked slabs with attention threads, gradient descent as ball on loss surface, hash table as funnel into buckets.

**Key differences from flowchart/structural:**
- Shapes are freeform — `<path>`, `<ellipse>`, `<circle>`, `<polygon>`
- Layout follows the subject's geometry, not a grid
- Color encodes **intensity**, not category — warm ramps = active/high-weight, cool/gray = dormant/low-weight
- Layering and overlap are encouraged for shapes — but never let a stroke cross text
- Small shape-based indicators allowed: triangles for flames, circles for bubbles, wavy lines for steam
- One `<linearGradient>` permitted (the only exception to no-gradients rule) — for continuous physical properties only, two stops from one ramp
- Animation permitted in HTML versions — CSS `@keyframes` on `transform` and `opacity` only, under 2s, wrapped in `@media (prefers-reduced-motion: no-preference)`

**Fidelity ceiling:** Schematics, not illustrations. A tank is a rounded rect. A flame is three triangles. Recognisable silhouette beats accurate contour.

**Label placement:**
- Labels outside the drawn object with thin leader lines (0.5px dashed `var(--t)` stroke) pointing to relevant parts
- Pick **one side** for labels — reserve ≥140px horizontal margin on the label side
- Default to right-side labels with `text-anchor="start"` unless geometry forces otherwise
- `ts` (12px) for callouts, `th` (14px medium) for major component names

**Prefer interactive over static:** if the real-world system has a control, give the diagram that control.

**Composition approach:**
1. Main object's silhouette — largest shape, centered in viewBox
2. Internal structure: chambers, pipes, membranes, mechanical parts
3. External connections: pipes, arrows, labels for inputs/outputs
4. State indicators last: color fills, small animated elements

**Physical-color scenes (sky, water, grass, skin, materials):** Use ALL hardcoded hex — never mix with `c-*` theme classes.

---

## Module: Mockup

### Use cases

UI mockups, forms, cards, dashboards.

### Component patterns

**Layout width:** 680px wide. Use `repeat(auto-fit, minmax(160px, 1fr))` for responsive columns.

**Aesthetic:** Flat, clean, white surfaces. Minimal 0.5px borders. Generous whitespace. No gradients, no shadows (except functional focus rings).

**Tokens:**
- Borders: always `0.5px solid var(--color-border-tertiary)` (or `-secondary` for emphasis)
- Corner radius: `var(--border-radius-md)` for most elements, `var(--border-radius-lg)` for cards
- Cards: white bg (`var(--color-background-primary)`), 0.5px border, radius-lg, padding 1rem 1.25rem
- Form elements (input, select, textarea, button, range slider) are pre-styled — write bare tags.
- Buttons: pre-styled with transparent bg, 0.5px border-secondary, hover bg-secondary, active scale(0.98). If it triggers `sendPrompt`, append ↗ arrow.
- Round every displayed number — always pass through `Math.round()`, `.toFixed(n)`, or `Intl.NumberFormat`.
- Spacing: rem for vertical rhythm (1rem, 1.5rem, 2rem), px for component-internal gaps (8px, 12px, 16px)
- Box-shadows: none, except `box-shadow: 0 0 0 Npx` focus rings on inputs

**Metric cards:**
- For summary numbers — `background: var(--color-background-secondary)`, no border, `border-radius: var(--border-radius-md)`, padding 1rem
- 13px muted label above, 24px/500 number below
- Grids of 2–4 with `gap: 12px`

**Grid overflow:** Use `minmax(0, 1fr)` to clamp children.

**Table overflow:** In constrained layouts (≤700px), use `table-layout: fixed` and explicit column widths, or allow horizontal scroll on a wrapper.

**Mockup presentation:**
- Contained mockups (mobile screens, chat threads, single cards, modals, small UI components): sit on `var(--color-background-secondary)` with `border-radius: var(--border-radius-lg)` and padding, or a device frame.
- Full-width mockups (dashboards, settings pages, data tables): no extra wrapper.

### Named use-case patterns

**1. Interactive explainer:**
```html
<div style="display: flex; align-items: center; gap: 12px; margin: 0 0 1.5rem;">
  <label style="font-size: 14px; color: var(--color-text-secondary);">Years</label>
  <input type="range" min="1" max="40" value="20" id="years" style="flex: 1;" />
  <span style="font-size: 14px; font-weight: 500; min-width: 24px;" id="years-out">20</span>
</div>
```

**2. Compare options:**
- Each option in a card
- Use badges for key differentiators
- Leading Tabler icon at 20px anchors each option visually
- Featured item accent: `border: 2px solid var(--color-border-info)` (only exception to 0.5px rule)
- Featured badge: `background: var(--color-background-info); color: var(--color-text-info); font-size: 12px; padding: 4px 12px; border-radius: var(--border-radius-md)`
- Don't put comparison tables inside the tool — output as markdown in response text

**3. Data record:**
```html
<div style="background: var(--color-background-primary); border-radius: var(--border-radius-lg); border: 0.5px solid var(--color-border-tertiary); padding: 1rem 1.25rem;">
  <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 16px;">
    <div style="width: 44px; height: 44px; border-radius: 50%; background: var(--color-background-info); display: flex; align-items: center; justify-content: center; font-weight: 500; font-size: 14px; color: var(--color-text-info);">MR</div>
    <div>
      <p style="font-weight: 500; font-size: 15px; margin: 0;">Maya Rodriguez</p>
      <p style="font-size: 13px; color: var(--color-text-secondary); margin: 0;">VP of Engineering</p>
    </div>
  </div>
</div>
```

---

## Module: Interactive

The interactive module provides the same content as the Mockup module's UI components section — controls, sliders, buttons, live state displays. Key additions:

- HTML steppers for cyclic/sequential content that shouldn't be a ring diagram
- Post-streaming JS is fine for steppers — show all content stacked vertically during streaming, then JS can reorganize after
- Use `sendPrompt()` for user follow-up actions

---

## Module: Chart

### Chart.js

```html
<div style="position: relative; width: 100%; height: 300px;">
  <canvas id="myChart" role="img" aria-label="Bar chart of quarterly revenue, Q1 through Q4">Quarterly revenue: Q1 12, Q2 19, Q3 8, Q4 15.</canvas>
</div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.js"></script>
<script>
  new Chart(document.getElementById('myChart'), {
    type: 'bar',
    data: { labels: ['Q1','Q2','Q3','Q4'], datasets: [{ label: 'Revenue', data: [12,19,8,15] }] },
    options: { responsive: true, maintainAspectRatio: false }
  });
</script>
```

**Chart.js rules:**
- Every `<canvas>` MUST have `role="img"` and descriptive `aria-label` + fallback text between tags
- Never rely on color alone to distinguish series — pair with dash pattern, marker shape, or fill pattern
- Canvas cannot resolve CSS variables — use hardcoded hex or Chart.js defaults
- Wrap `<canvas>` in `<div>` with explicit `height` and `position: relative`
- Canvas sizing: set height ONLY on wrapper div, never on canvas element itself. Use `responsive: true, maintainAspectRatio: false`
- Horizontal bar chart wrapper: height ≥ `(number_of_bars * 40) + 80` pixels
- Load UMD build via cdnjs — sets `window.Chart` global. Follow with plain `<script>` (no `type="module"`)
- Multiple charts: unique IDs (`myChart1`, `myChart2`), each gets own canvas+div pair
- Bubble/scatter: pad scale range ~10% beyond data range to prevent clipping at boundaries
- ≤12 categories needing all labels: `scales.x.ticks: { autoSkip: false, maxRotation: 45 }`
- Negative values: `-$5M` not `$-5M` — sign before currency symbol

**Custom legends (always disable Chart.js default):**
```js
plugins: { legend: { display: false } }
```
```html
<div style="display: flex; flex-wrap: wrap; gap: 16px; margin-bottom: 8px; font-size: 12px; color: var(--color-text-secondary);">
  <span style="display: flex; align-items: center; gap: 4px;">
    <span style="width: 10px; height: 10px; border-radius: 2px; background: #3266ad;"></span>Chrome 65%
  </span>
</div>
```

Include value/percentage in each label for categorical data. Position above (`margin-bottom`) or below (`margin-top`) — not inside canvas.

**Dashboard layout:** Metric cards above, chart canvas below without card wrapper. Use `sendPrompt()` for drill-down.

### Geographic maps (D3 choropleth)

Never invent coordinates. Three topology sources on jsdelivr:

| Coverage | URL | Projection | Object key |
|----------|-----|-----------|------------|
| US states | `https://cdn.jsdelivr.net/npm/us-atlas@3/states-10m.json` | `d3.geoAlbersUsa()` | `.states` |
| World countries | `https://cdn.jsdelivr.net/npm/world-atlas@2/countries-110m.json` | `d3.geoNaturalEarth1()` | `.countries` |
| Per-country subdivisions | `https://cdn.jsdelivr.net/npm/datamaps@0.5.10/src/js/data/{iso3}.topo.json` (lowercase alpha-3: `deu`, `jpn`, `gbr`) | varies | `.{iso3}` |

Fetch only from the CDN allowlist — `raw.githubusercontent.com` and other hosts are blocked.

**Before writing the widget:** `web_fetch` the topology URL to see real feature `id` and `properties.name` values.

```html
<div id="map" style="width: 100%;"></div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3/7.8.5/d3.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/topojson/3.0.2/topojson.min.js"></script>
<script>
const values = { 'California': 39, 'Texas': 30, 'New York': 19 };
const isDark = matchMedia('(prefers-color-scheme: dark)').matches;
const color = d3.scaleQuantize([0, 40], isDark ? d3.schemeBlues[5].slice().reverse() : d3.schemeBlues[5]);
const svg = d3.select('#map').append('svg').attr('viewBox', '0 0 900 560').attr('width', '100%');
const path = d3.geoPath(d3.geoAlbersUsa().scale(1100).translate([450, 280]));
d3.json('https://cdn.jsdelivr.net/npm/us-atlas@3/states-10m.json').then(us => {
  svg.selectAll('path').data(topojson.feature(us, us.objects.states).features).join('path')
    .attr('d', path).attr('stroke', isDark ? 'rgba(255,255,255,.15)' : '#fff')
    .attr('fill', d => color(values[d.properties.name] ?? 0));
});
</script>
```

---

## Module: Art

For "draw me a sunset" / "create a geometric pattern" etc.

Use SVG. Same technical rules (viewBox, safe area) but different aesthetic:
- Fill the canvas — art should feel rich, not sparse
- Bold colors: mix CSS variable semantic categories for variety
- Art is the one place custom `<style>` color blocks are fine — freestyle colors, `prefers-color-scheme` for dark mode variants if desired
- Layer overlapping opaque shapes for depth
- Organic forms with `<path>` curves, `<ellipse>`, `<circle>`
- Texture via repetition (parallel lines, dots, hatching) not raster effects
- Geometric patterns with `<g transform="rotate()">` for radial symmetry

---

## Module: Elicitation

For collecting skill arguments via interactive forms.

### Core principle: infer first

Check the conversation and attachments before rendering. Only ask for what you genuinely cannot determine. A one-question form beats five where four are already answerable. If everything can be inferred: skip the form entirely.

### Question phrasing

Phrase as questions from you, not field labels:

| Don't write | Write |
|---|---|
| Side: | Which side are you on? |
| Deadline: | When does this need to be finalized? |
| Concerns: | Any specific concerns I should focus on? |

### Structure

The shell auto-wires option toggles, "Other" reveal, file upload, and submit — write HTML with classes and `data-*` attributes. **Zero onclick handlers, zero `<script>`.**

**Fixed chrome — emit byte-for-byte:**
- The form wrapper, header, body, footer, `.elicit-group` rhythm, and `.elicit-question` label are pre-styled
- Header title always: `"[subject] details"` e.g. "Contract details"
- The header SVG (file icon) is fixed — do not substitute or redraw

```html
<form class="elicit">
  <div class="elicit-header">
    <svg viewBox="0 0 20 20" fill="currentColor"><path d="M11.586 2a1.5 1.5 0 0 1 1.06.44l2.914 2.914a1.5 1.5 0 0 1 .44 1.06V16.5a1.5 1.5 0 0 1-1.5 1.5h-9a1.5 1.5 0 0 1-1.492-1.347L4 16.5v-13A1.5 1.5 0 0 1 5.5 2zM5.5 3a.5.5 0 0 0-.5.5v13a.5.5 0 0 0 .5.5h9a.5.5 0 0 0 .5-.5V7h-2.5A1.5 1.5 0 0 1 11 5.5V3zm7.04 10.304a.5.5 0 0 1 .92.392c-.295.69-.871 1.304-1.66 1.304-.487 0-.892-.234-1.2-.574-.309.34-.713.574-1.2.574-.486 0-.892-.233-1.2-.574-.31.34-.714.574-1.2.574a.5.5 0 0 1 0-1c.212 0 .52-.18.74-.696l.034-.067a.5.5 0 0 1 .886.067c.221.516.528.696.74.696.213 0 .52-.18.74-.696l.035-.067a.5.5 0 0 1 .885.067c.22.516.527.696.74.696s.519-.18.74-.696m0-4a.5.5 0 0 1 .92.392c-.295.69-.871 1.304-1.66 1.304-.487 0-.892-.234-1.2-.574-.309.34-.713.574-1.2.574-.486 0-.892-.233-1.2-.574-.31.34-.714.574-1.2.574a.5.5 0 0 1 0-1c.212 0 .52-.18.74-.696l.034-.067a.5.5 0 0 1 .886.067c.221.516.528.696.74.696.213 0 .52-.18.74-.696l.035-.067a.5.5 0 0 1 .885.067c.22.516.527.696.74.696s.519-.18.74-.696M12 5.5a.5.5 0 0 0 .5.5h2.293L12 3.207z"/></svg>
    <span>Contract details</span>
  </div>
  <div class="elicit-body">
    <!-- .elicit-group blocks go here -->
  </div>
  <div class="elicit-footer">
    <button type="button" class="elicit-skip">Skip</button>
    <button type="button" class="elicit-submit">Continue</button>
  </div>
</form>
```

### Color story

Default to **blue** for selection states. Exceptions:
1. **Strong semantic reason:** amber = budget/cost, red = risk/destructive, green = success/confirmation — use `data-accent="warning|danger|success"` on `.elicit-pill`
2. **The element is inherently visual** — color inside the card's icon/SVG/preview only; selection chrome stays blue

Never set background or border via inline `style` on a pill — overrides selection-state CSS.

### Choice input formats

| Content | Format |
|---|---|
| Short labels, ≤4 words | Plain pills |
| Options with icons/subtitles | Cards |
| Output/layout pickers | Preview tiles |
| Dates | `<input type="date">` |
| Quantities/scales | `<input type="range">` |

Do not render every question as pills — vary the visual format across the form.

Every `.elicit-pill` must carry `data-value="<clean option value>"` — the shell reads this when collecting answers.

**Plain pills:**
```html
<div class="elicit-group">
  <label class="elicit-question">Which side are you on?</label>
  <div class="elicit-pills" data-name="side" data-multi="false">
    <button type="button" class="elicit-pill" data-value="Vendor">Vendor</button>
    <button type="button" class="elicit-pill" data-value="Customer">Customer</button>
    <button type="button" class="elicit-pill" data-value="Other" data-other>Other</button>
  </div>
  <input type="text" class="elicit-other" data-for="side" placeholder="Tell me more" hidden>
</div>
```

**Cards** (options with icons and subtitles):
```html
<div class="elicit-pills" data-name="processor" data-multi="false">
  <button type="button" class="elicit-pill" data-value="stripe"
    style="border-radius:12px; padding:14px 16px; display:flex; gap:12px; align-items:flex-start; text-align:left; min-width:180px; box-shadow:0 1px 2px rgba(0,0,0,0.04)">
    <i class="ti ti-credit-card" style="font-size:20px" aria-hidden="true"></i>
    <span>
      <span style="font-size:13px; font-weight:500">Stripe</span><br>
      <span style="font-size:11px; color:var(--color-text-tertiary)">Payments &amp; invoicing</span>
    </span>
  </button>
</div>
```

**Preview tiles** (output-format pickers):
```html
<div class="elicit-pills" data-name="output" data-multi="false">
  <button type="button" class="elicit-pill" data-value="waterfall"
    style="width:110px; border-radius:12px; padding:14px 10px; display:flex; flex-direction:column; align-items:center; gap:8px; box-shadow:0 1px 2px rgba(0,0,0,0.04)">
    <svg width="48" height="36" viewBox="0 0 48 36" fill="none" stroke="currentColor" stroke-width="1.5">
      <rect x="4" y="22" width="6" height="10"/><rect x="14" y="14" width="6" height="8"/>
      <rect x="24" y="8" width="6" height="6"/><rect x="34" y="4" width="6" height="28"/>
    </svg>
    <span style="font-size:13px; font-weight:500">Waterfall bridge</span>
  </button>
</div>
```

**Sliders and dates:**
```html
<input type="range" data-name="quality" min="1" max="5" step="1">
<input type="date" class="elicit-date" data-name="deadline">
```

When an answer might not be listed, add escape-hatch with `data-other` — selecting it reveals the paired `.elicit-other` input.

### File upload

Include a dropzone when the skill needs data/documents. Skip if the user already attached the relevant file.

The dropzone SVG icon is fixed chrome — emit byte-for-byte:
```html
<div class="elicit-group">
  <label class="elicit-question">Upload the contract (or paste the relevant text below):</label>
  <div class="elicit-files" data-name="contract">
    <label class="elicit-dropzone">
      <svg viewBox="0 0 20 20" fill="currentColor"><path d="M16.5 13a.5.5 0 0 1 .5.5v2a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 3 15.5v-2a.5.5 0 0 1 1 0v2a.5.5 0 0 0 .5.5h11a.5.5 0 0 0 .5-.5v-2a.5.5 0 0 1 .5-.5M10 3a.5.5 0 0 1 .374.168l4 4.5.059.082a.5.5 0 0 1-.732.65l-.075-.068L10.5 4.814V13.5a.5.5 0 0 1-1 0V4.814L6.374 8.332a.5.5 0 0 1-.748-.664l4-4.5.08-.071A.5.5 0 0 1 10 3"/></svg>
      <span>Choose file</span>
      <input type="file" multiple>
    </label>
  </div>
  <textarea class="elicit-textarea" data-name="contract_text"
    placeholder="or paste the contract text / key clauses here"></textarea>
</div>
```

Always pair dropzone with a textarea fallback in the same group.

### Free text and dates

```html
<textarea class="elicit-textarea" data-name="concerns" placeholder="Anything specific?"></textarea>
<input type="date" class="elicit-date" data-name="deadline">
```

### After submit — payload format

Answers arrive as a single line:
```
Contract details — Side: Customer · Diet: Vegan, Gluten-free · Deadline: 2027-01-05
```

- Labels = `data-name` attributes humanized to sentence case (`output_format` → `Output format`)
- `_text` suffix dropped, `_file` → ` file`, `_other` → ` (other)`
- Multi-select values: comma-joined
- Short textarea values: newlines flattened to ` / `
- Values 81–200 chars: wrapped in quotes
- Values over 200 chars: `Label: (N chars — see below)` + repeated verbatim under `--- Full content ---` fold
- If skipped: `(Skipped the form — proceed with defaults or ask me in plain text)`

### Polish

Elicitation forms are an explicit exception to the no-shadows rule: the form wrapper, pills, cards, and tiles all carry a light drop shadow. The wrapper's shadow is pre-applied; for cards and tiles add `box-shadow: 0 1px 2px rgba(0,0,0,0.04)` inline.

---

## Quick Decision Reference

| Task | Module | Output format |
|------|--------|---------------|
| "How does X work" (abstract) | `diagram` → illustrative | SVG or HTML with inline SVG |
| "How does X work" (physical) | `diagram` → illustrative | SVG or HTML with inline SVG |
| "Draw the architecture" | `diagram` → structural | SVG |
| "Walk me through the flow" | `diagram` → flowchart | SVG |
| "Draw the ERD / schema" | `diagram` → mermaid.js | HTML with mermaid import |
| "Explain the Krebs cycle" | `diagram` → HTML stepper | HTML (not ring SVG) |
| "Show me a UI for X" | `mockup` | HTML |
| "Compare these options" | `mockup` / `interactive` | HTML card grid |
| "Make an interactive explainer" | `interactive` | HTML with controls |
| "Chart this data" | `chart` → Chart.js | HTML with canvas |
| "Show this on a map" | `chart` → D3 choropleth | HTML with D3 |
| "Draw me a [creative image]" | `art` | SVG |
| "I need info before proceeding" | `elicitation` | HTML form with `.elicit-*` classes |

---

## CDN Allowlist (CSP-enforced)

Only these origins may be loaded from:
- `cdnjs.cloudflare.com`
- `esm.sh`
- `cdn.jsdelivr.net`
- `unpkg.com`
- `fonts.googleapis.com`
- `fonts.gstatic.com`

All other origins are blocked — requests silently fail.
