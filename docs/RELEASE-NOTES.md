Changes since v0.0.298:

- Unifies connected services and installed packages in Plugins, with a Git-backed catalog and clear Connect/Disconnect or Install/Remove actions.
- Improves connection cleanup, failed-install rollback, catalog refresh and multi-account support. Connected plugins use compact rows with a Show all control.
- Includes delivered bot replies exactly once in thread read-back, in order, while excluding private handoffs.
- Preserves original skill paths when a secondary native plugin manifest cannot be loaded.

Upgrade notes:

- Reconnect Tasks, CRM, Granola and Zoom once in Plugins after updating; they now come from the official Git marketplace. Existing native accounts and installed plugin folders are preserved.
- The Deployink integration is removed.
