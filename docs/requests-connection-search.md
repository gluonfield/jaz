# Connection Search in Cmd+K

- [x] Jaz Tasks exposes a read-only `search` tool following OpenAI's MCP search convention; committed and pushed as `5a4f536`.
- [x] Jaz CRM exposes a read-only `search` tool across every object; committed and pushed as `eeaea7d`.
- [x] Jaz searches every connected server with such a tool and returns typed results per connection.
- [x] Cmd+K shows one section per connection; choosing a result opens its URL in the side browser.
- [x] Run a thermo-nuclear code quality review and fix its findings.
- [x] Commit the verified Jaz change.
- [x] Open a chosen result in its connection's sidebar app at that item, not in a browser.
- [x] Rank CRM text matches by name first; pushed as CRM `2b9c0f6`.

Contract: `search({query})` returns `{results: [{id, title, url, text?}]}` as structured content ([OpenAI MCP docs](https://developers.openai.com/api/docs/mcp)). Jaz calls only tools named `search` that declare `readOnlyHint`, because it calls them on every debounced keystroke. `GET /v1/search/connections?q=` asks all such servers at once with a 5 s deadline and drops untitled results and non-web links. Sections arrive together, between Threads and Archived; a failing or slow server is left out and logged.

Tasks returns up to 10 issues (title; identifier, state and assignee as text). CRM returns 3 matches of each object, name matches first (record name; object name as text).

Verification: Go suites pass in all three repos; Jaz frontend 315 tests, typecheck and lint pass. The Jaz manager test runs real MCP servers and fails with the read-only guard or the link filter removed. End to end, this branch's backend in a scratch home searched a locally run, seeded Tasks server, and headless Chrome drove the real palette: the Tasks section rendered, arrow keys and Enter opened the issue URL, and an unmatched query showed "No matches". CRM was verified through its MCP test, not run locally.

Review: three thermo-nuclear passes, all by the implementing agent; an independent reviewer was interrupted twice and never reported. Fixes: CRM search builds results in one loop and always returns an array; failure logging skips requests the palette cancelled, judged from Jaz's own context. Declined: per-source progressive loading, which costs a sources endpoint and N requests for little gain with two fast servers; a shared cross-object query in CRM's records layer, since its MCP handlers already orchestrate service calls this way.

Opening in Jaz: each Tasks and CRM result carries OpenAI's MCP extensions preview target, `_meta["openai/preview"].target = {type: "mcp_app_tool", name, arguments}`: `show_tasks {view: "EE-1"}` or `show_crm {path: "/r/<id>"}`. Jaz keeps a target only when it names one of that server's sidebar (global entrypoint) apps and always sets the `app` field itself, so a server cannot supply it directly. Enter switches the left nav to that app and sends the target as a new call of its tool; the Tasks and CRM apps navigate on that input. Results without an app target still open their link. Verified end to end: Enter on a Tasks result opened the issue in the Tasks sidebar app, a second selection moved the open app to the next issue, and no browser tab opened. The Jaz test fails when the sidebar-app check or the host-owned `app` field is removed.

CRM ranking: text searches ordered matches newest first, so "Jim Mayer" returned three newer people whose notes mention him and cut off his own record. CRM's record query now ranks where the text matched: the name, then an email address or domain, then anything else, newest first within each. One lateral match computes both the filter and the rank. This also changes `search_records` with a text query and CRM's own palette. A records test fails with the ranking removed; live, "Jim Mayer" now returns his record first.
