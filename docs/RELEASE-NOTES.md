Changes since v0.0.288:

- Pins Antigravity to 1.5.14 and stops the agy CLI from updating Jaz's managed copy past its pin.
- Skips Codex's predicted next-prompt model call in bot workers, group seats, routine runs and memory jobs, where nobody types; user threads and bot chats keep suggestions.
