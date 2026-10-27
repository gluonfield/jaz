# Browser and Chat Divider

- [x] Put the drag target in the gap between the browser and chat.
- [x] Verify pointer and keyboard resizing, retained width and minimum pane sizes.
- [x] Review the focused fix.
- [x] Commit and integrate the verified fix locally.

The embedded browser paints above the isolated app chrome. Its resize handle and drag overlay sat below that browser surface. The existing handle now sits in the gutter; its temporary drag overlay belongs to the handle and mounts on the document body so dragging across a native webview continues to resize.

Verification: frontend typecheck and focused ESLint pass. The normal hidden Electron layout smoke passes with Go 1.26.3, covering both drag directions across the browser, keyboard resizing, retained width, minimum chat width, sidebar transitions and browser alignment. Captures were inspected. The fixture now uses the app's isolation and embedded browser; alignment follows the card edges, including its outer margin. Evidence: `runs/browser-chat-divider/` and `runs/browser-smoke-8Mh89C/`.

Regression control: restoring the original handle placement in the compiled fixture fails because the browser covers the drag target. The verified bundle was restored byte-for-byte afterward. Local integration receipt: `runs/browser-chat-divider/integration.json`.

Strict self-review: resize lifecycle owns the temporary overlay and removes it on release, cancellation, lost capture, blur and unmount. Existing transition state and width bounds remain the single source of truth. No new dependency, API or provider boundary change; no unresolved finding.
