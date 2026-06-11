# Product

## Register

product

## Users

One person: the owner of this jaz instance. A technical user running a personal AI assistant platform on their own machine, usually alongside terminals and editors. They open the desktop app to check what their assistant and its agent sessions are doing, to read transcripts, and to tune the agent's identity files (AGENTS.md, SOUL.md). Sessions of use are short and frequent: glance, read, edit, close.

## Product Purpose

jaz desktop is the control surface for the jaz personal assistant backend (Go, REST + SSE). It shows recent sessions, streams live activity from running agent sessions, and edits the markdown files that define the assistant's behavior. Success: the owner always knows what their assistant is doing and can shape it without touching the filesystem.

## Brand Personality

Warm companion. Friendly, personable, calm — a capable assistant, not an industrial console. Three words: warm, attentive, unhurried. The interface should feel like a well-organized notebook kept by someone who likes you, not a server dashboard.

## Anti-references

- Generic SaaS admin dashboards: card grids, hero metrics, panel chrome everywhere.
- Chat-app clones: the sidebar is structured like ChatGPT's (recents + navigation) but the product is NOT chat-first — pages like Agent are first-class destinations, sessions are one section among them.
- Dark hacker-terminal aesthetics: jaz's TUI already covers that register; the desktop app is its daylight counterpart.

## Design Principles

1. **Glanceable state.** The primary job is "what is my assistant doing?" — session state, live activity, and freshness must read in under a second.
2. **The files are the product.** Editing SOUL.md should feel consequential and pleasant — editor quality over feature count.
3. **Brand through restraint.** Cobalt-tinted neutrals and one cobalt accent; the rainbow ramp appears only when jaz is alive (composer comet, live indicators, welcome field). Personality carried by color discipline, type, spacing, and copy.
4. **Earned familiarity.** Standard affordances (sidebar nav, tabs, save buttons) done precisely; no invented controls.
5. **Quiet motion.** Transitions convey state (live updates, saves, navigation), 150–250ms, nothing decorative.

## Accessibility & Inclusion

Light and dark themes, WCAG AA: body text ≥4.5:1 (target ≥7:1 for primary text), secondary text ≥3.5:1, focus visible on every interactive element, full `prefers-reduced-motion` alternatives. Keyboard: sidebar and tabs navigable, Cmd+S saves.
