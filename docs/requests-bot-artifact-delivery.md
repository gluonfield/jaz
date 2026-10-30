# Bot artifact delivery

Bot chat now renders saved artifact events using the existing sandboxed `ArtifactBlock`. Previously the chat projection discarded these events and both history queries omitted them, while the tool told the agent the content had already been shown.

## Delivery contract

- History retains artifact events alongside the existing chat surfaces. Chat entries keep the source session, sequence and payload; replay uses the existing event coalescer.
- New artifact events record host acceptance for presentation. This marks turn delivery atomically with the saved event and keeps it visible on pages without turn context. It is not a client rendering acknowledgement.
- Historical events without that marker use the existing public user/routine turn context. No history migration or live transcript rewrite is needed.
- Artifact tools reuse the same session visibility rule as app presentations. Turns answering a peer bot or a group do not present into the user's bot chat.
- The MCP tool reports a saved artifact only after persistence succeeds. Missing sessions and failed writes cannot return success. The native tool reports preparation only. Neither claims the user has seen the result.
- Artifact prompt interactions return through the bot chat's existing send function.

## Child handoff using existing APIs

Creation in a child conversation does not present anything in its parent's chat. A saved event is addressed internally by its source session and sequence; there is no artifact registry or tool that presents an existing event by reference. `read_thread` exposes text and tool-name summaries, not artifact payloads. Full transcript HTTP history exposes the saved event payload.

The smallest supported handoff is a saved backend HTML/SVG file plus its source thread, path, title and short summary in the child's normal completion report. The parent can inspect the file through its filesystem tools or the existing session-file HTTP API, then call `visualise_show_widget` in the destination conversation. That tool currently requires `widget_code`; a filename or event reference alone does not render. This keeps raw code out of inter-agent report text, although the final presentation call still supplies it.

Use the existing file reader/preview proxy for a user-facing file link. A backend path is an agent-side reference, not an assumption that the client computer has that file. File references are mutable; include a content hash when a specific version matters. Keep the source thread/path or original event identity in the handoff report: the current presentation schema has no structured source-reference field.

This change adds no cross-conversation transport, gallery, artifact registry, new renderer or dependency.

## Verification

Go 1.26 tests exercise real SQLite persistence, public/private presentation, failed-save behavior, delivery accounting and HTTP history. Frontend tests cover historical events and standalone presentation pages. The normal hidden Electron history suite runs in explicit development mode, asserts StrictMode double mounting, and checks live delivery, replay without duplication, reload, the sandbox prompt bridge, visible bounds and rendering errors. A private replay of the original saved map uses unchanged source content through the production chat projection and renderer.

An external OS sampler verifies the hidden run does not show test windows or take foreground focus. Private trace, screenshots, hashes, check logs and the strict code-quality review are under ignored `runs/bot-artifact-delivery/`.
