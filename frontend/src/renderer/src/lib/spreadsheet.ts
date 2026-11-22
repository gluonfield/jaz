import { read, utils, type CellAddress, type CellObject, type Range, type WorkSheet } from 'xlsx'
import { spreadsheetMetadata, type SpreadsheetMetadata, type SpreadsheetStyle } from '@/lib/spreadsheetMetadata'

export type SpreadsheetSelection = { anchor: CellAddress; focus: CellAddress }

export interface SpreadsheetSheet extends Omit<SpreadsheetMetadata, 'defaultColumnCharacters'> {
  name: string
  source: WorkSheet
  rows: number[]
  columns: number[]
  defaultColumnWidth: number
  merges: Range[]
  used: Range
}

function offsets(length: number, size: (index: number) => number): number[] {
  const result = [0]
  for (let index = 0; index < length; index += 1) result.push(result[index] + Math.max(0, size(index)))
  return result
}

export function readSpreadsheet(data: ArrayBuffer, digitWidth: (font: SpreadsheetStyle) => number): SpreadsheetSheet[] {
  const book = read(data, { type: 'array', dense: true, raw: true, cellStyles: true, cellNF: true, cellHTML: false, bookFiles: true, sheetStubs: true })
  const metadata = spreadsheetMetadata(book)
  return book.SheetNames.map((name) => {
    const source = book.Sheets[name]
    const range = utils.decode_range(source['!ref'] ?? 'A1')
    const { defaultColumnCharacters, ...meta } = metadata.get(source)!
    const digit = digitWidth(meta.defaultStyle)
    const pixels = (width: number) => Math.floor((256 * width + Math.floor(128 / digit)) / 256 * digit)
    const merges = source['!merges'] ?? []
    const rowCount = merges.reduce((count, merge) => Math.max(count, merge.e.r + 1), range.e.r + 1)
    const columnCount = merges.reduce((count, merge) => Math.max(count, merge.e.c + 1), range.e.c + 1)
    return {
      name, source, ...meta, merges, used: range,
      defaultColumnWidth: pixels(defaultColumnCharacters),
      frozen: { r: Math.min(rowCount, meta.frozen.r), c: Math.min(columnCount, meta.frozen.c) },
      rows: offsets(rowCount, (index) => {
        const row = source['!rows']?.[index]
        return row?.hidden ? 0 : row?.hpt !== undefined ? row.hpt * 4 / 3 : row?.hpx ?? meta.defaultRowHeight
      }),
      columns: offsets(columnCount, (index) => {
        const col = source['!cols']?.[index]
        return col?.hidden ? 0 : col?.width !== undefined ? pixels(col.width) : col?.wpx ?? pixels(defaultColumnCharacters)
      }),
    }
  })
}

export function spreadsheetCell(sheet: SpreadsheetSheet, point: CellAddress): CellObject | undefined {
  return sheet.source['!data']?.[point.r]?.[point.c]
}

export function spreadsheetText(sheet: SpreadsheetSheet, point: CellAddress): string {
  const cell = spreadsheetCell(sheet, point)
  return cell ? cell.w ?? (cell.v === undefined ? '' : String(cell.v)) : ''
}

export function spreadsheetStyle(sheet: SpreadsheetSheet, point: CellAddress): SpreadsheetStyle {
  const cell = spreadsheetCell(sheet, point)
  return cell && sheet.formats.get(cell) || sheet.defaultStyle
}

export function mergedRange(sheet: SpreadsheetSheet, point: CellAddress): Range {
  return sheet.merges.find(({ s, e }) => point.r >= s.r && point.r <= e.r && point.c >= s.c && point.c <= e.c) ?? { s: point, e: point }
}

export function selectionRange(sheet: SpreadsheetSheet, { anchor, focus }: SpreadsheetSelection): Range {
  const range = { s: { r: Math.min(anchor.r, focus.r), c: Math.min(anchor.c, focus.c) }, e: { r: Math.max(anchor.r, focus.r), c: Math.max(anchor.c, focus.c) } }
  let changed = true
  while (changed) {
    changed = false
    for (const { s, e } of sheet.merges) {
      if (s.r > range.e.r || e.r < range.s.r || s.c > range.e.c || e.c < range.s.c) continue
      if (s.r < range.s.r || e.r > range.e.r || s.c < range.s.c || e.c > range.e.c) {
        range.s = { r: Math.min(range.s.r, s.r), c: Math.min(range.s.c, s.c) }
        range.e = { r: Math.max(range.e.r, e.r), c: Math.max(range.e.c, e.c) }
        changed = true
      }
    }
  }
  return range
}

export function initialSpreadsheetSelection(sheet: SpreadsheetSheet, address = 'A1'): SpreadsheetSelection {
  const range = /^\$?[A-Z]+\$?[1-9]\d*(?::\$?[A-Z]+\$?[1-9]\d*)?$/i.test(address) ? utils.decode_range(address.toUpperCase().replaceAll('$', '')) : utils.decode_range('A1')
  const bound = (point: CellAddress) => ({ r: Math.min(point.r, sheet.rows.length - 2), c: Math.min(point.c, sheet.columns.length - 2) })
  return { anchor: bound(range.s), focus: bound(range.e) }
}

export function selectionTSV(sheet: SpreadsheetSheet, selection: SpreadsheetSelection): string {
  const { s, e } = selectionRange(sheet, selection)
  const rows: string[] = []
  for (let r = s.r; r <= e.r; r += 1) {
    const cells: string[] = []
    for (let c = s.c; c <= e.c; c += 1) {
      const merge = mergedRange(sheet, { r, c })
      const text = merge.s.r === r && merge.s.c === c ? spreadsheetText(sheet, { r, c }) : ''
      cells.push(/[\t\n\r"]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text)
    }
    rows.push(cells.join('\t'))
  }
  return rows.join('\n')
}

export function spreadsheetIndex(axis: number[], position: number): number {
  let low = 0
  let high = axis.length - 1
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (axis[middle + 1] <= position) low = middle + 1
    else high = middle
  }
  return Math.min(low, axis.length - 2)
}

export function visibleIndices(axis: number[], start: number, end: number): number[] {
  const result: number[] = []
  for (let index = spreadsheetIndex(axis, start); index < axis.length - 1 && axis[index] < end; index += 1) {
    if (axis[index + 1] > axis[index]) result.push(index)
  }
  return result
}

export function spreadsheetLayout(sheet: SpreadsheetSheet, width: number, height: number): SpreadsheetSheet {
  const extend = (axis: number[], extent: number, size: number) => {
    const result = [...axis]
    while (result.at(-1)! < extent) result.push(result.at(-1)! + size)
    return result
  }
  const frozen = (axis: number[], count: number, extent: number) => axis[count] <= extent / 2 ? count : 0
  return { ...sheet, frozen: { r: frozen(sheet.rows, sheet.frozen.r, height), c: frozen(sheet.columns, sheet.frozen.c, width) }, columns: extend(sheet.columns, width, sheet.defaultColumnWidth), rows: extend(sheet.rows, height, sheet.defaultRowHeight) }
}

export function cellTextBounds(sheet: SpreadsheetSheet, range: Range): { left: number; width: number } {
  const cell = spreadsheetCell(sheet, range.s)
  const style = spreadsheetStyle(sheet, range.s)
  let start = range.s.c
  let end = range.e.c
  if (range.s.c === range.e.c && range.s.r === range.e.r && cell?.t === 's' && !style.wrap) {
    const empty = (c: number) => {
      const point = { r: range.s.r, c }
      const next = spreadsheetCell(sheet, point)
      const merge = mergedRange(sheet, point)
      return (next?.v === undefined || next.v === '') && !next?.f && merge.s.c === merge.e.c && merge.s.r === merge.e.r
    }
    if (style.horizontal === 'center' || style.horizontal === 'right') {
      while (start > 0 && empty(start - 1)) start -= 1
    }
    if (style.horizontal !== 'right') {
      while (end + 1 < sheet.columns.length - 1 && empty(end + 1)) end += 1
    }
  }
  return { left: sheet.columns[start] - sheet.columns[range.s.c], width: sheet.columns[end + 1] - sheet.columns[start] }
}

export function moveSpreadsheetSelection(sheet: SpreadsheetSheet, selection: SpreadsheetSelection, key: string, extend: boolean, edge: boolean, page = 20): SpreadsheetSelection | undefined {
  const point = { ...selection.focus }
  const merge = mergedRange(sheet, point)
  let axis: 'r' | 'c' = 'r'
  let direction = 1
  switch (key) {
    case 'ArrowLeft':
      axis = 'c'
      direction = -1
      point.c = merge.s.c - 1
      break
    case 'ArrowRight':
      axis = 'c'
      point.c = merge.e.c + 1
      break
    case 'ArrowUp':
      direction = -1
      point.r = merge.s.r - 1
      break
    case 'ArrowDown':
      point.r = merge.e.r + 1
      break
    case 'Tab':
      axis = 'c'
      direction = extend ? -1 : 1
      point.c = (extend ? merge.s.c : merge.e.c) + direction
      extend = false
      break
    case 'Enter':
      direction = extend ? -1 : 1
      point.r = (extend ? merge.s.r : merge.e.r) + direction
      extend = false
      break
    case 'PageUp':
      direction = -1
      point.r -= page
      break
    case 'PageDown':
      point.r += page
      break
    case 'Home':
      axis = 'c'
      direction = -1
      point.c = 0
      if (edge) point.r = 0
      break
    case 'End':
      axis = 'c'
      point.c = sheet.columns.length - 2
      if (edge) point.r = sheet.rows.length - 2
      break
    default: return undefined
  }
  const positions = axis === 'r' ? sheet.rows : sheet.columns
  const last = positions.length - 2
  point[axis] = Math.max(0, Math.min(point[axis], last))
  if (edge && key.startsWith('Arrow')) point[axis] = direction > 0 ? last : 0
  while (point[axis] >= 0 && point[axis] <= last && positions[point[axis] + 1] === positions[point[axis]]) point[axis] += direction
  if (point[axis] < 0 || point[axis] > last) return selection
  const next = mergedRange(sheet, point).s
  return { anchor: extend ? selection.anchor : next, focus: next }
}
