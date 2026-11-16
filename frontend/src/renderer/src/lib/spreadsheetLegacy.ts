import { CFB, type CellAddress, type WorkBook } from 'xlsx'

const BOUND_SHEET = 0x0085
const WINDOW = 0x023e
const PANE = 0x0041
const END = 0x000a

export function legacySpreadsheetPanes(book: WorkBook): CellAddress[] {
  const container = (book as WorkBook & { cfb?: unknown }).cfb
  if (!container) return []
  const entry = (CFB.find(container, '/Workbook') ?? CFB.find(container, '/Book')) as { content?: ArrayLike<number> } | undefined
  if (!entry?.content) return []
  const bytes = Uint8Array.from(entry.content)
  const data = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  function* records(start: number) {
    for (let offset = start; offset + 4 <= data.byteLength;) {
      const id = data.getUint16(offset, true)
      const length = data.getUint16(offset + 2, true)
      offset += 4
      if (offset + length > data.byteLength || id === END) return
      yield { id, offset, length }
      offset += length
    }
  }
  const sheets: number[] = []
  for (const record of records(0)) {
    if (record.id === BOUND_SHEET && record.length >= 4) sheets.push(data.getUint32(record.offset, true))
  }
  return sheets.map((start) => {
    let frozen = false
    for (const { id, offset, length } of records(start)) {
      if (id === WINDOW && length >= 2) frozen = Boolean(data.getUint16(offset, true) & 0x0008)
      if (id === PANE && length >= 4 && frozen) return { c: data.getUint16(offset, true), r: data.getUint16(offset + 2, true) }
    }
    return { r: 0, c: 0 }
  })
}
