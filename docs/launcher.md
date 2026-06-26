# Global launcher (⌥Space)

The desktop app registers a global shortcut — **Option+Space** — that pops a
floating Spotlight-style launcher over any app. Type a prompt and press Enter to
start a new session; the main window opens on it and streams the reply.

The launcher is its own frameless, transparent, always-on-top **full-screen**
overlay (`frontend/src/main/spotlight.ts`) covering the display under the cursor.
A composer bar floats near the top; the rest is a transparent drag surface. It is
created lazily and reused (show/hide), hides on blur, Esc, or a click on the
backdrop, and renders the `/launcher` route. Agent and model are picked with the
same `RuntimeSelect`/`ModelSelect` controls as the main composer (shared through
`useNewThreadControls`). On submit it behaves as a full client: it creates the
session, uploads any attachments, and fires the turn — which runs detached
server-side — then deep-links the main window to the session via `openInMain`.

## Screen-region capture

No button: while the launcher is open you just **drag anywhere outside the bar**
to select a region, which is attached to the message. The renderer draws the
selection box (coordinates in window space, with the window pinned to zoom 1 so
they map 1:1 to screen pixels) and on release sends the rect to the main process,
which hides the overlay and grabs exactly those pixels with the native
`/usr/sbin/screencapture -R<x,y,w,h>`. A click without a drag dismisses the
launcher. macOS only for now; a cross-platform path can replace the capture.

### macOS Screen Recording permission

Capturing screen content requires the **Screen Recording** permission (macOS
Catalina+). The first capture triggers the system prompt; until it is granted —
and the app relaunched — captures may come back empty or blank. Grant it under
**System Settings → Privacy & Security → Screen Recording**. No extra entitlement
is needed for the `screencapture` CLI route.
