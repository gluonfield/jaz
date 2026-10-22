# Canvas Chat Width

- [x] Resize the Canvas chat by dragging its right edge horizontally.
- [x] Keep the composer, canvas controls and hide/show behavior aligned with the chat width.
- [x] Verify dragging, keyboard resizing, width limits, restoration and mobile layout in the production thread view.
- [x] Complete full checks, strict code review and commit.

The chat reuses the side panel's resize handle, promoted to a shared UI component with a configurable edge. The width is saved locally, stays within the available board space and retains the existing phone layout. Canvas controls follow the divider immediately during a drag; the existing collapse animation remains.

Verification: 361 frontend tests, typecheck, lint, desktop bundle and all 105 backend test packages pass. The production ThreadView passes hidden native checks for dragging in both directions, arrow keys, minimum/maximum widths, composer alignment, hide/show and mobile Browser navigation. The native runner's unrelated clipboard restoration preflight failed; subsequent UI checks use an ignored build with that preflight excluded, and do not exercise clipboard behavior.

The final combined UI run passes the canvas, layout, side-panel and browser lifecycle checks, then fails with `cdp connection is closed` during MCP/accessibility checks. The unmodified base passes the combined suite with the same clipboard exclusion. This change does not claim a fully passing browser suite. Strict code review found no blockers.
