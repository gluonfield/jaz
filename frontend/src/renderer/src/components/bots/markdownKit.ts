import type { Editor } from '@tiptap/core'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'
import { carryEdits } from '@/lib/diff/lineDiff'

// The editor of a bot's AGENTS.md and MEMORY.md reads and writes Markdown.
// Underline has no Markdown form, so it stays off; links open only on purpose.
// A trailing empty line would be an edit the person never made.
export const markdownKit = [StarterKit.configure({ underline: false, trailingNode: false, link: { openOnClick: false } }), Markdown]

// savedMarkdown is the text that saves the editor's document over source, the
// file it was opened from. The editor writes its own Markdown (escapes,
// entities, spacing), so only the lines the person edited take it; every other
// line keeps source's text. Markdown lines can change meaning beside an edit
// (a blank line source leaves out), so a merge must reopen to the document the
// editor's Markdown represents: failing that, the edits also take their neighbouring lines, and
// failing that too, the whole file is the editor's Markdown.
export function savedMarkdown(editor: Editor, source: string): string {
  const manager = editor.markdown!
  const open = (text: string) => editor.schema.nodeFromJSON(manager.parse(text))
  const render = (text: string) => manager.serialize(open(text).toJSON())
  const base = render(source)
  const markdown = editor.getMarkdown()
  // The serializer can normalize structures such as line breaks in headings.
  const target = open(markdown)
  for (const margin of [0, 2]) {
    const merged = carryEdits(base, markdown, source, render, margin)
    if (open(merged).eq(target)) {
      return merged
    }
  }
  return markdown
}
