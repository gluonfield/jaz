# Drag-to-Allow Permission Guide

- [x] Open System Settings at the exact privacy list with a Jaz panel docked under the window, so the user drags Jaz into the list instead of navigating there.
- [x] Cover both Computer Use permissions: Accessibility and Screen Recording.
- [x] Run regression checks, full verification and strict code review.
- [x] Commit the verified change.
- [x] Live check on macOS 26: Allow opens the Accessibility list and the panel docks under the window (Augustinas's screenshot).
- [x] Redesign the panel to be unmistakable: a draggable copy of the row Jaz gets in the list (icon, name, toggle), aligned under the list, with an arrow from the row into the window and a ghost of the icon rising along it.
- [ ] Live check: the drag adds Jaz to the list and the panel closes on grant.

Design: Computer Use settings shows an Allow button for each missing permission. Allow opens the `Privacy_Accessibility` or `Privacy_ScreenCapture` pane and a non-activating panel window. An `osascript` JXA loop reads System Settings' window bounds from the window server, which needs no permission, and the panel docks under the window, showing only while Settings is frontmost. Dragging the panel's row starts a native file drag of the app bundle (`webContents.startDrag`). The panel closes when the permission is granted, when Settings closes, on its close button, or after 10s if Settings never appears. The old system-prompt path (`requestMacOsPermissions`) and the separate Screen Recording settings button are removed.

Verification: 312 frontend tests, typecheck, lint and the Electron bundle build pass. The IPC test fails if permission validation is removed. The tracker reported a live window's bounds, printed `none` for a missing app, and exited when its parent died; polling costs about 3% CPU only while the panel is open. Hidden offscreen Electron captures of the real panel window rendered correctly in light and dark themes.

Redesign: the panel window starts at the Settings window's bottom edge and spans the list's width (measured on macOS 26: 243pt from the window's left edge, 20pt from its right). Its row uses the system font, a macOS-blue toggle and white-on-gray rows in light mode; the arrow runs from the row's icon to the window edge, and the ghost loop pauses while the pointer is on the row. Hidden offscreen captures composited under the user's Settings screenshot checked alignment, both themes and the animation frames.

Not yet verified: drag start from the non-focusable panel, the drop adding Jaz, and whether Screen Recording reports granted before a relaunch. Each needs a person at the machine.
