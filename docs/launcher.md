# Global launcher (⌥Space)

The desktop app registers a global shortcut — **Option+Space** — that pops a
floating Spotlight-style launcher over any app. Type a prompt and press Enter to
start a new session; the main window opens on it and streams the reply.

The launcher is its own frameless, transparent, always-on-top window
(`frontend/src/main/spotlight.ts`). It is created lazily and reused (show/hide),
hides on blur or Esc, and renders the `/launcher` route. On submit it behaves as
a full client: it creates the session, uploads any attachments, and fires the
turn — which runs detached server-side — then deep-links the main window to the
session via `openInMain`.

## Screen-region capture

The launcher's **Drag to take a screenshot** button captures a region of the
screen and attaches it to the message. On macOS this shells out to the native
`/usr/sbin/screencapture -i` (the same crosshair as ⌘⇧4: drag a rectangle, or
press Space to grab a window). The PNG is returned to the renderer, attached,
and uploaded with the message. The button is hidden on non-macOS platforms until
a `desktopCapturer`-based path is added.

### macOS Screen Recording permission

Capturing screen content requires the **Screen Recording** permission (macOS
Catalina+). The first capture triggers the system prompt; until it is granted —
and the app relaunched — captures may come back empty or blank. Grant it under
**System Settings → Privacy & Security → Screen Recording**. No extra entitlement
is needed for the `screencapture` CLI route.
