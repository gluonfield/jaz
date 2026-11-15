# Proposal: plugins and app views in Jaz

Keep the rendering foundation. Jaz already hosts standard app views in the chat, the sidebar and the side panel. Four things hold it back: it does not fully follow the app standard, views cannot talk to the conversation, one Jaz-only display rule hides third-party views, and the plugin catalogue is compiled into the Go binary.

## Where a form breaks today

A form can render and save, but the thread never hears about it, so the agent cannot continue.

~~~jaz-artifact A form's round trip in Jaz
<svg viewBox="0 0 680 236" role="img" xmlns="http://www.w3.org/2000/svg">
<title>Where a form's round trip breaks in Jaz today</title>
<desc>An agent's tool call renders a card and the user's submit reaches the plugin server, but nothing returns to the thread, so the agent never continues.</desc>
<defs><marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker></defs>
<g class="c-gray"><rect x="30" y="30" width="190" height="56" rx="4" stroke-width="0.5"/><text class="th" x="125" y="54" text-anchor="middle">Agent calls a tool</text><text class="ts" x="125" y="72" text-anchor="middle">tool declares a view</text></g>
<g class="c-gray"><rect x="245" y="30" width="190" height="56" rx="4" stroke-width="0.5"/><text class="th" x="340" y="54" text-anchor="middle">Card renders in chat</text><text class="ts" x="340" y="72" text-anchor="middle">sandboxed frame</text></g>
<g class="c-gray"><rect x="460" y="30" width="190" height="56" rx="4" stroke-width="0.5"/><text class="th" x="555" y="54" text-anchor="middle">User submits form</text><text class="ts" x="555" y="72" text-anchor="middle">view calls its server</text></g>
<line class="arr" x1="222" y1="58" x2="243" y2="58" marker-end="url(#arrow)"/>
<line class="arr" x1="437" y1="58" x2="458" y2="58" marker-end="url(#arrow)"/>
<g class="c-amber"><rect x="460" y="140" width="190" height="56" rx="4" stroke-width="0.5" stroke-dasharray="4 3"/><text class="th" x="555" y="164" text-anchor="middle">Thread gets result</text><text class="ts" x="555" y="182" text-anchor="middle">nothing sent back</text></g>
<g class="c-amber"><rect x="245" y="140" width="190" height="56" rx="4" stroke-width="0.5" stroke-dasharray="4 3"/><text class="th" x="340" y="164" text-anchor="middle">Agent continues</text><text class="ts" x="340" y="182" text-anchor="middle">next turn never starts</text></g>
<line class="arr" x1="555" y1="88" x2="555" y2="138" stroke-dasharray="4 3" marker-end="url(#arrow)"/>
<line class="arr" x1="458" y1="168" x2="437" y2="168" stroke-dasharray="4 3" marker-end="url(#arrow)"/>
<path class="arr" d="M243 168 L125 168 L125 90" fill="none" stroke-dasharray="4 3" marker-end="url(#arrow)"/>
<rect class="c-gray" x="30" y="210" width="12" height="12" rx="2" stroke-width="0.5"/><text class="ts" x="48" y="220">Works today</text>
<rect class="c-amber" x="150" y="210" width="12" height="12" rx="2" stroke-width="0.5" stroke-dasharray="3 2"/><text class="ts" x="168" y="220">Missing today</text>
</svg>
~~~

## Proposed changes

Sizes are estimates.

### 1. Follow the app standard (done)

- [x] **Tell servers Jaz can show views.** The standard requires the feature to be negotiated, and its server kit registers a text-only tool when the client does not declare it ([spec](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)). Jaz creates all three of its clients with no options, so it never declares it: [`mcp.go:524`](../backend/internal/mcp/mcp.go), [`mcp.go:565`](../backend/internal/mcp/mcp.go), [`stdio.go:23`](../backend/internal/mcp/stdio.go). Servers that follow the standard give Jaz no views. One shared options value fixes it.
- [x] **Enforce each view's network policy.** The standard says the host must build the policy from the domains a view declares and block everything when none are declared ([spec](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)). `ReadApp` returns only the HTML and drops that declaration ([`apps.go:153`](../backend/internal/mcp/apps.go)), and the frame loads the HTML as is ([`MCPAppFrame.tsx:197`](../frontend/src/renderer/src/components/apps/MCPAppFrame.tsx)). Jaz's own generated artifacts are locked down ([`artifacts.ts:16`](../frontend/src/renderer/src/lib/artifacts.ts)); third-party view code is not. CRM already declares "no network connections" and Jaz ignores it.

### 2. Connect views to the conversation (done)

- [x] **Let a view post into its thread.** The standard's `ui/message` request sends a user message and starts a turn ([spec](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)). Jaz advertises only links, tool calls and logging ([`MCPAppFrame.tsx:165`](../frontend/src/renderer/src/components/apps/MCPAppFrame.tsx)). Artifacts in the same transcript row can already send a prompt ([`LiveEvent.tsx:59`](../frontend/src/renderer/src/components/session/LiveEvent.tsx)); app views should use that same path, with the existing "only the view in use" focus check.
- [x] **Let a view tell the model what the user did.** `ui/update-model-context` carries a selection or draft to the model with the next message, without starting a turn ([spec](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)). Send it as its own typed block on the next turn, never by rewriting the prompt text.
- [x] **Tell panel and inline views which thread they are in.** A panel view gets no thread and no size ([`SidePanel.tsx:107`](../frontend/src/renderer/src/components/session/SidePanel.tsx), [`mcpAppHost.ts:24`](../frontend/src/renderer/src/lib/mcpAppHost.ts)). Add the thread and the container size to the host context. Reporting a panel as "fullscreen" is fine: the standard has only inline, fullscreen and picture-in-picture.
- [x] **Let a card expand.** Answer `ui/request-display-mode` by opening the same call in the side panel, so a long form is not squeezed into the chat column.

### 3. Drop the Jaz-only display rule (done)

- [x] The standard shows a view whenever a tool that declares one is called ([spec](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)). Jaz shows it only for sidebar tools, or when the result also links the view ([`apps.go:233`](../backend/internal/mcp/apps.go)). No third-party server knows that rule, so their inline views stay hidden. The rule exists because CRM's lookup tools declare views and agents call them in batches. Tasks already solved this on its side: lookups declare nothing and `show_issues` declares the view. Moving CRM's `search_records` and `get_record` to that pattern lets Jaz delete the link check. The `presented` flag and its transcript filter ([`mcpApps.ts`](../frontend/src/renderer/src/lib/mcpApps.ts)) stay, because threads stored before this change hold hundreds of lookup events that must stay hidden.

### 4. Make plugins data instead of Go (medium)

- [ ] **Move link-only plugins out of the binary.** The catalogue is a Go list of ten plugins ([`catalog.go:23`](../backend/internal/connections/catalog.go)). Granola, Deployink, Tasks and CRM are only a name, a description and a server address ([`jazapps/plugin.go`](../backend/internal/connectors/jazapps/plugin.go)). Skills already install from a remote list with pinned checksums ([`sync.go`](../backend/internal/skills/sync.go)); plugins can ride the same mechanism. Gmail, Calendar, Drive, Slack, Telegram and WhatsApp stay in Go because they sync data in the background.
- [ ] **Ship plugins with a Jaz build.** Plugins are installable folders of skills and views today, using the formats in the [author guide](plugins.md). Bundling them with a build remains future work; see the [tender-queries proposal](proposal-tender-queries-plugin.md#installing-a-plugin).
- [ ] **Write the contributor page.** `docs/` holds request logs but no page that says what a plugin can declare and what Jaz gives a view. Write it once items 1 to 3 land.

## Not proposed

- A new rendering design. The frame, bridge and three placements are sound.
- A saved-state store for cards. The standard has none; a submitted form can reload its record from the server by the ID in the tool result.
- Picture-in-picture, streamed partial input, and plugin hooks.

## Decisions needed

- [x] Adopt the standard display rule and change CRM's two lookup tools (item 3): yes.
- [x] When a sidebar page sends a message, it starts a new thread. The Jaz-only `jaz/threads/create` is removed; Tasks now sends the standard message.
- [ ] Catalogue source: one Jaz-owned Git repository, or also third-party ones a user adds?
- [ ] May a catalogue plugin install a local-process server, which runs code on the Jaz machine, or remote servers only?
