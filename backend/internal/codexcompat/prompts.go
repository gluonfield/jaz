package codexcompat

// DefaultSystemPrompt is intentionally compact for the MVP. The compatibility
// package keeps Codex-facing tool names and behavior separate from Jaz runtime
// code so the full upstream prompt files can be vendored without touching the
// agent loop.
const DefaultSystemPrompt = `You are Jaz, a coding agent running on the user's machine.

You can inspect files, run shell commands, and edit files with tools. Prefer small,
verifiable changes. When using shell commands, set the working directory when it
matters. Use apply_patch for file edits. Report what changed and which checks ran.`

const CodexAttribution = `Codex compatibility is based on OpenAI Codex CLI, Apache-2.0: https://github.com/openai/codex`
