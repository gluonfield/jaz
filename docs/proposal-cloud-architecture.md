# Proposal: Cloud Architecture for Jaz (Team Edition)

Adopt **Approach B — a multi-tenant per-company Control Plane (Postgres + S3-compatible object storage) paired with an ephemeral per-user `jaz` runtime running inside Kubernetes `agent-sandbox`**. Keep standalone local desktop mode unchanged using the exact same `jaz` binary.

This replaces the earlier "Approach A first" lean for three concrete reasons:
1. **Team plugins need a shared index.** Plugin navigation pages (`backend/internal/plugins/page.go`) aggregate threads and `status.json` across the team (e.g. a shared Tenders board across sales and estimation reps). Per-user SQLite volumes cannot serve cross-user pages or team threads without waking every user's instance or building a brittle sync layer.
2. **Storage boundaries already exist.** `backend/internal/storage/interfaces.go` already separates `SessionStore`, `MessageStore`, `SessionEventStore`, `SettingsStorage`, and `UsageEventStore`. The earlier concern about restoring a multi-GB SQLite database into every sandbox disappears once Postgres owns the thread event log and S3 owns blobs, workspaces, and memory.
3. **Kubernetes `agent-sandbox` covers cloud and bare metal with one deployment.** The same Helm/Kustomize bundle runs on EKS/AKS/GKE or a single bare-metal Linux host running `k3s`, giving customers full VPC or on-premise ownership plus white-labeling.

## 1. System topology and trust boundaries

~~~jaz-artifact Jaz cloud architecture and trust boundaries
<svg width="100%" viewBox="0 0 680 516" role="img">
  <title>Jaz cloud architecture and trust boundaries</title>
  <desc>Three-tier architecture showing user devices connecting to a company control plane backed by Postgres and S3-compatible storage, which orchestrates ephemeral per-user Kubernetes agent-sandbox pods with zero stored credentials.</desc>
  <defs>
    <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
    </marker>
  </defs>
  <rect class="c-gray" x="112" y="12" width="10" height="10" rx="2" stroke-width="0.5"/>
  <text class="ts" x="128" y="17" dominant-baseline="central">Devices and clients</text>
  <rect class="c-purple" x="274" y="12" width="10" height="10" rx="2" stroke-width="0.5"/>
  <text class="ts" x="290" y="17" dominant-baseline="central">Control plane (state)</text>
  <rect class="c-teal" x="454" y="12" width="10" height="10" rx="2" stroke-width="0.5"/>
  <text class="ts" x="470" y="17" dominant-baseline="central">Ephemeral sandbox</text>

  <g class="c-gray">
    <rect x="40" y="44" width="184" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="132" y="63" text-anchor="middle" dominant-baseline="central">Desktop app</text>
    <text class="ts" x="132" y="81" text-anchor="middle" dominant-baseline="central">Local mode or cloud</text>
  </g>
  <g class="c-gray">
    <rect x="248" y="44" width="184" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="340" y="63" text-anchor="middle" dominant-baseline="central">Web and mobile</text>
    <text class="ts" x="340" y="81" text-anchor="middle" dominant-baseline="central">Cloud client only</text>
  </g>
  <g class="c-gray">
    <rect x="456" y="44" width="184" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="548" y="63" text-anchor="middle" dominant-baseline="central">Connected machine</text>
    <text class="ts" x="548" y="81" text-anchor="middle" dominant-baseline="central">Outbound file bridge</text>
  </g>

  <line x1="132" y1="100" x2="132" y2="136" class="arr" marker-end="url(#arrow)"/>
  <line x1="340" y1="100" x2="340" y2="136" class="arr" marker-end="url(#arrow)"/>
  <line x1="548" y1="100" x2="548" y2="136" stroke="var(--t)" stroke-width="1.2" stroke-dasharray="4 3" marker-end="url(#arrow)"/>

  <rect x="40" y="138" width="600" height="196" rx="16" fill="none" stroke="var(--b)" stroke-width="0.75" stroke-dasharray="6 4"/>
  <text class="th" x="58" y="158" dominant-baseline="central">Company control plane</text>
  <text class="ts" x="622" y="158" text-anchor="end" dominant-baseline="central">One deploy per customer</text>

  <g class="c-purple">
    <rect x="58" y="176" width="174" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="145" y="195" text-anchor="middle" dominant-baseline="central">API gateway</text>
    <text class="ts" x="145" y="213" text-anchor="middle" dominant-baseline="central">OIDC, SSE, plugins</text>
  </g>
  <g class="c-purple">
    <rect x="253" y="176" width="174" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="340" y="195" text-anchor="middle" dominant-baseline="central">Model + MCP proxy</text>
    <text class="ts" x="340" y="213" text-anchor="middle" dominant-baseline="central">Keys, OAuth, memory</text>
  </g>
  <g class="c-purple">
    <rect x="448" y="176" width="174" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="535" y="195" text-anchor="middle" dominant-baseline="central">Sandbox controller</text>
    <text class="ts" x="535" y="213" text-anchor="middle" dominant-baseline="central">Claims, warm pool</text>
  </g>

  <g class="c-purple">
    <rect x="58" y="256" width="270" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="193" y="275" text-anchor="middle" dominant-baseline="central">Postgres</text>
    <text class="ts" x="193" y="293" text-anchor="middle" dominant-baseline="central">Threads, events, ACLs, jobs</text>
  </g>
  <g class="c-purple">
    <rect x="352" y="256" width="270" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="487" y="275" text-anchor="middle" dominant-baseline="central">S3-compatible object store</text>
    <text class="ts" x="487" y="293" text-anchor="middle" dominant-baseline="central">Blobs, views, memory, files</text>
  </g>

  <line x1="193" y1="336" x2="193" y2="370" class="arr" marker-start="url(#arrow)" marker-end="url(#arrow)"/>
  <line x1="487" y1="334" x2="487" y2="370" class="arr" marker-end="url(#arrow)"/>

  <rect x="40" y="372" width="600" height="116" rx="16" fill="none" stroke="var(--b)" stroke-width="0.75" stroke-dasharray="6 4"/>
  <text class="th" x="58" y="392" dominant-baseline="central">Kubernetes agent-sandbox</text>
  <text class="ts" x="622" y="392" text-anchor="end" dominant-baseline="central">Cloud or bare-metal k3s</text>

  <g class="c-teal">
    <rect x="58" y="412" width="270" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="193" y="431" text-anchor="middle" dominant-baseline="central">Per-user sandbox pod</text>
    <text class="ts" x="193" y="449" text-anchor="middle" dominant-baseline="central">Same jaz binary, zero keys</text>
  </g>
  <g class="c-teal">
    <rect x="352" y="412" width="270" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="487" y="431" text-anchor="middle" dominant-baseline="central">Warm pool (gVisor / Kata)</text>
    <text class="ts" x="487" y="449" text-anchor="middle" dominant-baseline="central">Pre-booted, scales to zero</text>
  </g>
</svg>
~~~

### Component ownership

| Component | Deployment | Responsibility | Key invariant |
| --- | --- | --- | --- |
| **Jaz runtime (`jaz`)** | Local desktop child process OR inside K8s sandbox pod | ACP/native agent loop, skills, local shell/code tools, plugin execution, workspace file sync | One open-source binary; never holds org secrets or OAuth refresh tokens in cloud mode |
| **Control plane** | Always-on Go service(s), one per customer (closed-source) | OIDC/Entra ID auth, org branding/plugin catalogue, thread ledger & SSE fan-out, ACLs, model gateway, connector/OAuth gateway, scoped memory router, job scheduler, sandbox controller | Single source of truth for all cloud/team state; Postgres `LISTEN/NOTIFY` + leases replaces Redis |
| **Postgres** | Managed cloud DB or in-cluster StatefulSet | Users, groups, spaces, threads, append-only `sessionevents`, ACLs, blob metadata, view-state versions, scheduled loop/webhook queue, audit log | Thread list, transcripts, plugin pages, and search load with zero sandbox cold start |
| **S3-compatible store** | AWS S3, GCS, Azure Blob S3-gateway, or MinIO/RustFS on bare metal | Deduplicated attachments, artifacts, tracked workspace files, `view-state` JSON, versioned Markdown memory | Clients download PDFs/DOCX/views directly via authenticated gateway or short-lived presigned URLs |
| **K8s `agent-sandbox`** | EKS/AKS/GKE or bare-metal `k3s` | `SandboxTemplate`, `SandboxWarmPool`, `SandboxClaim`, `SandboxRouter`, `RuntimeClass` (gVisor or Kata) | Ephemeral compute only; strictly isolated via `NetworkPolicy` |

### Security boundary (no secrets in the sandbox)

Borrowing the egress-proxy pattern from Kortix and Kilo:
- **Zero credentials inside a sandbox pod.** The sandbox receives only a short-lived, user- and thread-scoped JWT (TTL ~15 min, auto-renewed by the controller while a turn runs).
- **Model Gateway** holds provider keys (or customer Azure OpenAI / Vertex / Bedrock bindings), enforces org model/agent allowlists, and records token usage per user/space.
- **Connector Gateway** holds encrypted OAuth tokens (reusing Jaz's `connections` + AES-256-GCM model backed by Postgres + KMS/secret key), handles background refresh even while sandboxes are scaled to zero, exposes user- and org-scoped MCP endpoints to the sandbox, and logs every write action for IT audit.
- **Kubernetes `NetworkPolicy`** blocks all lateral pod-to-pod traffic, blocks direct Postgres/K8s API/metadata-IP access, and restricts egress to the Control Plane gateways plus an optional org-approved allowlist.

## 2. Avoiding OpenAI and Anthropic's "stuck local/remote" quirk

Both Claude Code and OpenAI Codex conflate **where a thread's history lives** with **where code executes**. Local threads live in local files/SQLite; cloud threads live on vendor servers; moving between them requires one-way copies (`teleport` or `codex apply`) or fragile live-control tunnels that stall when a laptop sleeps.

Jaz separates **Thread Home** from **Execution Target**:

~~~jaz-artifact Per-turn routing without stuck threads
<svg width="100%" viewBox="0 0 680 292" role="img">
  <title>Per-turn routing without stuck threads</title>
  <desc>A single cloud thread ledger and S3 blob store serves turns executed either on a cloud sandbox or on a connected machine, using checkpoint handoff when switching execution targets.</desc>
  <defs>
    <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
    </marker>
  </defs>

  <g class="c-purple">
    <rect x="160" y="20" width="360" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="340" y="39" text-anchor="middle" dominant-baseline="central">Cloud thread ledger + S3 blobs</text>
    <text class="ts" x="340" y="57" text-anchor="middle" dominant-baseline="central">One source for desktop, web, phone</text>
  </g>

  <path d="M 270 76 L 270 100 L 180 100 L 180 122" fill="none" class="arr" marker-end="url(#arrow)"/>
  <path d="M 410 76 L 410 100 L 500 100 L 500 122" fill="none" class="arr" marker-end="url(#arrow)"/>

  <g class="c-teal">
    <rect x="50" y="124" width="260" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="180" y="143" text-anchor="middle" dominant-baseline="central">Turn on cloud sandbox</text>
    <text class="ts" x="180" y="161" text-anchor="middle" dominant-baseline="central">Default, works laptop-closed</text>
  </g>
  <g class="c-gray">
    <rect x="370" y="124" width="260" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="500" y="143" text-anchor="middle" dominant-baseline="central">Turn on connected machine</text>
    <text class="ts" x="500" y="161" text-anchor="middle" dominant-baseline="central">Local files or intranet tools</text>
  </g>

  <line x1="314" y1="152" x2="366" y2="152" stroke="var(--t)" stroke-width="1.2" stroke-dasharray="4 3" marker-start="url(#arrow)" marker-end="url(#arrow)"/>

  <path d="M 180 180 L 180 196 L 270 196 L 270 210" fill="none" class="arr" marker-end="url(#arrow)"/>
  <path d="M 500 180 L 500 196 L 410 196 L 410 210" fill="none" class="arr" marker-end="url(#arrow)"/>

  <g class="c-purple">
    <rect x="120" y="212" width="440" height="56" rx="8" stroke-width="0.5"/>
    <text class="th" x="340" y="231" text-anchor="middle" dominant-baseline="central">Checkpoint handoff + live event stream</text>
    <text class="ts" x="340" y="249" text-anchor="middle" dominant-baseline="central">Switching target seeds fresh ACP session</text>
  </g>
</svg>
~~~

### How threads and machines work together

- **Two thread homes, never bi-directionally merged:**
  - **Standalone local thread** (unauthenticated or explicitly local-only on desktop): lives in local SQLite + local disk. Works offline with zero server. Optional one-way **Publish to cloud** uploads its event log and blobs to the Control Plane and hands off to a cloud thread.
  - **Cloud/team thread** (default when signed into a company workspace): ledger always lives in Control Plane Postgres and attachments/files live in S3. Readable and actionable from Desktop, Web, and iOS at all times.
- **Per-turn execution target (no stuck threads):**
  - Any turn on a cloud thread executes on either the user's **Cloud Sandbox** (default) or a **Connected Machine** (the user's online desktop or a registered office workstation).
  - Consecutive turns on the same target resume the provider's native ACP session directly.
  - Switching target (e.g. Turn 1 on laptop, Turn 2 from iOS on a train) triggers an explicit **checkpoint handoff**: the new target starts a fresh native ACP session seeded with a structured handoff (recent transcript + summary + S3 workspace/blob refs + memory refs) while appending to the **same** Jaz thread in Postgres. No forked chats, no stranded history.
- **Connected machines built on Jaz's existing `desktopbridge`:**
  - Jaz already implements outbound WebSocket JSON-RPC from Electron desktop to backend (`backend/internal/desktopbridge/connection.go`, `browsercontrol/desktop_host.go`, `computercontrol/connection.go`).
  - Generalize this into **`machinebridge`**: an online desktop app (or headless `jaz-node` daemon on an office Windows/Linux box) holds one outbound TLS WebSocket to the Control Plane — zero inbound ports, works cleanly behind corporate NAT/VPNs.
  - The user locally approves explicit capabilities per machine: `fs` (strictly scoped to allowed directories, e.g. `~/Tenders` or a mapped NAS share), `browser` (existing side-browser control), `computer` (existing native desktop control), and optionally `exec` (running full local turns).
  - **Default pattern for enterprise teams (Tool Target, not Remote Control):** The turn runs in the Cloud Sandbox and calls scoped MCP tools (`machine_fs_list`, `machine_fs_read`, `machine_stage_file`) through `machinebridge`. Any file pulled from the laptop is immediately cached into the thread's S3 prefix — so closing the laptop mid-run never breaks the cloud agent.

## 3. Kubernetes `agent-sandbox` on cloud and bare metal

Using [`kubernetes-sigs/agent-sandbox`](https://agent-sandbox.sigs.k8s.io/docs/) gives one standard orchestrator for EKS/AKS/GKE and single-node bare-metal `k3s`:

- **Primitives used:**
  - `SandboxTemplate`: generic base image containing managed agent CLIs, Python/Office tools for plugins, and a small bootstrapper that pulls/verifies the pinned `jaz` binary hash (so runtime upgrades rarely require rebuilding base images).
  - `SandboxWarmPool`: keeps N pre-booted, unclaimed pods warm (zero tenant data attached) for sub-second `SandboxClaim` allocation.
  - `SandboxClaim`: **one ephemeral sandbox per active user** (not per thread). A user working across three tender threads reuses one warm pod and hot file cache instead of paying three cold starts. When a second teammate takes over a shared thread, their own sandbox hydrates that thread's S3 workspace — users never share a container or filesystem.
  - `SandboxRouter`: Control Plane proxies sandbox RPC/streams through the router using `X-Sandbox-ID` / `X-Sandbox-Namespace`, avoiding raw Pod IP exposure.
- **Runtime isolation:**
  - **Cloud VMs (EKS/AKS/GKE):** `gVisor` (`runsc`) via `RuntimeClass` (works without nested KVM).
  - **Bare-metal Linux / metal instances:** `Kata Containers` (hardware KVM microVM per user) or `gVisor`.
- **Critical caveat — do not rely on VM memory snapshots:**
  - Full memory/register Pod Snapshots in `agent-sandbox` are currently GKE-specific; upstream bare-metal pause stops the pod while keeping its PVC.
  - **Design around this:** treat sandboxes as 100% disposable compute. Hydrate thread workspace files from S3 on claim/resume (or local NVMe cache + S3 sync), flush diffs on turn completion with optimistic `generation` numbers (preventing a stale pod from overwriting a newer run), reap idle ACP child processes aggressively (measured at 740–940 MB per open agent session), and scale idle pods to zero (short timeout ~5 min with no client attached, ~20 min while watched).
- **Keep a small `SandboxDriver` interface in Go:**
  - `Acquire(ctx, userID)`, `Endpoint(ctx, lease)`, `Release(ctx, lease)`.
  - Production driver: `K8sAgentSandboxDriver`. Local dev / tiny single-host driver: `DockerPodmanDriver` (making it easy to test locally or fold in existing Podman isolation work without requiring a local K8s cluster).

## 4. Abstracted `BlobStore` (S3) for attachments, plugins, and workspaces

Today Jaz scatters files across four local-disk assumptions: `.jaz-attachments/` (`backend/internal/server/attachment_store.go`), raw `media.Ref.BlobPath` (`backend/internal/media/ref.go`), `view-state/` (`backend/internal/plugins/views.go`), and thread working folders read directly by `plugins/page.go`.

Unify all four behind one `internal/blobstore` interface with two implementations:
- **`LocalFSBlobStore`** — standalone desktop mode (zero external dependencies).
- **`S3BlobStore`** — any S3-compatible store (AWS S3, GCS S3-interop, Azure Blob S3 gateway, or **MinIO / RustFS** inside bare-metal `k3s`).

### S3 key layout and Postgres index

| Bucket prefix | Contents | Why it matters |
| --- | --- | --- |
| `tenants/<tid>/blobs/<sha256>` | Immutable attachments & media (`att:<blob_id>`) | Content-addressed dedupe (200-page tender packs uploaded once); works identically in cloud sandboxes and local desktop cache |
| `tenants/<tid>/threads/<id>/files/<relpath>` | Tracked thread workspace files (`status.json`, `query-sheet.xlsx`, buyer PDFs/DOCX) + manifest generation | Opening the **Tenders** sidebar page or viewing a linked PDF/DOCX in the side panel serves straight from S3 + Postgres with **zero sandbox cold start** |
| `tenants/<tid>/threads/<id>/view-state/<plugin>/<view>/<file>.json` | Saved plugin review state (200 KB cap, `If-Match` versioned) | Multiple teammates reviewing/approving tender queries cannot silently overwrite each other |
| `tenants/<tid>/artifacts/<id>/v<N>` | Versioned standalone documents, canvases, HTML widgets | ACL-controlled sharing independent of thread lifecycle |
| `tenants/<tid>/memory/<scope>/...` | Markdown memory pages (`users/<uid>`, `spaces/<sid>`, `org`) | Human-readable, versioned, auditable, immediately synced on correction |

**Client upload/download path:** Desktop, Web, and Mobile never push large attachments through the Go API memory or leak client-local paths into prompts. Clients request a short-lived presigned upload URL from the Control Plane, upload straight to S3, and attach the resulting `blob_id` to the prompt. Local runs on a signed-in desktop fetch `blob_id` into a local content cache on demand.

## 5. Team memory, team threads, and enterprise plugins

### Three-tier memory scope (`jazmem`)

Extend `backend/internal/memoryservice` so cloud/team mode routes `memory_search` and `memory_get_page` through a **Scoped Memory Router** in the Control Plane, enforcing verified OIDC user/group claims server-side (never trusting sandbox input):

| Scope | Read access | Write policy | Connector ingestion | Example (manufacturing / tenders) |
| --- | --- | --- | --- | --- |
| **Personal (`user:<uid>`)** | User only (local + cloud) | User's threads + personal `dream` (plus opt-in one-way Mac upload) | User's Gmail/Outlook, Zoom, WhatsApp | Personal notes, drafting style, private follow-ups |
| **Team Space (`space:<sid>`)** | Space members (e.g. `DC Tenders`, `Non-DC Tenders`, `West Sales`) | **Shared-space threads or explicit "Save to team memory" only** — private threads never auto-write here | Shared mailboxes, space-scoped Salesforce/SharePoint folders | Past buyer query rulings, consultant preferences, regional positions |
| **Org (`org:<tid>`)** | All org users (or role-filtered) | Curators/admins only (or PR-style approval from agents; mirrorable from Git/SharePoint) | Approved company repositories / ERP read-only feeds | Product matrix, 40 engine data sheets, standard deviations, common remarks |

- **Propagation of fixes & deletions:** Every memory page stays Markdown in S3 with a revision row in Postgres. Correcting a data-sheet value or deleting a stale deviation immediately re-indexes that scope across the entire company with a full audit trail (`who`, `when`, `source thread`).
- **Response attribution:** `memory_search` queries `[personal, active_space, org]` in parallel and tags every returned citation with its scope (`[Personal]`, `[Space: DC Tenders]`, `[Org]`) so agents never confuse a private note with an approved company spec.

### Team threads and plugin pages

- **Visibility:** `Private` (default), `Shared` (explicit teammates/link), or `Space` (visible in the Space sidebar and aggregated into Space plugin pages).
- **Concurrency:** Many live SSE observers, **one active turn writer** enforced atomically by `ClaimTurn` in Postgres. Every message stores `author_user_id` (already started in migration `0075_message_authorship.sql`). Teammates can watch live, submit the next turn when idle, review/approve plugin views (`If-Match` optimistic locking on `view-state`), or click **Fork** (cheap pointer to parent event seq + copy-on-write S3 workspace manifest).
- **Team plugin pages:** `backend/internal/plugins/page.go` swaps its local disk scan (`os.OpenRoot`) for a `PageDataSource` backed by Postgres + S3 `status.json`. Opening **Tenders** shows every live tender in the rep's spaces instantaneously without booting sandboxes.
- **White-labeling & enterprise governance:**
  - Control Plane serves an org bootstrap payload at OIDC sign-in: custom branding (name/logo/accent for Web/Desktop/iOS), **pinned Company Plugin Catalogue** (auto-synced into every sandbox and client), approved models/agents, and an **IT kill switch** (per-connector read/write toggles + instant revocation).
  - **Commercial critical path (independent of K8s):** Enterprise customers on Microsoft 365 Copilot will require **Microsoft Entra ID** OIDC plus **Outlook, OneDrive, SharePoint, and Teams** connectors (plus Salesforce/SAP read-only MCPs). Placing connectors in the Control Plane means building each enterprise connector once with centralized OAuth admin consent and audit logging.

## 6. What to borrow (and skip) from `herdrdev/herdr`

Source review of [`herdrdev/herdr`](https://github.com/herdrdev/herdr) highlights five patterns worth adopting directly and two that do not fit Jaz:

| Pattern in `herdr` | Source reference | How Jaz should apply it |
| --- | --- | --- |
| **Frozen capability-negotiated endpoint envelope** | `src/protocol/endpoint.rs` (`endpoint.hello.v1` / `welcome.v1`, `capabilities`, `Unknown` enum fallbacks) | Version `machinebridge` and client-gateway handshakes with capability lists so desktop clients, `jaz-node` hosts, and cloud sandboxes upgrade independently instead of lockstep (`docs/remote-backend.md`). |
| **Multi-endpoint multiplexing with interest gating & isolated failure** | `src/client/endpoint/message_policy.rs`, `docs/connecting-machines.mdx` | Desktop connects to Local Runtime + Company Cloud simultaneously. Local opens immediately offline; background threads/endpoints receive lightweight status (`working`/`asking`/`idle`) until opened; an offline cloud or machine dims cached state without blocking local work. |
| **Sleep-aware application heartbeats** | `src/client/endpoint/health.rs` (60s startup window vs 5s/10s live probe, immediate reprobe on wake, idle bridge cleanup) | Upgrade `desktopbridge.KeepAlive` (currently a blind 25s ping / 75s read deadline) so `machinebridge` detects lid-close, VPN resets, and office Wi-Fi drops cleanly without killing cloud runs. |
| **Transparent local asset bridging on paste/drop** | `src/client/clipboard_images.rs` | Intercept pasted images and dragged local files in Desktop/Web/Mobile and upload them directly to `BlobStore` with size limits — never sending local paths to a remote turn. |
| **Single controller vs multiple observers & durable/transient split** | `src/handoff_runtime.rs`, `docs/persistence-remote.mdx` | Match team thread semantics: one turn-writer lease, unlimited live SSE observers, and explicit separation of durable state (Postgres + S3) from transient streams recreated on reconnect. |

**Do not copy from `herdr`:**
- **SSH-initiated remote connections:** assumes developer SSH keys and inbound host reachability; non-technical enterprise teams behind corporate firewalls need OIDC + outbound TLS WebSockets.
- **PTY / OSC 7501 terminal scraping:** Jaz already owns structured ACP/native typed `sessionevents`, which power mobile UI, plugin views, and audit logs cleanly.

## 7. Phased build order

1. **Phase 1 — Runtime storage & blob interfaces (no behavior change locally):**
   - Introduce `internal/blobstore` (`LocalFSBlobStore` + `S3BlobStore`), migrating `attachment_store`, `media.Ref`, `plugins/views.go`, and `plugins/page.go` off raw disk paths.
   - Add Postgres implementation of `storage.Store` alongside SQLite (same event schema).
   - Add `worker` runtime mode flags (`--control-plane-url`, short-lived JWT auth, workspace S3 sync on turn boundaries).
2. **Phase 2 — Control plane + K8s `agent-sandbox` + team threads:**
   - Stand up Control Plane (OIDC/Entra ID, API gateway, Postgres thread/event service, Model Gateway, `K8sAgentSandboxDriver` + local `DockerPodmanDriver`, Helm chart for cloud & bare-metal `k3s` + MinIO).
   - Ship team threads (Private / Shared / Space), team plugin pages (`tenders`), and org-pinned plugin catalogue + white-label config.
3. **Phase 3 — Three-tier memory + centralized enterprise connectors:**
   - Deploy Scoped Memory Router (`Personal` / `Space` / `Org`) with explicit promotion and curator controls.
   - Move OAuth vault & MCP proxy to Control Plane `ConnectorGateway`; add Microsoft 365 (Entra/Outlook/SharePoint/OneDrive/Teams) and Salesforce/SAP connectors.
4. **Phase 4 — Connected machines (`machinebridge`) & cross-target turns:**
   - Generalize `desktopbridge` with `herdr`-style capability negotiation and sleep-aware health probes.
   - Add folder-scoped `machine_fs_*` tools (staging reads into S3) and per-turn local execution with checkpoint handoff.

## 8. Open decisions

- [ ] **First deployment target for Sterling Green:** customer cloud (AWS EKS / Azure AKS), CAS-managed cloud VPC, or on-premise bare-metal `k3s` box?
- [ ] **First-wave enterprise connectors:** do we prioritize Microsoft 365 (Entra ID + Outlook/SharePoint/OneDrive) + Salesforce first, or Google + Zoom + manual tender uploads for the initial team rollout?
- [ ] **Connected machine priority:** is desktop file-bridge (`machinebridge` reading approved local/NAS folders into cloud runs) needed in Phase 2, or can initial team users drag-and-drop tender packs straight into S3?
- [ ] **Team memory curation UX:** should writes to `Org` memory (data sheets, standard deviations) require explicit lead/admin approval in Jaz, or mirror read-only from a customer-owned SharePoint folder / Git repo first?
- [ ] **Control plane repository & licensing:** confirm whether the Control Plane lives in a separate private Go repo importing Jaz's shared contracts (`storage`, `sessionevents`, `blobstore`) so the open-source `jaz` repo stays cleanly standalone.
