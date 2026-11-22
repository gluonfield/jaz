import { sheetImages } from '@/lib/spreadsheetDrawings'
import { borderLine, type SpreadsheetBorder, type SpreadsheetMetadata, type SpreadsheetStyle } from '@/lib/spreadsheetFormat'
import { legacySpreadsheet } from '@/lib/spreadsheetLegacy'
import { relationships, visit } from '@/lib/spreadsheetXML'
import { utils, type WorkBook, type WorkSheet } from 'xlsx'

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

function color(value?: Color) {
  return value?.rgb && /^(?:[\da-f]{2})?[\da-f]{6}$/i.test(value.rgb) ? `#${value.rgb.slice(-6)}` : undefined
}

function spreadsheetBorders(styles: string): SpreadsheetBorder[] {
  const borders: SpreadsheetBorder[] = []
  let side: { name: keyof SpreadsheetBorder; style: string } | undefined
  visit(styles, (name, attrs) => {
    const border = borders.at(-1)
    if (name === 'border') borders.push({})
    else if (border && attrs.style && (name === 'top' || name === 'right' || name === 'bottom' || name === 'left')) {
      side = { name, style: attrs.style }
      border[name] = borderLine(attrs.style)
    } else {
      if (border && side && name === 'color') border[side.name] = borderLine(side.style, color(attrs))
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
  const parts = relationships(xml, 'xl/workbook.xml')
  const paths = new Map<string, string>()
  visit(xml('xl/workbook.xml'), (name, attrs) => {
    if (name === 'sheet') paths.set(attrs.name, parts.get(attrs.id)?.path ?? '')
  })
  const result = new Map<WorkSheet, SpreadsheetMetadata>()
  const legacy = legacySpreadsheet(book)
  for (const name of book.SheetNames) {
    const sheet = book.Sheets[name]
    const path = paths.get(name) ?? ''
    const metadata: SpreadsheetMetadata = { formats: new Map(), defaultStyle: styles[0] ?? {}, frozen: { r: 0, c: 0 }, gridlines: true, images: sheetImages(xml, (path) => book.files?.[path]?.content, path), defaultRowHeight: 20, defaultColumnCharacters: 9.140625, ...legacy[book.SheetNames.indexOf(name)] }
    visit(xml(path), (tag, attrs) => {
      if (tag === 'sheetView' && attrs.showGridLines === '0') metadata.gridlines = false
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
    if (!path && !metadata.formats.size) {
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
