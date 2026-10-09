# Random Text in the Composer

- [x] Find where unexplained composer text comes from.
- [x] Reproduce it in the hidden browser smoke suite.
- [x] Keep agent keyboard input inside the side-browser page; keep the composer's focus and draft intact.
- [x] Run full checks and strict review; commit.

Cause: the draft history in the app's local storage showed this thread's composer receiving `Can you check the Power CNC quote?Here is a longer message…`. These were two strings an agent sent to the side browser with CDP `Input.insertText`. Jaz keeps the composer focused while an agent drives the side browser. Chromium delivers `Input.insertText` and `Input.dispatchKeyEvent` for a webview to the frame focused in the outer window, so agent typing landed in the composer. Agent key presses (`browser_key`, `tab.pressKey`) took the same route, so Backspace could delete draft text and Enter could reach the composer.

Fix: `SideBrowser.command` focuses the webview before keyboard commands and refuses to send them if it cannot take focus. Its existing cleanup returns focus to the composer.

Verification: a new browser smoke stage focuses a composer-like textarea holding `my draft`, then has the agent type `agent` and press Backspace in the page. Before the fix it failed with `page "", composer "my draftagen"`; after the fix the page reads `agen` and the composer keeps its draft and focus. The full hidden Electron smoke suite passed twice with the fix. Two other runs failed earlier, at the bot chat history stage, which this branch does not touch. 311 frontend tests, typecheck and lint pass.
