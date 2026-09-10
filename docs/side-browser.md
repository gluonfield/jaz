# Jaz side browser

Enable browser tools in **Settings → Browser** and select **Jaz side browser**.
Keep the conversation open in the desktop app while the agent works. The shared
Jaz MCP tools expose this browser to any ACP provider that supports those tools.
Closing the panel, leaving the conversation, or cancelling a running tool stops
its browser work. Other conversation tabs cannot be claimed.

## Import browser sign-ins

The desktop side browser offers **Import sign-ins** on first use. Choose a
browser profile, then explicitly select the sites to copy. Dismissing the offer
is remembered; reopen it from **Settings → Browser → Jaz side browser → Import sign-ins**.
Chrome and Edge profiles are supported on macOS; Firefox profiles are supported
on macOS, Windows and Linux. The import is shared by side-browser tabs on that
computer and works independently of the selected ACP agent.

The Electron process reads the selected profile's cookie database locally and
imports only the selected sites into the existing preview partition. Source
profiles remain unchanged. On macOS, encrypted Chrome/Edge cookies require
permission to read that browser's Safe Storage key from Keychain. Cookie values
and keys are not returned through the import UI, MCP or server endpoints.
Agents can subsequently use the signed-in browser and observe its pages.

This copies cookies, without passwords, history, bookmarks, extensions or
ongoing synchronization. Partitioned cookies and Firefox container cookies are
excluded because Electron's cookie setter cannot preserve their isolation.
Session cookies keep their original lifetime; some sites may require signing in
again. Unsupported or damaged cookies are reported as failed imports.
Database access uses Electron's built-in Node SQLite API and adds no dependency.

## Browser identity

Before creating a side-browser tab, Jaz configures that preview session's native
User-Agent to omit the Jaz and Electron product tokens. The actual Chromium
version and platform remain intact and follow the installed Electron version.
The app's own session keeps its identity. Chromium continues to generate client
hints, language, fetch metadata, cookies and other request headers itself; page
and worker JavaScript observe the same User-Agent as network requests.
This uses [Electron's session API](https://www.electronjs.org/docs/latest/api/session#sessetuseragentuseragent-acceptlanguages)
without per-request header rewriting or browser-property patches.

This improves browser compatibility. A browser identity does not establish that
a person performed an action, and cannot guarantee that a site allows automation
or will keep an account unrestricted.

## Agent interface

`browser_navigate`, `browser_read_page`, `browser_find`, `browser_click`,
`browser_form_input`, `browser_key`, `browser_scroll`, `browser_wait`, and
`browser_screenshot` use the selected backend. `browser_hover` and `browser_drag`
also support the side browser and managed Chromium.

`browser_get_ax_state` reads Chromium's actual accessibility tree in the side
browser. It exposes computed names, roles, values and states, including closed
shadow roots and documents in same-site and cross-site frames. Numeric indices
identify DOM nodes within their document and stay stable across observations;
removed nodes and replaced documents cannot reuse an earlier index.

`browser_js` accepts `{ "code": "..." }` in side-browser mode. Its first successful result
includes API documentation; empty code requests the documentation again.
The persistent `tab` binding offers navigation, semantic observations, finding,
clicking, hovering, dragging, forms, keys, scrolling, waiting and screenshots.
Top-level declarations retain JavaScript scope and const/let semantics. Reuse
bindings across calls instead of redeclaring them. Imports, host filesystem
and network APIs are unavailable. State resets on cancellation or disconnection.
One screenshot (the last captured) can accompany each result.
Text results preserve the documentation and the latest output within a 12 KB
UTF-8 budget, with a marker when earlier output is truncated.

```javascript
await tab.goto('https://example.com')
const tree = await tab.getAXState()
```

Prefer `tab.getAXState()` before choosing a target, then use its numeric index
with `tab.click(index)`, `tab.hover(index)`, `tab.setValue(index, value)` or the
other target-taking methods. It emits and returns the tree as text. Subsequent
observations show added, changed and removed nodes. Pass `{disableDiffing: true}`
for a full tree; structural moves and screenshots also force a full observation.
Direct browser actions accept the same index as `ref: "ax:42"`.

`getState()` and `find()` retain the semantic DOM interface with opaque refs
that become stale after a new DOM observation or mutation. AX indices use
Chromium node identities and survive unrelated page mutations. This API
currently has no Playwright locator or native desktop-app surface.

Both interaction paths run inside the same `browser_js` session:

```javascript
await tab.hover(ref)
await tab.scroll('down', 0, ref)
const cdp = tab.cdp
await cdp.send('Input.dispatchMouseEvent', {
  type: 'mouseMoved',
  x: 501.35,
  y: 773.04,
  buttons: 0,
})
```

`tab.hover` and `tab.scroll` animate Jaz's pointer before dispatching browser
input. Scripted scroll distances use CSS pixels (default 800); explicit zero
hovers without scrolling or clicking. A zero-scroll target must be visible.
Scroll distances in this API remain CSS pixels, including when its optional
target is a numeric accessibility index.

`tab.cdp.send` sends a supported command directly to the current webview and
returns Chromium's response. It never requests an overlay move or press.
First-use documentation lists the supported CDP methods. Raw navigation uses
the desktop's URL; use `tab.goto` for server-local preview URLs. Refresh the
page observation after raw mutations before using element refs again.

## Implementation

The authenticated conversation WebSocket carries CDP-shaped requests to the
Electron renderer. Go reuses the existing browser page driver. The renderer
animates an overlay cursor, waits for arrival, then forwards input through an
IPC method restricted to webviews owned by that window. Electron's debugger
dispatches Chromium input. The system pointer stays under the user's control.

Accessibility observations use `Accessibility.enable` and
`Accessibility.getFullAXTree`. The desktop attaches only to iframe debugger
targets belonging to the current webview. Go merges their trees under the
corresponding exposed iframe nodes, flattens ignored ancestors, and omits hidden
frame documents and duplicate inline text boxes. Indices bind to the frame,
document loader and Chromium node identity. Actions resolve those DOM nodes into their frame's isolated world and
reuse the existing browser actions. Frame geometry maps the hit-tested point
into the top viewport; hit tests reject frames covered by another element.
Removing a debugger session also removes its descendants.
Target resolution scrolls instantly before measuring coordinates, so page CSS
for smooth scrolling cannot leave the target moving while the cursor aims.

This uses the same Chromium AX/CDP primitives as Codex. Tree rendering and diff
selection are Jaz code; Codex's private accessibility WASM is not included.
The full tree string is available inside `browser_js`; emitted tool text keeps
the existing 12 KB output budget. Scripts can inspect a specific part of a large
tree and print the relevant lines.

Scripts run in a persistent QuickJS WASM context with memory, CPU and elapsed
time limits. QuickJS evaluates scripts directly with its async-global flag; no
source rewriting is used. The interpreter rejects overlapping scripts; browser
actions also reject overlap rather than queue behind an active action. Await each
action before issuing the next. Bounded promise-job batches enforce deadlines,
and host-action completion yields to the renderer's timers. Failed scripts abort
unfinished actions, and cancellation releases the interpreter even when a host
operation does not respond. High-level calls return through the authenticated
conversation action endpoint, sharing input validation with the MCP tools. Raw
CDP calls go directly to the owned webview's IPC method. Localhost navigation uses the
existing server preview proxy, allowing the server and desktop to be separate.

The Codex 26.903.61454 extraction informed the design: short spring movements,
long curved movements, rotation/stretch, an idle wiggle and arrival before input.
Jaz implements its own cursor asset and animation; pixel/frame equality has not
been established. The extracted service was not bundled: it requires a private
Node host, native-pipe discovery and companion accessibility WASM.

Provider prompts, compaction, models, authentication and native tools remain
provider-owned. Browser documentation arrives through the MCP tool result.

### Codex compatibility and extension boundaries

Codex uses the same Streamable HTTP MCP tools as other ACP agents. The ACP
adapter forwards the conversation header through the provider's advertised HTTP
MCP capability; browser behavior belongs to the shared browser service.
[Codex supports this transport and per-server headers](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
Compatibility means Codex can discover and operate Jaz's tools. The JavaScript
API is Jaz's documented API; Codex's private `cua` runtime and its Node,
Playwright and native-app APIs are separate integrations.

The implementation has these extension points:

| Owner | Responsibility |
| --- | --- |
| `browserApi.ts` | JavaScript bindings, action contracts and first-use documentation |
| `BrowserRepl` | Interpreter lifetime, limits, cancellation and output |
| Go `browsercontrol` | Shared action validation, page operations and AX node identity |
| `SideBrowser` / `BrowserCursor` | Conversation lifetime and cursor-before-input ordering |
| Electron `browserControl` / `BrowserFrames` | Owned webview commands and iframe debugger sessions |

Add browser behavior to the browser API and service, reuse existing page actions,
and extend the Electron command allowlist only when a new primitive is needed.
Provider adapters continue translating MCP configuration. Interpreter changes
do not require changing page actions or the cursor; a future native-app or
Playwright surface should advertise its own supported capabilities.

## Verification

From `frontend`, run `bun run test:browser` with Go 1.26 and Electron installed.
`JAZ_ELECTRON_BINARY` can select an existing Electron executable. The command
builds the production preview/controller/IPC modules into a temporary fixture
and runs the Go test tagged `browserintegration`. It checks cursor-before-input
ordering, direct-CDP hover without overlay movement, zero scroll on a scrollable
page, cancellation (including delayed preview URL resolution), webview ownership, and a
real MCP script through the production conversation HTTP handler and Electron,
including session-header binding, observed success after output truncation, and
an image result. Each Electron process uses a fresh browser profile. The fixture
reports pending commands on timeout and writes a screenshot into the printed
temporary artifact directory.

The same real MCP/HTTP/Electron path verifies AX names from labels, hidden and
password exclusion, stable numeric indices, unchanged/full/diff observations,
closed shadow roots, nested same-site and cross-site frame clicks and form input,
hidden-frame exclusion, obscured-target rejection, wrapped text, smooth-scrolling pages, root-index
scrolling, removed nodes, document replacement and full trees after screenshots.
Unit checks cover ignored ancestors, frame hierarchy and
structural moves. The cursor/input and direct-CDP checks run alongside these.

The same fixture creates synthetic Chrome/Firefox profiles and exercises real
SQLite reads, Chrome decryption/host verification, cookie flags, selected-site
isolation, Keychain denial, unchanged source data and trusted-renderer IPC.
It uses the production import UI to authenticate the side browser with an
imported HttpOnly test cookie, checks dismissal/profile switching/failure retry,
and captures light, dark and narrow layouts. These tests never read the user's
real sign-ins. Actual OS Keychain prompts and Windows/Linux runtime behavior
require platform testing; the fixture runs on the current desktop platform.

The identity check inspects the first navigation and a subsequent fetch on a
local HTTP server, compares their User-Agent with the page and a worker, checks
native client hints/language/driver state, and confirms the app session is
unchanged. It fails against the original Electron/Jaz User-Agent.

`bun test`, `bun run typecheck`, `bun run build:bundle`, and the Go browser,
HTTP browser, settings, app and server suites cover the remaining contracts.
Run `bun run test:browser --codex` for an additional native Codex acceptance
check. It uses the installed `codex` binary and existing ChatGPT OAuth login,
ignores user configuration, removes inherited `OPENAI_API_KEY`, and preserves
the provider's default model. Only the disposable fixture's `browser_js` tool
is pre-approved in that test process; production approval behavior is unchanged.
The check requires successful native MCP calls, a returned image, a completed
turn and an exact final success message, then independently verifies the persistent
JavaScript binding and exactly one trusted button click on the expected page.
The test log records the CLI version; `codex.jsonl` in the temporary fixture
directory retains its native events. This verifies browser MCP compatibility;
full ACP prompt/reload/compaction/auth parity and other live providers need their
separate release checks.

Chromium screenshot capture can stall. High-level screenshots have a five-second
deadline and return an error while preserving the browser connection and script
bindings. A transport regression withholds the screenshot response, then delivers
it late and verifies that the next command still succeeds. The cause of the
intermittent Chromium stall remains unresolved; the deadline bounds its effect.
Native Codex acceptance runs have reproduced the stall after successful AX
observations and trusted clicks. Passing subsequent runs does not clear this
reliability issue. Electron's `capturePage()` also stalled in a diagnostic probe;
it was not retained as an alternative implementation.
