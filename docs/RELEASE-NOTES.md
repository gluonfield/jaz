Changes since v0.0.283:

- Word files (`.docx`) open in the side panel reader as they are laid out, read-only. A link to quoted words opens the document at those words, highlighted.
- Disconnecting an Antigravity account finishes promptly: sign-out completes before the settings refresh, and the account's processes stay supervised.
- Zoom's production sign-in is relayed through the HTTPS callback.
- An agent's modes, models and commands are kept as one row per thread and replaced on change. Before, every update added a full copy; the older copies are removed in the background.
