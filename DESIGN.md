# Design

Visual system for jaz desktop (Electron renderer, `frontend/`). Light, warm-companion product UI. Color strategy: **Restrained** — pure white ground, tinted sage neutrals, one sage primary, clay accent ≤10% of any view.

## Theme

Light only (v1). The scene: the owner checks on their assistant in daylight, beside a dark terminal — the desktop app is the daylight counterpart to the TUI. No dark mode yet.

## Color (OKLCH)

| Role | Value | Use |
|---|---|---|
| `--color-bg` | `oklch(1 0 0)` | App ground — pure white, no hidden warmth |
| `--color-surface` | `oklch(0.972 0.004 110)` | Sidebar, panels — faint sage tint |
| `--color-surface-2` | `oklch(0.945 0.006 110)` | Hover, raised, code blocks |
| `--color-ink` | `oklch(0.27 0.015 110)` | Primary text (≈13:1 on bg) |
| `--color-ink-2` | `oklch(0.50 0.014 110)` | Secondary text (≈5:1) |
| `--color-ink-3` | `oklch(0.62 0.012 110)` | Timestamps, hints — large/secondary only (≈3.5:1) |
| `--color-primary` | `oklch(0.55 0.12 110)` | Brand sage — primary buttons (white text), active nav, links, selection |
| `--color-primary-soft` | `oklch(0.93 0.04 110)` | Selected-row tint, primary-soft badges (ink text) |
| `--color-accent` | `oklch(0.62 0.13 55)` | Warm clay — acp/agent badges, save feedback (white text on fills) |
| `--color-accent-soft` | `oklch(0.94 0.035 55)` | Soft clay tint for badges (dark clay text) |
| `--color-running` | `oklch(0.65 0.13 75)` | Session running/busy (amber) |
| `--color-ok` | `oklch(0.60 0.13 145)` | Idle/success (green) |
| `--color-danger` | `oklch(0.55 0.19 25)` | Failed/error (red) |
| `--color-border` | `oklch(0.90 0.006 110)` | Hairlines |

Text on saturated fills (primary/accent buttons, status pills): white. Dark text only on `-soft` tints.

## Typography

- **UI + prose**: Inter Variable (self-hosted via @fontsource-variable). One family; hierarchy via weight (400/500/600) and a tight 1.125–1.2 scale.
- **Technical**: JetBrains Mono Variable for session ids, tool names/args, code in markdown, the editor.
- Scale (rem): 0.75 (meta), 0.8125 (sidebar/labels), 0.875 (body/UI), 1.0 (section), 1.125 (page title), 1.375 (display, rare).
- Prose line length ≤72ch; `text-wrap: balance` on headings.

## Components

- **Sidebar** (`--color-surface`, 1px border-right): ChatGPT-shaped left rail. Sessions section (last 7, "Show more"), then Pages section (Agent, future: Settings, New session). Active item: `--color-primary-soft` bg + ink text, 8px radius.
- **Badges/pills**: 4px-radius soft tints — runtime (`native` neutral, `acp` clay-soft), state dot (running amber / idle green / failed red) with 8px dot + label.
- **Buttons**: primary (sage fill, white text), secondary (white, 1px border), ghost (text-only). All states: hover, focus-visible ring (`2px --color-primary` offset 2), active, disabled (50% opacity).
- **Editor**: CodeMirror 6, light theme on `--color-bg`, mono 0.875rem/1.7, gutter in `--color-ink-3`, active line `--color-surface`, selection `--color-primary-soft`.
- **Transcript**: user messages right-tinted card (`--color-surface`), assistant prose plain on bg (72ch), tool calls as bordered mono cards with name + collapsed args.
- **Empty states**: teach the screen ("No sessions yet — start one with `jaz chat`"), never bare "nothing here".
- Loading: skeleton rows/blocks, not centered spinners.

## Layout & Motion

- Shell: fixed 264px sidebar + fluid content column (max 860px for prose pages, centered with generous 32–48px padding). macOS hiddenInset titlebar: 52px draggable strip above sidebar.
- Radius: 8px controls, 10px cards/editor. Subtle shadows only on overlays (toast).
- Motion: 150–250ms ease-out; state only (route fade ≤150ms, toast slide-in, save flash, live transcript rows fade-in). Full `prefers-reduced-motion: reduce` fallbacks (instant/crossfade).
- Z-scale: `--z-dropdown: 10`, `--z-toast: 20`, `--z-tooltip: 30`.
