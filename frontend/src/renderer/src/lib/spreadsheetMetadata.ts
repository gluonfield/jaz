import { parser as createXMLParser } from 'sax'
import { legacySpreadsheetPanes } from '@/lib/spreadsheetLegacy'
import { utils, type CellObject, type WorkBook, type WorkSheet } from 'xlsx'

export interface SpreadsheetStyle {
  bold?: boolean
  italic?: boolean
  fontSize?: number
  fontFamily?: string
  color?: string
  fill?: string
  horizontal?: 'left' | 'center' | 'right' | 'justify'
  vertical?: 'top' | 'center' | 'bottom'
  wrap?: boolean
  border?: SpreadsheetBorder
}

type SpreadsheetBorder = Partial<Record<'top' | 'right' | 'bottom' | 'left', string>>

const BORDER_LINES: Record<string, string> = {
  medium: '2px solid',
  thick: '3px solid',
  double: '3px double',
  dotted: '1px dotted',
  hair: '1px dotted',
  dashed: '1px dashed',
  dashDot: '1px dashed',
  dashDotDot: '1px dashed',
  mediumDashed: '2px dashed',
  mediumDashDot: '2px dashed',
  mediumDashDotDot: '2px dashed',
  slantDashDot: '2px dashed',
}

interface Color {
  rgb?: string
}

interface RawStyle {
  fontId?: number
  fillId?: number
  borderId?: number
  alignment?: { horizontal?: string; vertical?: string; wrapText?: boolean }
}

interface StyledWorkbook extends WorkBook {
  files?: Record<string, { content?: Uint8Array }>
  Styles?: {
    Fonts?: { bold?: boolean; italic?: boolean; sz?: number; name?: string; color?: Color }[]
    Fills?: { patternType?: string; fgColor?: Color }[]
    CellXf?: RawStyle[]
  }
}

export interface SpreadsheetMetadata {
  formats: Map<CellObject, SpreadsheetStyle>
  defaultStyle: SpreadsheetStyle
  frozen: { r: number; c: number }
  defaultRowHeight: number
  defaultColumnCharacters: number
}

function color(value?: Color) {
  return value?.rgb && /^(?:[\da-f]{2})?[\da-f]{6}$/i.test(value.rgb) ? `#${value.rgb.slice(-6)}` : undefined
}

function visit(xml: string, onTag: (name: string, attributes: Record<string, string>) => void) {
  if (!xml) return
  const parser = createXMLParser(true, { xmlns: true })
  parser.onopentag = (tag) => onTag(tag.local, Object.fromEntries(Object.values(tag.attributes).map((attribute) => [attribute.local, attribute.value])))
  parser.write(xml).close()
}

function spreadsheetBorders(styles: string): SpreadsheetBorder[] {
  const borders: SpreadsheetBorder[] = []
  let side: { name: keyof SpreadsheetBorder; line: string } | undefined
  visit(styles, (name, attrs) => {
    const border = borders.at(-1)
    if (name === 'border') borders.push({})
    else if (border && attrs.style && (name === 'top' || name === 'right' || name === 'bottom' || name === 'left')) {
      side = { name, line: BORDER_LINES[attrs.style] ?? '1px solid' }
      border[name] = `${side.line} #000`
    } else {
      if (border && side && name === 'color') border[side.name] = `${side.line} ${color(attrs) ?? '#000'}`
      side = undefined
    }
  })
  return borders
}

export function spreadsheetMetadata(workbook: WorkBook): Map<WorkSheet, SpreadsheetMetadata> {
  const book = workbook as StyledWorkbook
  const xml = (path: string) => {
    const content = book.files?.[path]?.content
    return content ? new TextDecoder().decode(content) : ''
  }
  const borders = spreadsheetBorders(xml('xl/styles.xml'))
  const styles = (book.Styles?.CellXf ?? []).map((style): SpreadsheetStyle => {
    const font = book.Styles?.Fonts?.[style.fontId ?? 0]
    const fill = book.Styles?.Fills?.[style.fillId ?? 0]
    const alignment = style.alignment
    return {
      bold: Boolean(font?.bold),
      italic: Boolean(font?.italic),
      fontSize: font?.sz ? font.sz * 4 / 3 : undefined,
      fontFamily: font?.name,
      color: color(font?.color),
      fill: fill?.patternType === 'solid' ? color(fill.fgColor) : undefined,
      horizontal: ['left', 'center', 'right', 'justify'].includes(alignment?.horizontal ?? '') ? alignment?.horizontal as SpreadsheetStyle['horizontal'] : undefined,
      vertical: alignment?.vertical === 'center' || alignment?.vertical === 'top' ? alignment.vertical : 'bottom',
      wrap: Boolean(alignment?.wrapText),
      border: borders[style.borderId ?? 0],
    }
  })
  const relationships = new Map<string, string>()
  visit(xml('xl/_rels/workbook.xml.rels'), (name, attrs) => {
    if (name === 'Relationship' && attrs.TargetMode !== 'External') {
      const parts: string[] = []
      for (const part of (attrs.Target.startsWith('/') ? attrs.Target.slice(1) : `xl/${attrs.Target}`).split('/')) {
        if (part === '..') parts.pop()
        else if (part !== '.') parts.push(part)
      }
      relationships.set(attrs.Id, parts.join('/'))
    }
  })
  const paths = new Map<string, string>()
  visit(xml('xl/workbook.xml'), (name, attrs) => {
    if (name === 'sheet') paths.set(attrs.name, relationships.get(attrs.id) ?? '')
  })
  const result = new Map<WorkSheet, SpreadsheetMetadata>()
  const legacyPanes = legacySpreadsheetPanes(book)
  for (const name of book.SheetNames) {
    const sheet = book.Sheets[name]
    const metadata: SpreadsheetMetadata = { formats: new Map(), defaultStyle: styles[0] ?? {}, frozen: legacyPanes[book.SheetNames.indexOf(name)] ?? { r: 0, c: 0 }, defaultRowHeight: 20, defaultColumnCharacters: 9.140625 }
    visit(xml(paths.get(name) ?? ''), (tag, attrs) => {
      if (tag === 'pane' && (attrs.state === 'frozen' || attrs.state === 'frozenSplit')) {
        metadata.frozen = { r: Math.max(0, Math.floor(Number(attrs.ySplit) || 0)), c: Math.max(0, Math.floor(Number(attrs.xSplit) || 0)) }
      }
      if (tag === 'sheetFormatPr') {
        if (Number(attrs.defaultRowHeight) > 0) metadata.defaultRowHeight = Number(attrs.defaultRowHeight) * 4 / 3
        if (Number(attrs.defaultColWidth) > 0) metadata.defaultColumnCharacters = Number(attrs.defaultColWidth)
      }
      if (tag === 'c' && attrs.r && attrs.s) {
        const point = utils.decode_cell(attrs.r)
        const cell = sheet['!data']?.[point.r]?.[point.c]
        const style = styles[Number(attrs.s)]
        if (cell && style) metadata.formats.set(cell, style)
      }
    })
    if (!paths.has(name)) {
      for (const row of sheet['!data'] ?? []) {
        for (const cell of row ?? []) {
          const fill = cell?.s as { patternType?: string; fgColor?: Color } | undefined
          if (cell && fill?.patternType === 'solid') metadata.formats.set(cell, { fill: color(fill.fgColor) })
        }
      }
    }
    result.set(sheet, metadata)
  }
  return result
}
