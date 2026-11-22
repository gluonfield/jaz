import type { SpreadsheetCorner, SpreadsheetImageAnchor } from '@/lib/spreadsheetFormat'
import { relationships, visit } from '@/lib/spreadsheetXML'

const EMU_PER_PIXEL = 9525
const IMAGE_TYPES: Record<string, string> = { png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', bmp: 'image/bmp' }
const CORNER_FIELDS: Record<string, { field: keyof SpreadsheetCorner; scale: number }> = {
  col: { field: 'c', scale: 1 },
  row: { field: 'r', scale: 1 },
  colOff: { field: 'x', scale: EMU_PER_PIXEL },
  rowOff: { field: 'y', scale: EMU_PER_PIXEL },
}

export function sheetImages(read: (path: string) => string, bytes: (path: string) => Uint8Array | undefined, sheet: string): SpreadsheetImageAnchor[] {
  const images: SpreadsheetImageAnchor[] = []
  for (const { path, type } of relationships(read, sheet).values()) {
    if (!type.endsWith('/drawing')) continue
    const media = relationships(read, path)
    let anchor: { from?: SpreadsheetCorner; to?: SpreadsheetCorner; size?: { width: number; height: number }; embed?: string } = {}
    let corner: SpreadsheetCorner | undefined
    const finish = () => {
      const target = media.get(anchor.embed ?? '')?.path ?? ''
      const image = IMAGE_TYPES[target.slice(target.lastIndexOf('.') + 1).toLowerCase()]
      const data = bytes(target)
      const extent = anchor.to ? { to: anchor.to } : anchor.size && { size: anchor.size }
      if (anchor.from && extent && image && data) images.push({ type: image, data: new Uint8Array(data), from: anchor.from, offsets: 'pixels', ...extent })
      anchor = {}
    }
    visit(read(path), (name, attrs) => {
      if (name === 'twoCellAnchor' || name === 'oneCellAnchor' || name === 'absoluteAnchor') finish()
      else if (name === 'from' || name === 'to') corner = anchor[name] = { r: 0, c: 0, x: 0, y: 0 }
      else if (name === 'ext' && attrs.cx && !anchor.size) anchor.size = { width: Number(attrs.cx) / EMU_PER_PIXEL, height: Number(attrs.cy) / EMU_PER_PIXEL }
      else if (name === 'blip' && attrs.embed) anchor.embed = attrs.embed
    }, (name, text) => {
      const target = CORNER_FIELDS[name]
      if (target && corner) corner[target.field] = Number(text) / target.scale
    })
    finish()
  }
  return images
}
