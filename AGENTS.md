# Engineering Rules

- Use Go 1.26.
- Keep code and JSON minimal. Each line of code should fight for its existence; every field and line must earn its place.
- Trim strings at real input boundaries only: user input, config files, env vars, HTTP payloads, CLI args, and persisted loose text. Do not sprinkle `strings.TrimSpace` over internal constants, typed IDs, enum values, or values that have already crossed a validation boundary.
- When there's an opportunity for dramatic simplification or restructuring, bring it up. Favor "code judo" moves that delete layers, unify shapes, collapse special cases, or make the design inevitable over incremental patches.
- Bug fixes should first look for deletion or correction of the underlying contract. A solution that only adds branches, flags, helpers, or UI glue is suspicious; prefer removing stale paths, collapsing duplicated state, or moving behavior to the owning layer before adding code.
- Do not add code comments until they are genuinely needed to explain specific behavior the code itself cannot describe.
- Keep concrete implementations focused and interfaces small.
- Put behavior in the layer that owns the concept. Shared transcript/message shapes belong in storage or a dedicated shared package, not copied through server, ACP, and UI paths.
- Keep provider-facing data separate from display/transcript data. Do not mutate prompts and then repair snapshots by string matching; carry explicit typed boundaries instead.
- Architect the native Jaz agent behind protocol-shaped interfaces, modeled after MCP-style request/response/event contracts. Typed content blocks, tool calls/results, permissions, streaming updates, and capabilities should cross explicit interfaces instead of direct server/provider coupling.
- Keep native runtime behavior transport-neutral. Native, ACP, and future protocol adapters should share internal turn/session/tool contracts; protocol-specific code translates only at the boundary.
- Preserve migration paths to ACP or MCP-style protocols when adding agent features. Prefer capabilities and feature detection over hardcoded runtime branches, and do not hide native-only semantics inside prompts.
- Split files when a feature starts mixing transport, persistence, formatting, and UI concerns. Avoid pushing files toward 1k lines without a strong structural reason.
- Frontend shared hooks and lib code must not import component-owned types. Put cross-layer contracts in `lib`.
- Keep feature diffs scoped. Do not mix unrelated UI polish, settings work, dependency churn, or generated output into behavioral changes.
- Prefer Viper's default field mapping. Add `mapstructure` tags only for real mismatches.
- Keep `main.go` files as command dispatch and process entrypoints only. Arbitrary domain types, helper functions, clients, transports, URL builders, and request/response shapes belong in the package that owns that concept; the only allowed exception is global Viper bootstrap/config wiring.
- Use Fx constructors directly in `fx.Provide`; avoid pass-through wrappers.
- Do not add defensive nil checks for required constructor-injected dependencies. If a required Fx service is missing, fail fast instead of silently degrading; model truly optional dependencies explicitly.
- Codex ACP defaults to the user's Codex OAuth credentials. Never silently pass coordinator provider keys to Codex subprocesses; a provider API key reaches Codex only when the user explicitly selects a non-OpenAI model provider (e.g. OpenRouter) for it.
- Target deployments run the Jaz server on a VM and clients on user computers; never assume client-local file paths are visible to the server or agents.
- Every test you add must be useful: it must run in the relevant verification path and either protect real behavior or clarify a tricky contract. A test that is skipped, does not run, or provides no useful signal must not exist just to raise coverage.
- Reference repos (`openclaw`, `hermes`) are learning material, not authority.

## Backend Architecture

- Add boundaries to control real growth, not to satisfy a template. A tiny endpoint may stay direct; once transport parsing, feature policy, persistence, or presentation concerns mix, split them before the file becomes a dumping ground.
- Treat `backend/internal/server` as the HTTP process shell: router mounting, middleware, auth wrapping, health, and cross-cutting server lifecycle only. New feature behavior should not be added as `Server` methods unless it is truly server infrastructure.
- Keep backend imports pointed in one direction: `server` -> `httpapi/<feature>` -> `internal/<feature>` -> `storage` contracts. Feature packages must not import `server`, `httpapi`, or concrete storage implementations such as `storage/sqlite`; production storage adapters must not import feature packages.
- Put HTTP adapters in `backend/internal/httpapi/<feature>` when an endpoint has meaningful request parsing, response DTOs, or status-code mapping. HTTP DTOs live there; feature packages expose domain records/views unless the wire shape is genuinely the domain shape.
- Put feature semantics in `backend/internal/<feature>` when there is validation, aggregation, orchestration, policy, or a reusable view. Keep services small, depend on storage contracts, and return typed/domain errors only where callers need to distinguish them.
- Prefer app/Fx wiring for concrete stores, services, and handlers as slices mature. Avoid pushing new feature constructors into `server`; move toward mounted handlers or route modules without introducing pass-through wrapper constructors.
- Keep multi-record writes atomic in storage methods. Introduce a transaction/unit-of-work abstraction only when repeated cross-record operations need it; never smuggle concrete SQLite handles into feature services.
- Add explicit clock/`Now` dependencies only when date/time behavior needs deterministic tests or policy control. Add actor/workspace context only where authorization or workspace policy depends on it; services must not read HTTP request headers.
- Keep durable data contracts in `backend/internal/storage`. Storage exposes records, events, and mutation primitives; feature services derive user-facing views. Add projection storage only when a derived view is demonstrably expensive.
- For SQLite, put migrations in `backend/internal/storage/sqlite/migrations`, query SQL in `backend/internal/storage/sqlite/queries/<feature>`, and generated Go in `backend/internal/storage/sqlite/generated/<feature>` using plain feature names, not redundant `*db` suffixes. Handwritten `backend/internal/storage/sqlite/*.go` files may open/configure the DB, run migrations, manage transactions, and map rows, but must not contain raw query SQL strings.
- Test at the lowest boundary that protects behavior: service tests with fake storage for policy, SQLite adapter tests against a temp DB for persistence, HTTP API tests for status/DTO mapping, and end-to-end route tests only for routing or middleware behavior.
- Split packages before they sprawl: if `httpapi/<feature>` or `internal/<feature>` starts mixing routing, validation, aggregation, formatting, errors, and storage contracts in one large file, split into focused files such as `handler.go`, `service.go`, `errors.go`, and `types.go`.
- Use the usage feature as the golden slice for nontrivial features: `internal/usage` owns usage semantics and derived daily buckets, `internal/httpapi/usage` owns HTTP, `storage` exposes usage events, `internal/storage/sqlite/queries/usage` owns SQL, and `internal/storage/sqlite/generated/usage` is sqlc output.
- For ACP/native/runtime work, keep protocol adapters at the boundary. Internal runtime contracts own turns, tools, permissions, usage, capabilities, and streaming events; adapters translate protocol-specific shapes only at the edge.

## Integrations Goal

- Build Jaz integrations around `Observe + Act + Materialize`: providers observe external changes into typed records, expose actions for agents, and optionally materialize records into logical artifacts.
- Keep connector packages reusable and provider-focused. Connectors must not know `~/.jaz`, jazmem paths, source-page layout, scheduler internals, dedupe, retries, notifications, or exact filesystem destinations.
- Jaz runtime owns connection lookup, OAuth client construction, scheduling, cursors, dedupe, retries, notifications, raw JSONL writing, artifact writing, and jazmem-compatible source-page writing.
- Store observed provider data as append-only raw JSONL first, then materialize into source pages or artifacts. Do not write directly into curated jazmem lanes like `people/` or `projects/`; dream/promotion owns curated memory.
- Prefer source materialization by domain lane, for example `sources/email` and `sources/chat`. For chat/social data, batch by conversation/day or importance instead of creating one markdown page per event.
- Use generic OAuth connection storage and refresh for both first-party integrations such as Gmail/Calendar and user-added OAuth MCP servers such as Linear or n8n.
- Expose connected accounts to agents through a Jaz-managed MCP surface with stable, readable tool names. Support multiple accounts per provider through connection aliases such as `personal` or `work`.
- Treat Matrix as the first chat/social connector model: observe Matrix sync events into chat records, expose conversation/message actions, and materialize chat artifacts through the Jaz runtime rather than from the connector.
