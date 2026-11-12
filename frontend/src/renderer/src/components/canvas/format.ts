import type { CanvasItem } from '@/lib/api/canvas'

export const SIZES = ['small', 'body', 'large', 'title', 'huge'] as const
export const SHAPES = ['rect', 'rounded', 'pill', 'ellipse', 'diamond', 'hexagon', 'parallelogram', 'document', 'database', 'queue', 'cloud', 'person'] as const
export const FILLS = ['white', 'red', 'orange', 'yellow', 'green', 'blue', 'purple'] as const
export const INKS = ['gray', 'red', 'orange', 'green', 'blue', 'purple'] as const
export const ALIGNS = ['left', 'center', 'right'] as const

export type Size = (typeof SIZES)[number]
export type Shape = (typeof SHAPES)[number]
export type Align = (typeof ALIGNS)[number]

// JSON Canvas numbers its preset colours; Jaz also takes them by name.
const PRESETS: Record<string, string> = { 1: 'red', 2: 'orange', 3: 'yellow', 4: 'green', 5: 'blue', 6: 'purple' }

function pick<T extends string>(options: readonly T[], value: unknown): T | undefined {
  return options.find((option) => option === value)
}

// A named colour becomes a class and any other CSS colour an inline value.
export interface Tint {
  name?: string
  custom?: string
}

function tint(options: readonly string[], value: unknown): Tint | undefined {
  if (typeof value !== 'string' || !value) return undefined
  const name = pick(options, PRESETS[value] ?? value)
  return name ? { name } : { custom: value }
}

export interface Look {
  // Plain text flows on the board, a note is a sticky, a block is a shape.
  kind: 'text' | 'note' | 'block'
  shape?: Shape
  fill?: Tint
  ink?: Tint
  size?: Size
  align?: Align
  hollow: boolean
}

export function lookOf(item: CanvasItem): Look {
  const shape = pick(SHAPES, item.shape)
  const fill = tint(FILLS, item.color)
  return {
    kind: shape ? 'block' : fill ? 'note' : 'text',
    shape,
    fill,
    ink: tint(INKS, item.ink),
    size: pick(SIZES, item.size),
    align: pick(ALIGNS, item.textAlign),
    hollow: item.hollow === true,
  }
}

const MARKER = /^\s*(?:[-*+]|\d+\.)\s+/
const LINE = /^(\s*(?:[-*+]|\d+\.)\s+|\s*#+\s+|\s*)(.*)$/

// Bold or italic for the whole of each line, or plain again when every line
// already carries the mark.
export function toggleMark(text: string, mark: '**' | '*'): string {
  const lines = text.split('\n').map((line) => LINE.exec(line)!)
  const stars = (body: string, edge: RegExp) => edge.exec(body)![0].length
  const marked = (body: string) => {
    const [lead, trail] = [stars(body, /^\**/), stars(body, /\**$/)]
    return body.length > lead && (mark === '**' ? lead >= 2 && trail >= 2 : lead % 2 === 1 && trail % 2 === 1)
  }
  const written = lines.filter(([, , body]) => body.trim())
  const strip = written.length > 0 && written.every(([, , body]) => marked(body))
  return lines.map(([line, lead, body]) => {
    if (!body.trim() || (!strip && marked(body))) return line
    return lead + (strip ? body.slice(mark.length, -mark.length) : mark + body + mark)
  }).join('\n')
}

// The same for a selection inside an editor, which keeps what was selected.
export function toggleMarkAt(text: string, start: number, end: number, mark: '**' | '*'): { text: string; start: number; end: number } {
  const size = mark.length
  if (text.slice(start - size, start) === mark && text.slice(end, end + size) === mark) {
    return { text: text.slice(0, start - size) + text.slice(start, end) + text.slice(end + size), start: start - size, end: end - size }
  }
  return { text: text.slice(0, start) + mark + text.slice(start, end) + mark + text.slice(end), start: start + size, end: end + size }
}

// Every line a bullet, or none of them.
export function toggleList(text: string): string {
  const lines = text.split('\n')
  const written = lines.filter((line) => line.trim())
  const strip = written.length > 0 && written.every((line) => /^\s*[-*+]\s+/.test(line))
  return lines.map((line) => {
    if (!line.trim()) return line
    return strip ? line.replace(/^(\s*)[-*+]\s+/, '$1') : `- ${line.replace(MARKER, '')}`
  }).join('\n')
}

export function tableRows(item: CanvasItem): string[][] {
  const rows = Array.isArray(item.rows) ? item.rows.filter(Array.isArray) : []
  const width = Math.max(1, ...rows.map((row) => row.length))
  const cells = rows.map((row) => Array.from({ length: width }, (_, column) => (row[column] === undefined || row[column] === null ? '' : String(row[column]))))
  return cells.length ? cells : [Array.from({ length: width }, () => '')]
}

export type TableEdit =
  | { cell: [row: number, column: number]; text: string }
  | { addRow: number }
  | { addColumn: number }
  | { removeRow: number }
  | { removeColumn: number }

// A table keeps at least one row and one column.
export function editTable(rows: string[][], edit: TableEdit): string[][] {
  if ('cell' in edit) {
    return rows.map((row, r) => row.map((cell, c) => (r === edit.cell[0] && c === edit.cell[1] ? edit.text : cell)))
  }
  if ('addRow' in edit) {
    return [...rows.slice(0, edit.addRow), rows[0].map(() => ''), ...rows.slice(edit.addRow)]
  }
  if ('addColumn' in edit) {
    return rows.map((row) => [...row.slice(0, edit.addColumn), '', ...row.slice(edit.addColumn)])
  }
  if ('removeRow' in edit) {
    return rows.length > 1 ? rows.filter((_, r) => r !== edit.removeRow) : rows
  }
  return rows[0].length > 1 ? rows.map((row) => row.filter((_, c) => c !== edit.removeColumn)) : rows
}
