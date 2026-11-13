import type { CanvasItem } from '@/lib/api/canvas'
import { bounds, type Rect } from './board'

export const ALIGNMENTS = ['left', 'center', 'right', 'top', 'middle', 'bottom'] as const
export type Alignment = (typeof ALIGNMENTS)[number]
type Placed = Rect & { id: string }

// The moves that line the items up along one edge or centre line of the box
// around them.
export function alignItems(items: Placed[], to: Alignment): Partial<CanvasItem>[] {
  const box = bounds(items)
  const moved: Record<Alignment, (item: Placed) => Partial<CanvasItem>> = {
    left: () => ({ x: box.x }),
    center: (item) => ({ x: box.x + (box.width - item.width) / 2 }),
    right: (item) => ({ x: box.x + box.width - item.width }),
    top: () => ({ y: box.y }),
    middle: (item) => ({ y: box.y + (box.height - item.height) / 2 }),
    bottom: (item) => ({ y: box.y + box.height - item.height }),
  }
  return items.map((item) => ({ id: item.id, ...Object.fromEntries(Object.entries(moved[to](item)).map(([key, value]) => [key, Math.round(value as number)])) }))
}

// The picked items get a `z` above or below every other item, keeping their
// own order. Nothing changes when they are already there.
export function layerItems(items: CanvasItem[], picked: ReadonlySet<string>, to: 'front' | 'back'): Partial<CanvasItem>[] {
  const z = (item: CanvasItem) => (typeof item.z === 'number' ? item.z : 0)
  const rest = items.filter((item) => !picked.has(item.id)).map(z)
  const mine = items.filter((item) => picked.has(item.id))
  if (!rest.length || !mine.length) return []
  const done = to === 'front' ? Math.min(...mine.map(z)) > Math.max(...rest) : Math.max(...mine.map(z)) < Math.min(...rest)
  if (done) return []
  const start = to === 'front' ? Math.max(...rest) + 1 : Math.min(...rest) - mine.length
  return mine.map((item, index) => ({ id: item.id, z: start + index }))
}
