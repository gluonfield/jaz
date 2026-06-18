# Jazmem Commands And Responses

Use this reference when you need exact CLI commands, response shapes, server endpoints, eval behavior, or maintenance details.

## Core Commands

Search:

```bash
jazmem ask "what do we know about Leeroo"
jazmem ask --deep "what connects Ink and Leeroo"
jazmem "Ink enterprise Claude deployment"
jazmem search --limit 5 "Oxford Edge Irwin Zaid"
jazmem search --text "physics reasoning environment"
jazmem search --deep "Alice Acme history"
jazmem --agentic "what do we know about Leeroo"
jazmem --agentic --deep "what do we know about Leeroo"
jazmem "who works at Acme"
jazmem "what connects Alice and Widget Co"
```

`--deep` is the single compute knob. Raw search: wider candidate pool plus two-hop link expansion. Agentic: also retrieves more pages (30 instead of 12), packs more evidence (48 chunks instead of 20), and runs a gap-driven second retrieval round when the first answer leaves gaps. Use it as an escalation, not a default.

Read pages:

```bash
jazmem file projects/ink
jazmem get projects/ink
jazmem get --raw projects/ink
jazmem get --body projects/ink
```

Inspect:

```bash
jazmem doctor
```

Maintenance is handled by the Jaz scheduler. Do not run maintenance commands during ordinary memory writing unless explicitly asked.

## Server Mode

The CLI prefers a running server so one process owns all index writes (no version skew between CLI and server binaries):

- No flags: auto-detects `http://127.0.0.1:5299/jazmem` (jaz embedded API), then `http://127.0.0.1:9477` (standalone jazmem-server); falls back to direct database access. A stderr line reports which server was picked.
- `--server URL` or `JAZMEM_SERVER`: pin a specific server (works for remote hosts).
- `--local`: force direct database access, skipping detection.
- Routed commands: search, get, file, and doctor. Maintenance commands are for explicit jazmem-internals work only.

```bash
jazmem doctor                          # auto-detects the jaz server when running
jazmem --server 192.168.1.10:5299/jazmem "Alice open loops"
```

## Raw Search Response

`jazmem <query>` and `jazmem search <query>` return `SearchResponse`:

```json
{
  "results": [
    {
      "slug": "people/alice",
      "title": "Alice Smith",
      "score": -0.00000157,
      "modified_at": "2026-06-08T12:00:00Z",
      "matches": [
        {
          "chunk": 0,
          "snippet": "Alice and Riley are friends...",
          "score": -0.00000157
        }
      ]
    }
  ],
  "stats": {
    "pages": 1,
    "chunks": 1,
    "graph_hits": 0
  }
}
```

Interpretation:

- `results`: ranked page-level hits.
- `matches`: matched chunks under each page.
- `stats.pages`: returned pages.
- `stats.chunks`: returned matched chunks.
- `stats.graph_hits`: extra pages added by link/backlink expansion.
- `score`: best score for the page. Lower is better for SQLite BM25.
- `title`: frontmatter `title`, then first `# H1`, then slug tail.
- `slug`: markdown path under root without `.md`.
- `via`: present only when it matters — `relationship` (typed-edge match) or `link` (neighbor pulled in by expansion, not a direct match). Absent via = direct text/title hit. The slug prefix is the page lane (`people/`, `inbox/`, ...); canonical lanes are curated, `inbox`/`sources` are raw.
- `modified_at`: when the page last changed — a staleness signal.
- `dreams/` pages (dream runs, review queues) are jazmem bookkeeping and are excluded from search; read them directly by slug.

To dig deeper on any result, `jazmem get <slug>` returns the full page including `links` and `backlinks` (its graph neighborhood) for further hops.

## Agentic Response

`jazmem --agentic <query>` returns `AgenticResponse`:

```json
{
  "answer": "Alice and Riley are friends and have worked on jazmem search.",
  "citations": [
    {
      "id": 1,
      "slug": "people/alice",
      "title": "Alice Smith",
      "chunk": 0
    }
  ],
  "gaps": [],
  "stats": {
    "pages": 12,
    "chunks": 13,
    "graph_hits": 1
  },
  "model_used": "openai/gpt-5.4-mini",
  "rounds": 1,
  "synthesis_ok": true,
  "diagnostics": {
    "pages_gathered": 12,
    "chunks_gathered": 13,
    "graph_hits": 1
  }
}
```

Rules:

- Agentic mode requires the configured provider's key, such as `OPENROUTER_API_KEY` for OpenRouter or `OPENAI_API_KEY` for OpenAI.
- Agentic mode uses its own internal retrieval budget. `--limit` does not control it; `--deep` is the only agentic retrieval knob.
- With `--deep`, `rounds` can be 2 and `diagnostics` gains `followup_queries` and `followup_chunks`.
- `citations[].id` matches the [n] markers in the answer text; `jazmem ask` renders them as a Sources legend.
- Raw search remains deterministic and does not call an LLM.
- Use agentic mode when answering the user directly; use raw search when selecting pages to read or edit.

Provider env:

- `PROVIDER_ENDPOINT`: OpenAI-compatible base endpoint; defaults to `https://openrouter.ai/api/v1`.
- `OPENROUTER_API_KEY` or `OPENAI_API_KEY`: provider API key selected from `PROVIDER_ENDPOINT`.
- `MODEL`: chat model; defaults to `openai/gpt-5.4-mini`.
- `REASONING_EFFORT`: optional; sent as `reasoning_effort` when set.

## Other Responses

`jazmem get <slug>` returns `Page`:

- `slug`, `path`, `type`, `title`, `aliases`
- `frontmatter`, `body`, `raw`, `modified_at`
- `links`, `backlinks`: graph neighborhood as `{slug, type, source}` — type is `reference`, `mention`, or a typed relationship such as `works_at`; use these to hop to related pages for more context

`jazmem file <slug>` returns the canonical markdown file path as plain text.

Maintenance commands and `jazmem doctor` return JSON reports. `jazmem doctor` includes `typed_link_count`.

## Eval

Eval does not call an LLM. It checks expected slugs against raw retrieval results.

```bash
jazmem eval
jazmem eval --limit 3
jazmem eval --file ./my-eval.json
```

Reports:

- `hit_rate`
- `precision`
- `recall`
- `mrr`
- per-case returned slugs and expected slugs

Use eval before adding retrieval machinery such as embeddings, reranking, or boost tuning.

## Server

Start server:

```bash
jazmem-server --addr 127.0.0.1:9477
```

Endpoints:

```bash
curl 'http://127.0.0.1:9477/health'
curl 'http://127.0.0.1:9477/doctor'
curl 'http://127.0.0.1:9477/search?q=Alice%20Riley&limit=5'
curl 'http://127.0.0.1:9477/search?q=Alice%20Riley&deep=1'
curl 'http://127.0.0.1:9477/search?q=Alice%20Riley&agentic=1&deep=1'
curl 'http://127.0.0.1:9477/file/people/alice'
curl 'http://127.0.0.1:9477/file/people/alice?raw=1'
curl -X POST 'http://127.0.0.1:9477/reindex'
curl -X POST 'http://127.0.0.1:9477/dream'
curl -X POST 'http://127.0.0.1:9477/link-hygiene'
```

There is no capture endpoint. Store data by editing markdown files.

## MCP Server

Start `jazmem-server`; MCP is exposed on the same process:

```bash
jazmem-server
jazmem-server --root ~/.jaz/memory --db ~/.jaz/jazmem.sqlite
```

MCP client config:

```json
{
  "mcpServers": {
    "jazmem": {
      "url": "http://127.0.0.1:9477/mcp"
    }
  }
}
```

Jaztools MCP tools for Jaz agents:

- `memory_search`: input `{ "query": "...", "deep": true }` (`deep` optional); output is the delegated search worker's final text, usually containing an answer, references, checked pages, search notes, and meaningful gaps. It uses the configured Jaz memory-search ACP agent rather than direct jazmem provider keys.
- `memory_get_page`: input `{ "path": "people/alice" }`; primary text content is raw markdown. Structured output is metadata such as `{ "found": true, "path": "...", "type": "...", "aliases": [...], "modified_at": "...", "links": [...], "backlinks": [...] }` or `{ "found": false, "error": "not found: people/alice", "suggestions": [...] }`.

Standalone `jazmem-server` tools may differ from Jaztools and can require the provider-key-backed agentic mode described above.

MCP is read-only. There is no MCP write/capture/index/dream tool. To store data, edit markdown directly. Indexing, dreaming, and link hygiene are CLI/server/scheduler operations.
