# Side Browser Appearance

- [x] Fix the unreadable Haas EC-400 page in the side browser.
- [x] Verify transparent pages and pages with explicit backgrounds in light and dark Jaz themes.
- [x] Run required checks and review the change.
- [x] Commit the verified change.

The live page receives `prefers-color-scheme: dark` correctly. Its root and body are transparent, with dark text. PreviewPanel gives the embedded page Jaz's `bg-bg`, which turns that transparent canvas dark.

Both embedded page elements now use a white backing surface. The app chrome retains its theme, and explicit page backgrounds retain their colours. No theme propagation or page CSS overrides were added.

Verification: the screenshot-pixel regression fails on the original code (RGB 20/21/23 instead of white) and passes after the fix across webview/iframe, dark/light app theme and transparent/explicit dark page backgrounds. The full hidden desktop browser suite, full Go 1.26 backend suite, 361 frontend tests, typecheck, lint and production bundle pass. The strict code-quality review found no remaining blockers. A screen audit recorded 2,877 samples with no test windows, system dialogs or test foreground activity.
