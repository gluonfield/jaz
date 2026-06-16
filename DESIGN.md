# Design

Visual system for jaz desktop (Electron renderer, `frontend/`). Calm product UI with a cobalt brand. Color strategy: **Restrained** — cool paper ground, cobalt-tinted neutrals, one cobalt primary, rainbow reserved for live states, clay as a rare warm counterpoint.

## Brand system

- **Cobalt at rest, rainbow in motion.** Cobalt (`hue 262`) is the everyday brand voice: primary actions, selection, links, caret, toggles. The rainbow ramp appears *only while jaz is alive* — the composer focus comet, the welcome PixelField confetti, the voice visualizer, and `.jaz-shimmer` dots on live thinking/tool indicators. No wordmark in app chrome — the welcome PixelField is the only wordmark moment.
- **Neutrals belong to the brand.** Every neutral is tinted toward hue 262 at chroma 0.004–0.012 — never generic warm or pure gray.
- **Capsule controls.** One-line interactive elements are pills (`rounded-full`): buttons, icon buttons, sidebar rows, menu rows, chips, segmented options, toasts. Boxes that hold multi-line content (inputs, textareas, cards, popovers, modals) use `--radius-control` 10px / `--radius-card` 12px. The settings ThemeSwitcher is the reference pattern.

## Theme

Light and dark, class-based (`:root.dark`), user-selectable (system/light/dark in Settings). Light: cool paper ground. Dark: cobalt-tinted charcoal; surfaces grow lighter as they come forward; primary lifts to light cobalt with dark on-primary text.

## Color (OKLCH) — light / dark

| Role | Light | Dark | Use |
|---|---|---|---|
| `--color-bg` | `oklch(0.963 0.007 262)` | `oklch(0.208 0.007 262)` | App ground |
| `--color-surface` | `oklch(0.94 0.009 262)` | `oklch(0.248 0.008 262)` | Sidebar, cards, composer |
| `--color-surface-2` | `oklch(0.906 0.011 262)` | `oklch(0.298 0.009 262)` | Hover, raised, code blocks |
| `--color-ink` | `oklch(0.27 0.01 262)` | `oklch(0.945 0.004 262)` | Primary text |
| `--color-ink-2` | `oklch(0.5 0.01 262)` | `oklch(0.76 0.006 262)` | Secondary text |
| `--color-ink-3` | `oklch(0.57 0.01 262)` | `oklch(0.62 0.007 262)` | Timestamps, hints |
| `--color-primary` | `oklch(0.5 0.13 262)` | `oklch(0.79 0.11 262)` | Brand cobalt — primary buttons, links, active states, caret, switch/checkbox on |
| `--color-primary-strong` | `oklch(0.43 0.13 262)` | `oklch(0.85 0.11 262)` | Hover on primary fills, badge text on `-soft` |
| `--color-primary-soft` | `oklch(0.92 0.035 262)` | `oklch(0.35 0.05 262)` | Active rows, selection, acp badge fill |
| `--color-on-primary` | `oklch(1 0 0)` | `oklch(0.21 0.03 262)` | Text/icons on primary fills |
| `--color-accent` | clay `hue 55` | clay, lifted | Warm counterpoint: needs-auth, unsaved dot, mono syntax |
| `--color-running` | amber `hue 75` | amber, lifted | Session running status dots (sidebar) |
| `--color-ok` / `--color-danger` | green 145 / red 25 | lifted | Status |
| `--color-border` | `oklch(0.853 0.01 262)` | `oklch(0.325 0.009 262)` | Hairlines |
| `--color-rainbow-1…5` | red→violet ramp | lifted | Live states only (see brand system) |

Text on saturated fills: `--color-on-primary`. Dark text only on `-soft` tints.

## Typography

- **UI + prose**: Inter Variable (self-hosted via @fontsource-variable). One family; hierarchy via weight (400/500/600) and a tight 1.125–1.2 scale.
- **Technical**: JetBrains Mono Variable for session ids, tool names/args, code in markdown, the editor.
- Scale (rem): 0.75 (meta), 0.8125 (sidebar/labels), 0.875 (body/UI), 1.0 (section), 1.125 (page title), 1.375 (display, rare).
- Prose line length ≤72ch; `text-wrap: balance` on headings.

## Components

- **Sidebar** (`--color-surface`, 1px border-right, resizable): empty 52px titlebar drag strip (traffic lights only); New Thread, Sessions (last 7), Loops, Settings footer. Rows are pills; active item: `--color-primary-soft` bg + ink text.
- **Badges/pills**: soft tints, balanced padding (`px-1.5 py-[3px]`, 11px text) — runtime (`native` neutral surface-2, `acp` primary-soft with primary-strong text), status dot (running amber / failed red).
- **Live indicators**: `.jaz-shimmer` dots (thinking "live", tool "running"). Reduced motion: static cobalt.
- **Buttons**: pills — primary (cobalt fill, on-primary text, hover primary-strong), secondary/ghost (text + surface-2 hover), danger (danger-soft hover). Focus-visible ring `2px --color-primary` offset 2.
- **Form fields**: `bg-bg` with `ring-1 ring-border`, cobalt ring on focus; 10px radius (12px+ for cards). Compact inline fields (search, model query) are pills with an `ink/10` wash.
- **Switch/Checkbox**: on = `bg-primary` with `on-primary` knob/tick; off = faint ink track / bordered box.
- **Composer**: borderless card on `--color-surface`; rainbow conic comet orbits while focused; cobalt caret.
- **Transcript**: user messages right-tinted card (`--color-surface`), assistant prose plain on bg (72ch), tool calls as inline mono chevron rows expanding to bordered panels.
- **Empty states**: teach the screen; welcome page is the PixelField (brand cobalt particles + rainbow confetti wordmark).
- Loading: skeleton rows/blocks, not centered spinners.

## Layout & Motion

- Shell: resizable sidebar (default 264px) + fluid content column. macOS hiddenInset titlebar: 52px draggable strip above sidebar.
- Radius: pills for one-line controls; `--radius-control` 10px for inputs, `--radius-card` 12px for cards/popovers/modals.
- Motion: 150–250ms ease-out; state only (route fade, toast slide-in, live rows fade-in, shimmer on live dots). Full `prefers-reduced-motion: reduce` fallbacks.
- Z-scale: `--z-dropdown: 10`, `--z-modal: 60`, `--z-file-drop: 70`, `--z-command: 80`, `--z-toast: 90`, `--z-tooltip: 100`.
