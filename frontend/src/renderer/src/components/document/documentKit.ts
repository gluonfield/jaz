import { Extension, InputRule, Node, type Editor, type JSONContent, type MarkdownToken, type MarkdownTokenizer, type NodeViewRenderer } from '@tiptap/core'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import { Table, TableKit, renderTableToMarkdown } from '@tiptap/extension-table'
import { Placeholder } from '@tiptap/extensions'
import { Plugin } from '@tiptap/pm/state'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'

const INLINE_MATH = String.raw`\\\([\s\S]+?\\\)|\$\$[^\n$]+\$\$`
const DISPLAY_MATH = String.raw` {0,3}(?:\\\[[\s\S]+?\\\]|\$\$ *\n[\s\S]+?\n {0,3}\$\$) *(?:\n+|$)`
const FOOTNOTE = String.raw`\[\^[^\]\s]+\]`
const FOOTNOTE_DEFINITION = FOOTNOTE + String.raw`:[^\n]*(?:\n(?: {4}|\t)[^\n]*)*(?:\n+|$)`

// A fence nested in a numbered list keeps part of the list's indentation on
// every line after its first; the closing fence shows how much.
function sourceBlock(token: MarkdownToken): JSONContent {
  const raw = (token.raw ?? '').replace(/\n+$/, '')
  const indent = /\n( +)(?:`{3,}|~{3,})$/.exec(raw)?.[1] ?? ''
  const source = raw.split('\n').map((line, index) => (index && line.startsWith(indent) ? line.slice(indent.length) : line)).join('\n')
  return { type: 'sourceBlock', attrs: { source } }
}

const sourceInline = (token: MarkdownToken): JSONContent => ({ type: 'sourceInline', attrs: { source: token.raw } })

const sourceOf = (node: JSONContent) => node.attrs?.source as string

// Fenced code, artifacts, display maths and footnote texts stay as the
// Markdown they were written in: the reader draws them, and editing one edits
// its source. A block opens for editing when the person has just typed its fence.
const sourceBlockNode = (addNodeView?: () => NodeViewRenderer) =>
  Node.create({
    name: 'sourceBlock',
    group: 'block',
    atom: true,
    addAttributes: () => ({ source: { default: '' }, editing: { default: false, rendered: false } }),
    parseHTML: () => [{ tag: 'pre[data-source]', getAttrs: (element) => ({ source: element.textContent }) }],
    renderHTML: ({ node }) => ['pre', { 'data-source': '' }, node.attrs.source as string],
    markdownTokenName: 'code',
    parseMarkdown: sourceBlock,
    renderMarkdown: sourceOf,
    addNodeView,
    addInputRules() {
      return [
        new InputRule({
          find: /^```([\w-]*)\s$/,
          handler: ({ range, match, chain }) => {
            chain().insertContentAt(range, { type: this.name, attrs: { source: '```' + match[1] + '\n\n```', editing: true } }).run()
          },
        }),
      ]
    },
  })

// Images, inline maths and footnote marks are kept the same way inside a line.
const sourceInlineNode = (addNodeView?: () => NodeViewRenderer) =>
  Node.create({
    name: 'sourceInline',
    group: 'inline',
    inline: true,
    atom: true,
    addAttributes: () => ({ source: { default: '' } }),
    parseHTML: () => [{ tag: 'span[data-source]', getAttrs: (element) => ({ source: element.textContent }) }],
    renderHTML: ({ node }) => ['span', { 'data-source': '' }, node.attrs.source as string],
    markdownTokenName: 'image',
    parseMarkdown: sourceInline,
    renderMarkdown: sourceOf,
    addNodeView,
    addInputRules() {
      return [
        new InputRule({
          find: new RegExp(`(?:${INLINE_MATH})$`),
          handler: ({ range, match, chain }) => {
            chain().insertContentAt(range, { type: this.name, attrs: { source: match[0] } }).run()
          },
        }),
      ]
    },
  })

// Each further kind of Markdown kept as source is read under its own token:
// one Markdown already has, or one found by pattern, a block at a line's start.
function kept(name: string, level: 'block' | 'inline', pattern?: string) {
  return Extension.create({
    name: `${name}Source`,
    markdownTokenName: name,
    parseMarkdown: level === 'block' ? sourceBlock : sourceInline,
    markdownTokenizer: pattern ? tokenizer(name, level, pattern) : undefined,
  })
}

function tokenizer(name: string, level: 'block' | 'inline', pattern: string): MarkdownTokenizer {
  const start = new RegExp(level === 'block' ? `^(?:${pattern})` : pattern, 'm')
  const whole = new RegExp(`^(?:${pattern})`)
  return {
    name,
    level,
    start: (src) => src.search(start),
    tokenize: (src) => {
      const raw = whole.exec(src)?.[0]
      return raw ? { type: name, raw } : undefined
    },
  }
}

// A table is written without the blank lines the extension puts around it,
// which the document already has.
const MarkdownTable = Table.extend({ renderMarkdown: (node, helpers) => renderTableToMarkdown(node, helpers).trim() })

// Text copied as Markdown arrives as what it says, not as its punctuation.
const BLOCK_MARKDOWN = /^(?:#{1,6} |[-*+] |\d+\. |> |```|\|.+\|\s*$)/m
const MarkdownPaste = Extension.create({
  name: 'markdownPaste',
  addProseMirrorPlugins() {
    const paste = (event: ClipboardEvent) => {
      const data = event.clipboardData
      const text = data?.types.includes('text/html') ? '' : (data?.getData('text/plain') ?? '')
      return BLOCK_MARKDOWN.test(text) && this.editor.commands.insertContent(this.editor.markdown!.parse(text).content ?? [])
    }
    return [new Plugin({ props: { handlePaste: (_view, event) => paste(event) } })]
  },
})

// replaceSource puts the Markdown the person wrote in place of the source node
// at pos, as whatever that Markdown now is: nothing, the same kind, or prose.
export function replaceSource(editor: Editor, pos: number, markdown: string): boolean {
  const node = editor.state.doc.nodeAt(pos)!
  const range = { from: pos, to: pos + node.nodeSize }
  const blocks = editor.markdown!.parse(markdown).content ?? []
  const content = node.isInline && blocks.length === 1 && blocks[0].type === 'paragraph' ? (blocks[0].content ?? []) : blocks
  return content.length ? editor.commands.insertContentAt(range, content) : editor.commands.deleteRange(range)
}

// documentKit is a Markdown document as the reader shows it, its source nodes
// drawn by sourceView. Underline has no Markdown form, a trailing empty line
// would be an edit the person never made, and links are opened by the reader
// rather than the browser.
export const documentKit = (sourceView?: () => NodeViewRenderer) => [
  StarterKit.configure({ underline: false, trailingNode: false, codeBlock: false, link: { openOnClick: false, HTMLAttributes: { class: 'chat-prose-link', target: null, rel: null } } }),
  Markdown,
  TaskList,
  TaskItem.configure({ nested: true }),
  sourceBlockNode(sourceView),
  sourceInlineNode(sourceView),
  TableKit.configure({ table: false }),
  MarkdownTable,
  MarkdownPaste,
  Placeholder.configure({ placeholder: 'Type / for blocks' }),
  kept('displayMath', 'block', DISPLAY_MATH),
  kept('footnoteDefinition', 'block', FOOTNOTE_DEFINITION),
  kept('inlineMath', 'inline', INLINE_MATH),
  kept('footnote', 'inline', FOOTNOTE),
]
