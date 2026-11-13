import type { ChainedCommands, Editor } from '@tiptap/core'
import type { EditorState } from '@tiptap/pm/state'
import { isInTable, selectedRect } from '@tiptap/pm/tables'
import { AlignCenter, AlignLeft, AlignRight, ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Calendar, CodeXml, Columns3, Heading1, Heading2, Heading3, Image, List, ListOrdered, ListTodo, Minus, Pilcrow, Rows3, Sigma, Table, TextQuote, Trash2, type LucideIcon } from 'lucide-react'

// One thing a person can turn a line into or put on the page, offered by the
// slash menu and the toolbar alike. words are further names it is found by.
export type DocumentCommand = {
  label: string
  icon: LucideIcon
  words?: string
  run: (chain: ChainedCommands) => ChainedCommands
  active?: (editor: Editor) => boolean
}

const heading = (level: 1 | 2 | 3, icon: LucideIcon): DocumentCommand => ({
  label: `Heading ${level}`,
  icon,
  words: `h${level} title`,
  run: (chain) => chain.setHeading({ level }),
  active: (editor) => editor.isActive('heading', { level }),
})

// A block kept as Markdown source opens ready to be typed into.
const source = (label: string, icon: LucideIcon, words: string, markdown: string): DocumentCommand => ({
  label,
  icon,
  words,
  run: (chain) => chain.insertContent({ type: 'sourceBlock', attrs: { source: markdown, editing: true } }),
})

const text: DocumentCommand = { label: 'Text', icon: Pilcrow, run: (chain) => chain.setParagraph(), active: (editor) => editor.isActive('paragraph') }
const headings = [heading(1, Heading1), heading(2, Heading2), heading(3, Heading3)]
const table: DocumentCommand = { label: 'Table', icon: Table, words: 'grid', run: (chain) => chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }) }
const bulleted: DocumentCommand = { label: 'Bulleted list', icon: List, words: 'unordered', run: (chain) => chain.toggleBulletList(), active: (editor) => editor.isActive('bulletList') }
const checklist: DocumentCommand = { label: 'Checklist', icon: ListTodo, words: 'todo task checkbox', run: (chain) => chain.toggleTaskList(), active: (editor) => editor.isActive('taskList') }
const numbered: DocumentCommand = { label: 'Numbered list', icon: ListOrdered, words: 'ordered', run: (chain) => chain.toggleOrderedList(), active: (editor) => editor.isActive('orderedList') }
const quote: DocumentCommand = { label: 'Quote', icon: TextQuote, words: 'blockquote', run: (chain) => chain.toggleBlockquote(), active: (editor) => editor.isActive('blockquote') }
const code = source('Code block', CodeXml, 'snippet fence', '```\n\n```')
const formula = source('Formula', Sigma, 'math equation latex', '\\[\n\n\\]')
const divider: DocumentCommand = { label: 'Divider', icon: Minus, words: 'rule line hr', run: (chain) => chain.setHorizontalRule() }
const date: DocumentCommand = {
  label: 'Date',
  icon: Calendar,
  words: 'today',
  run: (chain) => chain.insertContent(new Date().toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })),
}

// Markdown aligns a whole column, so every cell of the caret's column is set.
const alignColumn = (label: string, icon: LucideIcon, align: string): DocumentCommand => ({
  label,
  icon,
  run: (chain) =>
    chain.command(({ state, tr }) => {
      if (!isInTable(state)) return false
      const { map, tableStart, left, right } = selectedRect(state)
      for (const cell of map.cellsInRect({ left, right, top: 0, bottom: map.height })) tr.setNodeAttribute(tableStart + cell, 'align', align)
      return true
    }),
})

export const tableEdits: DocumentCommand[][] = [
  [
    { label: 'Insert row above', icon: ArrowUp, run: (chain) => chain.addRowBefore() },
    { label: 'Insert row below', icon: ArrowDown, run: (chain) => chain.addRowAfter() },
    { label: 'Insert column left', icon: ArrowLeft, run: (chain) => chain.addColumnBefore() },
    { label: 'Insert column right', icon: ArrowRight, run: (chain) => chain.addColumnAfter() },
  ],
  [alignColumn('Align left', AlignLeft, 'left'), alignColumn('Align center', AlignCenter, 'center'), alignColumn('Align right', AlignRight, 'right')],
  [
    { label: 'Delete row', icon: Rows3, run: (chain) => chain.deleteRow() },
    { label: 'Delete column', icon: Columns3, run: (chain) => chain.deleteColumn() },
    { label: 'Delete table', icon: Trash2, run: (chain) => chain.deleteTable() },
  ],
]

// pickImage asks the person for a picture, which arrives after the menu has closed.
export function documentCommands(pickImage: () => void) {
  const image: DocumentCommand = {
    label: 'Image',
    icon: Image,
    words: 'picture photo',
    run: (chain) => {
      pickImage()
      return chain
    },
  }
  return {
    styles: [text, ...headings],
    table,
    image,
    checklist,
    more: [[bulleted, numbered, quote], [code, formula, date, divider]],
    slash: [[...headings, table], [bulleted, checklist, code, date, divider, formula, image, numbered, quote]],
  }
}

// The "/words" being typed at the caret, where they start a line or a word.
export function slashQuery(state: EditorState): { from: number; to: number; query: string } | null {
  const { $from, empty } = state.selection
  if (!empty || !$from.parent.isTextblock) return null
  const match = /(?:^|\s)\/([a-z0-9 ]*)$/i.exec($from.parent.textBetween(0, $from.parentOffset, undefined, '\ufffc'))
  return match ? { from: $from.pos - match[1].length - 1, to: $from.pos, query: match[1].toLowerCase() } : null
}

export const matchesQuery = (command: DocumentCommand, query: string) => `${command.label} ${command.words ?? ''}`.toLowerCase().includes(query)
