# Jaz side browser

Enable browser tools in **Settings → Browser** and select **Jaz side browser**.
Keep the desktop app running while the agent works. Once a conversation has
connected, browser activity in the current conversation opens the right side
panel and selects the agent's browser tab. This includes commands against an
existing page and commands continuing inside a script after the panel is hidden.
Commands wait for the desktop panel's opening animation before observing or acting.
Status and tab-list checks leave the panel alone. Other conversations keep working
in the background without changing the current chat's panel. Active conversations
retain their own page, viewport and JavaScript bindings.
An agent can also open its first page after you leave that conversation.
Browser commands preserve the focused chat control. Hidden panels disable pointer
interaction and hide the agent cursor while their browser work continues.
The shared Jaz MCP tools expose this browser to any ACP provider that supports
those tools. Cancellation, disconnection, changing backend, or closing the app
stops browser work. Other conversations' tabs cannot be claimed.
Pages and panels outlive a backend outage, including one long enough to show
the launch screen, and reconnect when the backend returns. Changing backend or
closing the app clears them.

With **This machine** selected (a loopback backend on port 5299), localhost
previews load directly and keep their original URLs, including paths, queries
and fragments. **Open in Browser** opens that same URL. Remote backends, including
loopback tunnels on other ports, use the server preview proxy.
Ordinary app links and new-tab browser links open in side-browser tabs.
JavaScript popup windows retain their shared browser session and `window.open`
callbacks. **Open in Browser** explicitly uses the system browser.
Only main-frame navigation updates a tab's URL; embedded frames cannot replace
the page by changing their own URL or fragment.

Hidden browser sessions are eligible for unloading after five minutes without
browser commands, including JavaScript sessions that have not opened a page.
Cleanup checks the conversation's current status and keeps sessions for
running agents, queued work and pending commands. Unknown or unavailable status
keeps the page alive until a later check. Status requests time out after ten
seconds and are cancelled when new work or visibility makes them obsolete.
Visible pages remain loaded.
Unloading releases the script context and any webview, preserving only the URL,
panel dimensions and lightweight control connection. The next command or opening
Preview reloads that URL. Empty chats create their browser surface on first use.
Page-local state and script variables reset; imported sign-ins remain in the
shared browser partition.

Opening Preview hides the left navigation and gives the browser about 60% of
the available content area, with an 800-pixel default minimum when space permits. The conversation
keeps at least 360 pixels on desktop. The visible grip on the panel's left edge
can be dragged in either direction or clicked and adjusted with the arrow keys.
Each panel view retains its own resized width while the conversation is mounted.
Reopening or resizing navigation reduces the browser width to fit the remaining
content area; closing navigation restores the preferred browser width. CSS sizes
the desktop browser in the same layout pass as navigation, keeping their edges
synchronized throughout opening, closing and interrupted animations.
Opening, closing and switching desktop panels use the same spring curve through
a CSS width transition. Drag resizing and Reduced Motion remain immediate.

## Downloads

Use **Browser menu (⋮) → Downloads** for recent downloads from all side-browser
tabs on this computer. The list shows transfer progress, the actual saved filename
and location, and **Open** and **Show in folder** for completed files. **Cancel**
stops an active download; the folder button opens the system Downloads directory.

Downloads keep Electron's native Save dialog. Jaz records the location you choose,
including renamed files. The latest 50 finished downloads and all active downloads
are kept in `browser-downloads.json` in the desktop's app data directory. History
survives restarting Jaz; unfinished transfers show as interrupted after restart.
The history starts when this feature is installed. Existing files remain accessible
through the Downloads folder. Downloads and their history stay on the desktop,
including when the Jaz backend runs remotely.

## Saved passwords

Submitting a top-level HTTPS login form offers **Save password?** or
**Update password?** in the browser toolbar. Saving requires clicking **Save**
or **Update**. **Not now** dismisses the proposal. An unchanged saved login
does not prompt again.

Use **Browser menu → Passwords** to fill a saved login or delete it.
Filling requires selecting an account and matches the exact HTTPS origin,
including the port. **Save login on this page** can capture filled login fields
on sites that do not submit a standard HTML form. Embedded login frames and
passkeys are outside this password feature.

Capture retains hidden, read-only and disabled usernames used by email-first sign-in flows.
Filling selects a login field even when a sign-up form appears earlier on the page.
A hidden, read-only or disabled username must match the selected saved account before its
password can be filled.

The desktop stores passwords in `browser-passwords.enc` under Electron's app
data directory, encrypted with Electron `safeStorage` and the operating system's
key storage. Passwords are shared by the side-browser tabs on that computer.
Saving is unavailable without OS encryption; Linux's `basic_text` fallback is
rejected. A failed write preserves the previous file.

The isolated browser preload sends submitted credentials directly to Electron's
main process. The app toolbar receives only origin, username and prompt metadata.
The Jaz server and browser MCP tools have no password-store API. Autofill writes
the selected credential into the matching page's login fields.

## Import cookies and passwords

Open **Browser menu → Import cookies and passwords**, or the same action in
**Settings → Browser → Jaz side browser**. The dialog selects a browser profile
and lets the user choose **Saved passwords** and **Cookies** independently.
There is no permanent import banner. Close the source browser before importing.

Chrome and Edge support cookies and saved passwords on macOS. Firefox supports
cookies on macOS, Windows and Linux; its password toggle is unavailable.
Passwords are read from the profile and account login databases, restricted to
HTTPS form logins, and imported into the encrypted desktop vault. Existing Jaz
passwords are preserved. Duplicate source logins use the newest password timestamp.
The menu and import results contain counts and account metadata, never passwords.

Cookie import copies the selected profile's current cookies into the shared
preview partition. Partitioned cookies and Firefox container cookies are excluded
because Electron's cookie setter cannot preserve their isolation. Session cookies
keep their original lifetime; some sites may still ask the user to sign in.
Chrome/Edge encryption uses that browser's Safe Storage key from macOS Keychain.
Source databases are read-only. The import adds no database dependency.

Import results report successes and failures separately for each selected data
type. Unsupported or damaged records are counted as failures; a failed type can
be retried without replacing existing passwords. This is a one-time copy, with
no ongoing synchronization, history, bookmarks, or extension import. Agents can
subsequently use the signed-in side browser and observe its pages.

### Google sign-in

Google can reject the side browser with “This browser or app may not be secure.”
Its [supported-browser policy](https://support.google.com/accounts/answer/7675428)
restricts embedded and automated browsers. Chromium compatibility and saved
passwords do not establish support for signing in to Google inside Electron.

Sign in to the destination site in Chrome first, then use **Import cookies and passwords**
with that Chrome profile and **Cookies** enabled. Open the destination URL again in Jaz, such as
`https://console.firebase.google.com/`; reloading Google's `/signin/rejected`
page can leave the rejection screen displayed. This attempts to reuse the
existing session; Google may still require authentication in a supported browser.
Signing in externally alone does not update Jaz's separate cookie partition.

### LinkedIn

LinkedIn's public homepage can return a Cloudflare block even while its sign-in
page works. When a top-level GET of that homepage returns HTTP 403, Jaz redirects
the same tab to LinkedIn's `/login` page. Sign in there to enter the feed.
Successful homepage responses, other paths and background requests keep their
original behavior.

## Browser identity

Before creating a side-browser tab, Jaz configures that preview session's native
User-Agent to omit the Jaz and Electron product tokens. The actual Chromium
version and platform remain intact and follow the installed Electron version.
The app's own session keeps its identity. Chromium continues to generate client
hints, language, fetch metadata, cookies and other request headers itself; page
and worker JavaScript observe the same User-Agent as network requests.
This uses [Electron's session API](https://www.electronjs.org/docs/latest/api/session#sessetuseragentuseragent-acceptlanguages)
without per-request identity rewriting or browser-property patches.

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
server preview proxy when connected to a remote backend.

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
| `BrowserRetention` | Idle deadline and protection against concurrent work or stale cleanup decisions |
| Go `browsercontrol` | Shared action validation, page operations and AX node identity |
| `BrowserSessions` / `BrowserWorkspace` | App-owned conversation browsers and their visible panel slots |
| `SideBrowser` / `BrowserCursor` | Browser connection lifetime and cursor-before-input ordering |
| Electron `browserControl` / `BrowserFrames` | Owned webview commands and iframe debugger sessions |

Add browser behavior to the browser API and service, reuse existing page actions,
and extend the Electron command allowlist only when a new primitive is needed.
Provider adapters continue translating MCP configuration. Interpreter changes
do not require changing page actions or the cursor; a future native-app or
Playwright surface should advertise its own supported capabilities.

## Verification

Run `go test ./...` from `backend`, and `bun test` and `bun run typecheck`
from `frontend`.

From `frontend`, `bun run test:browser` exercises the MCP/HTTP/Electron path
with disposable browser profiles. It opens desktop test windows and requires
Go 1.26, Electron and OpenSSL. `JAZ_ELECTRON_BINARY` selects an existing
Electron executable. Add `--codex` for native acceptance using the installed
Codex CLI and existing ChatGPT OAuth login.
