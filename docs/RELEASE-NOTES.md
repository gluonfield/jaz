Changes since v0.0.284:

- A message sent to Codex while it works is delivered after a Codex subagent has started. Before, it failed with "native ACP steering failed" and was dropped. Codex ACP moves to 2.8.0 with Codex 0.170.0.
- A tool connected while a Codex thread is open reaches that thread on its next turn: the idle Codex process is restarted and the thread resumed.
- A thread can be set to defer MCP tool discovery, through the API.
- Every control's hint is one app tooltip that appears after 150 ms, with its keyboard shortcut beside the label. While a turn runs, the send menu reads Send now and Queue, and holding Command turns the send button into Queue. Voice has its own button beside stop.
- A file's path can be copied from its menu, and menu actions that are not available are hidden.
- Child threads stay grouped in the sidebar's recents.
- A view's frame shrinks with its content. Before, it could grow but not shrink.
- Zoom result cards open scheduled meetings, and join links are kept when meeting details are missing.
- An open spreadsheet reloads when the agent rewrites it.
- Retrying a connection's sign-in replaces the abandoned attempt.
