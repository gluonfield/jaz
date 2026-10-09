# Agent Browsing Without the Side Panel

- [x] Find why agents still use the side browser without it opening in the right side panel.
- [x] Reproduce it in the real app against a real backend.
- [x] Keep every conversation's browser and panel through backend outages.
- [x] Add a regression check to the normal smoke suite; run full checks and strict review; commit.

Cause: the desktop swaps the whole app for the launch screen after 30 seconds without a backend. Restarting a `go run` backend often takes longer than that. `BrowserWorkspace` and the side panel state lived under that swap, so each restart destroyed every conversation's browser page, its connection and its panel. Afterwards only the conversation on screen got a browser again, and it was empty. The agent's next command opened the panel on a blank tab and failed with "Side browser is closed". Background agents failed with "open this conversation in the Jaz desktop app" until that chat was reopened. On 9 Oct, agent sessions restarted together at 13:04, 13:09, 13:14 and 13:20. The Website Discussion Review agent failed six browser calls after them; it reconnected only when the chat was reopened. Across the week, 28 calls in 11 conversations failed this way.

Fix: `App` (moved out of `main.tsx`) mounts `BrowserWorkspace` and `SidePanelStateProvider` above the connection gate. Pages, panels and browser connections now survive the launch screen and reconnect when the backend returns. Changing backend still clears them.

Verification: a temporary rig ran the real app routes against an isolated real backend and killed the backend for 60 seconds. Before the fix, the visible chat's browser and tabs disappeared. The agent's next command opened an empty tab and failed, and the background bot's browser was gone. After the fix, both kept the same pages and connections, and the next command succeeded. The side-panel smoke suite now renders through the production `App` and drops and restores the connection. That check fails when the providers sit under the router again, and passes with the fix. Typecheck, lint and 316 frontend tests pass. The full hidden Electron smoke suite passed 5 of 8 runs with the fix and 5 of 5 on the unchanged base. All three failures came at load average about 137, each in a different stage. Interleaved under the same load, the base and the fix both passed 3 of 3.

Not fixed here:
- In development, StrictMode opens a conversation's browser socket twice. The backend keeps the first socket and rejects the second, then the first closes, so for about 2 seconds after a chat first opens, browser calls fail.
- A second Jaz desktop on the same backend owns any conversation it connected first. On 9 Oct an agent-launched dev desktop (`localhost:5180`, profile `Electron`) ran against the live backend. While such a desktop holds a conversation, browsing succeeds in that window and never shows in yours.
