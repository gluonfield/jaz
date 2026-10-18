import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'

// The editor of a bot's AGENTS.md and MEMORY.md reads and writes Markdown.
// Underline has no Markdown form, so it stays off; links open only on purpose.
// A trailing empty line would be an edit the person never made.
export const markdownKit = [StarterKit.configure({ underline: false, trailingNode: false, link: { openOnClick: false } }), Markdown]
