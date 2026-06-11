# Writing Jazmem Markdown

Use this reference when storing data, creating pages, editing canonical pages, deciding where a memory belongs, or handling citations/relationships.

## Memory Horizons

`LONG_TERM.md` and `SHORT_TERM.md` live at the memory root and are injected into every jaz turn alongside today's `daily/` page. They are not indexed pages.

- `LONG_TERM.md`: dream-maintained, agents read-only. To surface something for it, record it in daily/ or inbox/ with a citation; dream promotes what recurs or was directly stated.
- `SHORT_TERM.md`: keep it true about the present. When focus, active projects, or open loops change, replace the affected lines immediately — do not append a history; daily/ is the history.
- `daily/YYYY-MM-DD.md`: append entries as the day unfolds under `## Notes` / `## Open Loops` (the scheduler creates the skeleton; create it with those sections if missing). Amend a bullet only when it was wrong or superseded the same day.

## Write Workflow

Agents store data by editing raw markdown files. Jazmem indexes it.

1. Search for existing pages before writing.
2. Use `jazmem file <slug>` to get the canonical file path for existing pages.
3. Edit the markdown file directly with the available filesystem editing tool.
4. For new pages, create `<root>/<slug>.md` with frontmatter and an H1.
5. Preserve exact user wording for ideas, preferences, decisions, and concerns.
6. Add source citations to every durable fact.
7. Run `jazmem index`.
8. Verify with a search query that should find the new memory.
9. Commit with plain git only if the memory root is a git repo and the user explicitly asks.

Do not manually update SQLite after markdown edits.

## Raw Or Uncertain Information

For raw or uncertain information, create an inbox/source markdown file directly:

```md
---
title: Alice launch update preference
type: inbox
created: 2026-06-08T12:00:00Z
source: chat
---

# Alice launch update preference

Alice said she prefers concise launch updates with explicit open questions.

[Source: User, chat, 2026-06-08]
```

Good raw locations:

- `inbox/YYYY-MM-DD-short-topic.md` for untriaged user context
- `daily/YYYY-MM-DD.md` for dated operating notes
- `sources/chat/YYYY-MM-DD-topic.md` for chat source material
- `sources/email/YYYY-MM-DD-topic.md` for email source material
- `sources/agent/YYYY-MM-DD-topic.md` for agent work traces

## Promote To Canonical Pages

Edit canonical markdown when the fact is durable and sourced:

- preference
- decision
- relationship
- current role
- recurring open loop
- stable project context
- reusable concept
- important agent learning

Before creating a canonical page:

1. Search for the entity/topic by name and aliases.
2. If a page exists, update it instead of creating a duplicate.
3. If no page exists, apply the notability gate.
4. Pick the most specific directory and slug.
5. Create the page with frontmatter, H1, citations, and wikilinks.
6. Run `jazmem index`.
7. Verify the page is findable by name and alias.

## Page Taxonomy

Use these directories unless the user establishes a new taxonomy:

- `people/` - people the user may refer to again
- `companies/` - companies, orgs, institutions
- `projects/` - ongoing projects and tools
- `concepts/` - reusable mental models and ideas
- `notes/` - durable notes without a stronger home
- `daily/` - daily rollups and dated operating notes
- `sources/email/` - imported email source material
- `sources/chat/` - imported chat source material
- `sources/agent/` - agent traces and problem-solving logs
- `inbox/` - raw markdown notes waiting for triage
- `dreams/runs/` - dream run records
- `dreams/review/` - ambiguous dream candidates

Slug rules:

- Lowercase, path-like, no `.md` in the slug.
- Examples: `people/alice-smith`, `projects/jazmem`, `concepts/brain-first-memory`.
- Slug maps to `<root>/<slug>.md`.

## Aliases

Exact title/alias matches are the strongest retrieval signal. Aliases are not optional metadata; they decide whether the page is findable.

- On page creation, record every name variant seen so far in `aliases:` frontmatter: nicknames, short forms, initials, legal names, handles, product names.
- Keep `title` canonical; variants belong in `aliases`.
- When a reasonable search variant fails to find an existing page, add that variant as an alias and run `jazmem index`. This is how retrieval self-heals.
- Do not add generic words as aliases (`engineer`, `the company`); an alias must point to this page specifically.

```md
---
title: Alice Smith
type: people
aliases: [Alice, A. Smith, alicedev]
---
```

## Canonical Page Shape

```md
---
title: Alice Smith
type: people
aliases: [Alice]
---

# Alice Smith

## Current

- Runs engineering at Acme. [Source: [[sources/chat/2026-06-08-alice-context]], 2026-06-08]

## Preferences

- Prefers concise launch updates with explicit open questions. [Source: User, chat, 2026-06-08]

## Relationships

- [[people/riley]] - friend. [Source: [[inbox/2026-06-08-lunch-note]], 2026-06-08]

## Open Loops

- Ask about Acme launch timeline. [Source: [[inbox/2026-06-08-acme-launch]], 2026-06-08]

## Sources

- [[sources/chat/2026-06-08-alice-context]]
- [[inbox/2026-06-08-lunch-note]]
```

Recommended sections:

- People: `Current`, `Preferences`, `Relationships`, `History`, `Open Loops`, `Sources`
- Companies: `Current`, `People`, `Projects`, `History`, `Open Loops`, `Sources`
- Projects: `State`, `Decisions`, `Design`, `Open Loops`, `Sources`
- Concepts: `Summary`, `Use When`, `Examples`, `Related`, `Sources`
- Agent traces: `Problem`, `Approach`, `Fix`, `Lessons`, `Sources`

Keep pages scannable. Prefer bullets for factual memory. Avoid long unsourced prose. When a page grows past roughly 150 lines, move detail into a `sources/` page and keep a summarized bullet with a wikilink to it.

## Citations

Every durable fact written to a canonical page should include a source.

Citation forms:

- User statement: `[Source: User, chat, 2026-06-08]`
- Source page: `[Source: [[inbox/2026-06-08-note]], 2026-06-08]`
- Email: `[Source: email from Alice re: Acme launch, 2026-06-08]`
- Chat: `[Source: chat with Alice, 2026-06-08]`
- Agent work: `[Source: [[sources/agent/2026-06-08-jazmem-implementation]], 2026-06-08]`
- Web/external: `[Source: Publication, URL, 2026-06-08]`

Always use absolute dates in citations (`2026-06-09`), never relative ones like "yesterday" or "last week".

Source precedence when sources conflict:

1. User direct statements
2. Canonical memory pages with citations
3. Raw inbox/source markdown pages
4. External sources

When memory conflicts, do not silently choose one. Preserve both claims with dates and sources, then flag the contradiction.

## Updating Facts Over Time

Canonical pages describe the present. When a fact changes:

1. Update the `## Current` bullet in place with the new fact, source, and date.
2. Move the displaced fact to `## History` with a date range:

```md
## History

- Ran engineering at Acme (2024 to 2026-05). [Source: User, chat, 2026-06-09]
```

3. Never leave a known-stale fact in `## Current`.

When a relationship ends, move its bullet from `## Relationships` to `## History` with a date range. Typed edges are only indexed from `## Relationships`, so the move drops the edge while the record survives. Update both pages of a reciprocal relationship.

## Memlinks And Relationships

Use explicit wikilinks for durable references:

```md
[[people/alice]]
[[people/alice|Alice]]
[[projects/jazmem]]
```

Use `## Relationships` for stable relationships only:

```md
## Relationships

- [[companies/acme]] - works at. [Source: User, chat, 2026-06-08]
- [[companies/widget-co]] - invested in. [Source: User, chat, 2026-06-08]
- [[people/riley]] - friend. [Source: User, chat, 2026-06-08]
```

Jazmem indexes typed relationship edges from explicit wikilinks inside `## Relationships` sections.

Supported v1 labels:

- `works at` -> `works_at`
- `works with` / `collaborator` -> `works_with`
- `founder` / `founded` -> `founder_of`
- `invested in` / `investor` -> `invested_in`
- `advisor` / `advises` -> `advises`
- `friend` -> `friend`

Relationship query examples:

```bash
jazmem "who works at Acme"
jazmem "who invested in Widget Co"
jazmem "who founded Widget Co"
jazmem "what companies has Alice invested in"
jazmem "who are Alice's friends"
jazmem "what connects Alice and Widget Co"
```

Rules:

- Do not create reciprocal relationship bullets for ordinary mentions.
- Do create reciprocal relationship bullets for durable relationships such as friend, works with, founder, advisor, investor, collaborator.
- If unsure, write a raw markdown note to `inbox/` or `dreams/review/`.
- After creating a new entity page, run `jazmem link-hygiene` to generate relationship proposals.
- Promote proposals only by manually editing canonical markdown, then run `jazmem index`.

## Acquiring Context While Working

Classify user-provided context immediately:

1. Raw observation - write exact wording into `inbox/`, `daily/`, or `sources/*`.
2. Durable fact - update canonical page with citation.
3. Relationship - update `## Relationships` on both relevant pages when high confidence.
4. Preference - update the person/user/project page with date and source.
5. Decision - update the project page under `## Decisions`.
6. Open loop - update the relevant page under `## Open Loops`.
7. Reusable process - update `concepts/` or `sources/agent/`.

For significant work sessions, produce memory artifacts:

- A source page under `sources/agent/` describing the problem, approach, commands/files changed, result, and lessons.
- Canonical project updates under `projects/`.
- New concepts only when the pattern is reusable.
- New people/company pages only when they pass the notability gate.

Notability gate:

- People: likely to recur, connected to user work, preferences, decisions, or relationships.
- Companies: relevant to user work, investments, projects, or recurring references.
- Concepts: reusable mental model or repeated theme.
- Otherwise: keep it in `inbox/` or a source page.
