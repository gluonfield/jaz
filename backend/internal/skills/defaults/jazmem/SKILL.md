---
name: jazmem
description: Use jazmem CLI and markdown memory. Trigger when searching, reading, citing, storing, organizing, reindexing, evaluating, dreaming, or maintaining durable personal memory in jazmem, including the LONG_TERM/SHORT_TERM/daily memory horizons.
metadata:
  short-description: Search and maintain jazmem memory
---

# Jazmem

Markdown-first personal memory. Markdown files are the source of truth; SQLite is a rebuildable index. Default root `~/.jaz/memory`, db `~/.jaz/jazmem.sqlite` (override: `JAZMEM_ROOT`, `JAZMEM_DB`, `--root`, `--db`).

The CLI auto-connects to a running server (jaz at `:5299/jazmem`, or jazmem-server at `:9477`) so the server stays the single index writer; `--server URL`/`JAZMEM_SERVER` pins one, `--local` forces direct database access. Editing markdown files always works directly — only index/search/dream operations route through the server.

## Memory Horizons

Three files are injected into every jaz turn. Know who writes what:

| Surface | Holds | You (agent) | Dream (nightly) |
|---|---|---|---|
| `LONG_TERM.md` | identity, goals, standing preferences, key people | **read-only** | sole writer; facts must recur or be directly stated |
| `SHORT_TERM.md` | current focus, active projects, open loops | **update in place, live**, when the present changes | prunes stale entries |
| `daily/YYYY-MM-DD.md` | raw log of today | **append as you go**, mid-session, not at session end | reads, never writes |

Rules:

- SHORT_TERM.md says what is true about the present and gets overwritten; daily/ says what happened and never does.
- Capture immediately when you learn something durable: append to today's daily page, update SHORT_TERM.md if focus/loops changed, run `jazmem index`. Memory is a behavior, not a backup.
- The current daily page is in your context — append what's new, amend the bullet that changed, never rewrite the page.
- Never edit LONG_TERM.md; if something belongs there, it will earn its way in via dream. Mention it in daily/ with a citation.

## Core Rules

- Check jazmem before answering about people, projects, preferences, decisions, relationships, open loops, "what do we know".
- Ground claims in citations; absolute dates only (`2026-06-10`, never "yesterday").
- Write declarative facts, not instructions: "User prefers concise updates" ✓, "Always be concise" ✗.
- If a fact will be stale in 7 days, it belongs in daily/, not on a canonical page. No PR numbers, SHAs, "fixed bug X". Reusable procedures belong in skills, not memory.
- Store data by editing markdown, then `jazmem index`. Never treat SQLite as truth or edit it directly.
- Record every known name variant in `aliases:` frontmatter — exact title/alias match is the strongest retrieval signal.
- Keep `## Current` current: displaced facts move to `## History` with date ranges; ended relationships move out of `## Relationships` (that drops the typed edge).
- Uncertain or raw material goes to `inbox/`, exact wording preserved, not to canonical pages.

## Search

```bash
jazmem ask "what do we know about Alice"   # ANSWER: LLM synthesis + citations + gaps
jazmem ask --deep "..."                    # + bigger budget + gap-driven second round
jazmem "Alice Acme open loops"             # raw retrieval: ranked pages + chunks, free
jazmem search --limit 5 --text "..."       # rendered text
jazmem "who works at Acme"                 # typed-edge relational forms
jazmem "what connects Alice and Riley"
jazmem search --deep "..."                 # escalation: wider pool + two-hop links
```

`ask` answers a question; raw search finds pages. Use raw when picking pages to read or edit; use `ask` when answering the user. (`jazmem --agentic` is the JSON form of ask.)

Raw results carry `modified_at` (staleness) and, only when it matters, `via`: `relationship` = typed-edge match, `link` = neighbor pulled in by expansion (not a direct match). The slug prefix is the lane — canonical lanes are curated, `inbox/`/`sources/` are raw. `--limit` does not affect ask/agentic; `--deep` is the only compute knob — an escalation, not a default.

### When Search Misses

1. Reformulate with concrete nouns, not question words.
2. Try name variants; try the relational forms.
3. `jazmem search --deep --limit 20 "<query>"`.
4. `jazmem get --raw <slug>` the closest hit; follow its wikilinks, `links`, and `backlinks`.
5. Only then say memory is missing. If a legitimate variant failed, add it as an alias and reindex.

## Read and Write

```bash
jazmem get people/alice        # page JSON incl. links/backlinks (graph neighborhood)
jazmem get --raw people/alice  # raw markdown
jazmem file people/alice       # path for editing; not-found returns suggestions
```

Write path: search first → `jazmem file <slug>` → edit markdown → for new pages create `<root>/<slug>.md` with frontmatter, H1, aliases → cite every fact `[Source: ..., YYYY-MM-DD]` → close with `jazmem index && jazmem search "<verifying query>"`. New canonical pages must pass the notability gate; when unsure, `inbox/` instead.

Lanes: `people/ companies/ projects/ concepts/ notes/` (canonical) · `daily/ inbox/ sources/{email,chat,agent}/` (raw) · `dreams/{runs,review}/` (dream's).

Typed relationships index only from `## Relationships` wikilink bullets with supported labels (`works at`, `works with`, `founder`, `invested in`, `advisor`, `friend`):

```md
## Relationships
- [[companies/acme]] - works at. [Source: User, chat, 2026-06-10]
```

Details: [references/writing-memory.md](references/writing-memory.md). Commands/schemas: [references/commands.md](references/commands.md).

## MCP Tools

Served by jaz at `http://127.0.0.1:5299/mcp/jazmem` (or standalone `jazmem-server` at `:9477/mcp`). Read-only — writes happen by editing markdown.

- `jazmem_search`: agentic cited answer; `deep: true` when thin.
- `jazmem_search_raw`: deterministic retrieval (`limit`, `deep`); drives your own search→get→follow-links loop.
- `jazmem_get`: raw markdown + links/backlinks + near-miss suggestions.

## Maintenance

`jazmem index` (rebuild after edits) · `jazmem doctor` (counts) · `jazmem eval` (fixed retrieval eval, no LLM) · `jazmem dream` (consolidation: promotes cited bullets, maintains LONG_TERM/SHORT_TERM) · `jazmem link-hygiene` (relationship proposals → review). The jazmem scheduler runs reindex/dream/hygiene automatically inside jaz.

## Anti-Patterns

- Answering from general knowledge when jazmem has memory; concluding "missing" after one search.
- Deferring capture to session end; editing LONG_TERM.md directly.
- Unsourced facts; relative dates; imperative phrasing; artifact IDs that rot in a week.
- Alias-less new pages; stale `## Current` bullets; relationships buried in prose.
- Forgetting `jazmem index`; treating SQLite as truth; `--deep` on every query.
