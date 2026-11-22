import type { CellAddress, CellObject } from 'xlsx'

export type SpreadsheetBorder = Partial<Record<'top' | 'right' | 'bottom' | 'left', string>>

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

export interface SpreadsheetCorner extends CellAddress {
  x: number
  y: number
}

export type SpreadsheetImageAnchor = {
  type: string
  data: Uint8Array<ArrayBuffer>
  from: SpreadsheetCorner
  offsets: 'pixels' | 'cell fractions'
} & ({ to: SpreadsheetCorner } | { size: { width: number; height: number } })

export interface SpreadsheetMetadata {
  formats: Map<CellObject, SpreadsheetStyle>
  defaultStyle: SpreadsheetStyle
  frozen: CellAddress
  gridlines: boolean
  images: SpreadsheetImageAnchor[]
  defaultRowHeight: number
  defaultColumnCharacters: number
}

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

export function borderLine(style: string, color = '#000'): string {
  return `${BORDER_LINES[style] ?? '1px solid'} ${color}`
}
