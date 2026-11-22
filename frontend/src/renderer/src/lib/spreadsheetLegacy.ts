import { CFB, type WorkBook } from 'xlsx'
import { BORDER_STYLES, borderLine, type SpreadsheetCorner, type SpreadsheetImageAnchor, type SpreadsheetMetadata, type SpreadsheetStyle } from '@/lib/spreadsheetFormat'

const FORMULA = 0x0006
const END = 0x000a
const CONTINUE = 0x003c
const PANE = 0x0041
const FONT = 0x0031
const BOUND_SHEET = 0x0085
const PALETTE = 0x0092
const MULTIPLE_RK = 0x00bd
const MULTIPLE_BLANK = 0x00be
const RICH_STRING = 0x00d6
const XF = 0x00e0
const DRAWING_GROUP = 0x00eb
const DRAWING = 0x00ec
const SHARED_STRING_LABEL = 0x00fd
const BLANK = 0x0201
const NUMBER = 0x0203
const LABEL = 0x0204
const BOOLEAN = 0x0205
const WINDOW = 0x023e
const RK = 0x027e
const BOF = 0x0809
const BIFF8 = 0x0600
const CELLS = new Set([FORMULA, RICH_STRING, SHARED_STRING_LABEL, BLANK, NUMBER, LABEL, BOOLEAN, RK])

const ART_BLIP_ENTRY = 0xf007
const ART_SHAPE = 0xf004
const ART_OPTIONS = 0xf00b
const ART_ANCHOR = 0xf010
const ART_JPEG = 0xf01d
const ART_PNG = 0xf01e
const BLIP_PROPERTY = 0x0104
const SIGNATURES: Record<number, { type: string; bytes: number[] }> = {
  [ART_JPEG]: { type: 'image/jpeg', bytes: [0xff, 0xd8, 0xff] },
  [ART_PNG]: { type: 'image/png', bytes: [0x89, 0x50, 0x4e, 0x47] },
}

const BASE_COLORS = ['000000', 'FFFFFF', 'FF0000', '00FF00', '0000FF', 'FFFF00', 'FF00FF', '00FFFF']
const DEFAULT_PALETTE = [
  ...BASE_COLORS, ...BASE_COLORS, '800000', '008000', '000080', '808000', '800080', '008080', 'C0C0C0', '808080',
  '9999FF', '993366', 'FFFFCC', 'CCFFFF', '660066', 'FF8080', '0066CC', 'CCCCFF', '000080', 'FF00FF', 'FFFF00', '00FFFF', '800080', '800000', '008080', '0000FF',
  '00CCFF', 'CCFFFF', 'CCFFCC', 'FFFF99', '99CCFF', 'FF99CC', 'CC99FF', 'FFCC99', '3366FF', '33CCCC', '99CC00', 'FFCC00', 'FF9900', 'FF6600', '666699', '969696',
  '003366', '339966', '003300', '333300', '993300', '993366', '333399', '333333',
]
const HORIZONTAL: SpreadsheetStyle['horizontal'][] = [undefined, 'left', 'center', 'right', undefined, 'justify', 'center']
const VERTICAL: SpreadsheetStyle['vertical'][] = ['top', 'center', 'bottom']

type Blip = Pick<SpreadsheetImageAnchor, 'type' | 'data'>

function* artRecords(data: DataView) {
  for (let offset = 0; offset + 8 <= data.byteLength;) {
    const header = data.getUint16(offset, true)
    const length = data.getUint32(offset + 4, true)
    yield { type: data.getUint16(offset + 2, true), instance: header >> 4, offset: offset + 8, length }
    offset += 8 + ((header & 0xf) === 0xf ? 0 : length)
  }
}

function blips(group: DataView): (Blip | undefined)[] {
  const result: (Blip | undefined)[] = []
  for (const { type, offset, length } of artRecords(group)) {
    if (type !== ART_BLIP_ENTRY) continue
    const blip = offset + 36 + group.getUint8(offset + 33)
    const signature = blip + 8 <= offset + length ? SIGNATURES[group.getUint16(blip + 2, true)] : undefined
    const bytes = new Uint8Array(group.buffer, group.byteOffset + blip, Math.max(0, offset + length - blip))
    let start = 8
    while (signature && start < 64 && !signature.bytes.every((byte, at) => bytes[start + at] === byte)) start += 1
    result.push(signature && start < 64 ? { type: signature.type, data: bytes.slice(start) } : undefined)
  }
  return result
}

function images(drawing: DataView, blips: (Blip | undefined)[]): SpreadsheetImageAnchor[] {
  const result: SpreadsheetImageAnchor[] = []
  let shape: { blip?: Blip; anchor?: { from: SpreadsheetCorner; to: SpreadsheetCorner } } = {}
  const finish = () => {
    if (shape.blip && shape.anchor) result.push({ ...shape.blip, ...shape.anchor, offsets: 'cell fractions' })
    shape = {}
  }
  for (const { type, instance, offset, length } of artRecords(drawing)) {
    if (type === ART_SHAPE) finish()
    if (type === ART_OPTIONS) {
      for (let entry = offset; entry < offset + instance * 6 && entry + 6 <= drawing.byteLength; entry += 6) {
        if ((drawing.getUint16(entry, true) & 0x3fff) === BLIP_PROPERTY) shape.blip = blips[drawing.getUint32(entry + 2, true) - 1]
      }
    }
    if (type === ART_ANCHOR && length >= 18) {
      const corner = (at: number) => ({ c: drawing.getUint16(at, true), x: drawing.getUint16(at + 2, true) / 1024, r: drawing.getUint16(at + 4, true), y: drawing.getUint16(at + 6, true) / 256 })
      shape.anchor = { from: corner(offset + 2), to: corner(offset + 10) }
    }
  }
  finish()
  return result
}

export function legacySpreadsheet(book: WorkBook): Partial<SpreadsheetMetadata>[] {
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
  const joined = (start: number, wanted: number) => {
    const parts: Uint8Array[] = []
    let continuing = false
    for (const { id, offset, length } of records(start)) {
      continuing = id === wanted || (continuing && id === CONTINUE)
      if (continuing) parts.push(bytes.subarray(offset, offset + length))
    }
    const result = new Uint8Array(parts.reduce((total, part) => total + part.length, 0))
    parts.reduce((at, part) => (result.set(part, at), at + part.length), 0)
    return new DataView(result.buffer)
  }
  const modern = data.byteLength >= 6 && data.getUint16(0, true) === BOF && data.getUint16(4, true) === BIFF8
  const sheets: number[] = []
  const fonts: number[] = []
  const formats: number[] = []
  const palette = [...DEFAULT_PALETTE]
  for (const { id, offset, length } of records(0)) {
    if (id === BOUND_SHEET && length >= 4) sheets.push(data.getUint32(offset, true))
    if (!modern) continue
    if (id === PALETTE) {
      for (let index = 0; index < data.getUint16(offset, true) && index < 56; index += 1) palette[8 + index] = [0, 1, 2].map((channel) => data.getUint8(offset + 2 + index * 4 + channel).toString(16).padStart(2, '0')).join('')
    }
    if (id === FONT && length >= 16) fonts.push(offset)
    if (id === XF && length >= 20) formats.push(offset)
  }
  const color = (index: number) => palette[index] && `#${palette[index]}`
  const font = (offset?: number): SpreadsheetStyle => {
    if (offset === undefined) return {}
    const wide = data.getUint8(offset + 15) & 1
    return {
      fontSize: data.getUint16(offset, true) / 15,
      italic: Boolean(data.getUint16(offset + 2, true) & 0x0002),
      color: color(data.getUint16(offset + 4, true)),
      bold: data.getUint16(offset + 6, true) >= 600,
      fontFamily: new TextDecoder(wide ? 'utf-16le' : 'latin1').decode(bytes.subarray(offset + 16, offset + 16 + data.getUint8(offset + 14) * (wide ? 2 : 1))),
    }
  }
  const styles = formats.map((offset): SpreadsheetStyle => {
    const fontIndex = data.getUint16(offset, true)
    const alignment = data.getUint8(offset + 6)
    const lines = data.getUint32(offset + 10, true)
    const fills = data.getUint32(offset + 14, true)
    const side = (line: number, ink: number) => line ? borderLine(BORDER_STYLES[line], color(ink)) : undefined
    return {
      ...font(fonts[fontIndex > 4 ? fontIndex - 1 : fontIndex]),
      horizontal: HORIZONTAL[alignment & 0x07],
      vertical: VERTICAL[alignment >> 4 & 0x07] ?? 'bottom',
      wrap: Boolean(alignment & 0x08),
      fill: fills >>> 26 === 1 ? color(data.getUint16(offset + 18, true) & 0x7f) : undefined,
      border: { left: side(lines & 0xf, lines >>> 16 & 0x7f), right: side(lines >>> 4 & 0xf, lines >>> 23 & 0x7f), top: side(lines >>> 8 & 0xf, fills & 0x7f), bottom: side(lines >>> 12 & 0xf, fills >>> 7 & 0x7f) },
    }
  })
  const pictures = modern ? blips(joined(0, DRAWING_GROUP)) : []
  return sheets.map((start, index) => {
    const cells = book.Sheets[book.SheetNames[index]]?.['!data']
    const sheet: Partial<SpreadsheetMetadata> = modern ? { formats: new Map(), defaultStyle: styles[15] ?? styles[0] ?? {}, images: images(joined(start, DRAWING), pictures) } : {}
    const format = (r: number, c: number, at: number) => {
      const cell = cells?.[r]?.[c]
      const style = styles[data.getUint16(at, true)]
      if (cell && style) sheet.formats?.set(cell, style)
    }
    let frozen = false
    for (const { id, offset, length } of records(start)) {
      if (id === WINDOW && length >= 2) {
        frozen = Boolean(data.getUint16(offset, true) & 0x0008)
        sheet.gridlines = Boolean(data.getUint16(offset, true) & 0x0002)
      }
      if (id === PANE && length >= 4 && frozen) sheet.frozen = { c: data.getUint16(offset, true), r: data.getUint16(offset + 2, true) }
      if (length < 6) continue
      const r = data.getUint16(offset, true)
      const c = data.getUint16(offset + 2, true)
      if (CELLS.has(id)) format(r, c, offset + 4)
      const width = id === MULTIPLE_BLANK ? 2 : id === MULTIPLE_RK ? 6 : 0
      for (let at = offset + 4; width && at + width <= offset + length - 2; at += width) format(r, c + (at - offset - 4) / width, at)
    }
    return sheet
  })
}
