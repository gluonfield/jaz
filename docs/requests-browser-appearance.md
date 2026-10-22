# Side Browser Appearance

- [x] Fix the unreadable Haas EC-400 page in the side browser.
- [x] Verify transparent pages and pages with explicit backgrounds in light and dark Jaz themes.
- [x] Run required checks and review the change.
- [x] Commit the verified change.
- [x] Resolve the follow-up review's transparent dark-scheme regression and reverify.

The live page receives `prefers-color-scheme: dark` correctly. Its root and body are transparent, with dark text. Electron's default guest transparency exposes Jaz's dark surface behind it.

The follow-up review found that a white CSS backing also makes transparent dark-scheme pages unreadable. The fix now disables guest transparency in `PREVIEW_WEB_PREFERENCES`, allowing Chromium to paint the canvas from the page's own colour scheme. The React background changes are reverted. The regression uses the existing side-browser fixture and captures the guest page directly, removing the extra React root, runtime mutation, iframe evaluator and shared screenshot API change. It runs last to preserve other tests' initial browser state.

The expanded screenshot-pixel regression covers transparent light pages, explicit dark backgrounds and transparent dark-scheme pages in both app themes. It passes with the native fix and fails when guest transparency is restored. A fresh capture of the real Haas page is readable. Final full backend tests, 361 frontend tests, typecheck, lint, production bundle and full hidden desktop suite pass; no review blockers remain.

Earlier runs exposed voice timing, editor input and clipboard failures; final reruns passed. The guest capture also removes a false pixel failure caused by the parent window's agent cursor. Evidence is in `runs/appearance-review/`.
