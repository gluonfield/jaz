import type { CanvasItem } from '@/lib/api/canvas'
import { contains, overlap, type Rect } from './board'

// An item as the page drew it: where it is saved, at the size it measured.
export interface Drawn extends Rect {
  id: string
  type?: string
  place?: unknown
  // Below 1 when the content was shrunk to fit its box.
  scale?: number
}

const SIDES = ['below', 'right', 'above', 'left'] as const
const ALIGN = { start: 0, center: 0.5, end: 1 }
const MOST = 30

interface Place {
  side: (typeof SIDES)[number]
  anchor: string
  gap: number
  align: number
}

function placeOf(value: unknown): Place | undefined {
  if (typeof value !== 'object' || value === null) return undefined
  const fields = value as Record<string, unknown>
  const side = SIDES.find((name) => typeof fields[name] === 'string')
  return side && {
    side,
    anchor: fields[side] as string,
    gap: typeof fields.gap === 'number' ? fields.gap : 40,
    align: ALIGN[fields.align as keyof typeof ALIGN] ?? 0,
  }
}

function beside(anchor: Rect, own: Rect, { side, gap, align }: Place): { x: number; y: number } {
  const across = side === 'below' || side === 'above'
  return {
    x: Math.round(across ? anchor.x + (anchor.width - own.width) * align : side === 'right' ? anchor.x + anchor.width + gap : anchor.x - gap - own.width),
    y: Math.round(!across ? anchor.y + (anchor.height - own.height) * align : side === 'below' ? anchor.y + anchor.height + gap : anchor.y - gap - own.height),
  }
}

// Items that ask to sit beside another get their place from the measured
// sizes, an anchor before whatever hangs off it. Those that cannot be placed
// are named in `stuck`.
export function resolvePlaces(items: Drawn[]): { moves: Partial<CanvasItem>[]; stuck: string[] } {
  const rects = new Map<string, Rect>(items.map((item) => [item.id, item]))
  let waiting = items.flatMap((item) => {
    const place = placeOf(item.place)
    return place ? [{ item, place }] : []
  })
  const moves: Partial<CanvasItem>[] = []
  for (let moved = true; moved;) {
    moved = false
    for (const entry of waiting) {
      const anchor = rects.get(entry.place.anchor)
      if (!anchor || waiting.some((other) => other.item.id === entry.place.anchor)) continue
      const at = beside(anchor, entry.item, entry.place)
      rects.set(entry.item.id, { ...entry.item, ...at })
      moves.push({ id: entry.item.id, ...at, place: null })
      waiting = waiting.filter((other) => other !== entry)
      moved = true
    }
  }
  return {
    moves,
    stuck: waiting.map(({ item, place }) => (rects.has(place.anchor)
      ? `\`${item.id}\` is placed against \`${place.anchor}\`, which waits on a place itself`
      : `\`${item.id}\` is placed against \`${place.anchor}\`, which is not on this page`)),
  }
}

// What a reader would trip over: items on top of each other, items across a
// section's edge, and content shrunk to fit its box.
export function layoutIssues(items: Drawn[]): string[] {
  const issues: string[] = []
  const solid = items.filter((item) => item.type !== 'group' && item.type !== 'stroke')
  solid.forEach((a, index) => {
    for (const b of solid.slice(index + 1)) {
      const [width, height] = overlap(a, b)
      if (width > 2 && height > 2) issues.push(`\`${a.id}\` overlaps \`${b.id}\` by ${Math.round(width)} x ${Math.round(height)} px`)
    }
  })
  for (const section of items.filter((item) => item.type === 'group')) {
    for (const item of items) {
      const [width, height] = overlap(section, item)
      if (item !== section && item.type !== 'stroke' && width > 2 && height > 2 && !contains(section, item) && !contains(item, section)) {
        issues.push(`\`${item.id}\` crosses the edge of section \`${section.id}\``)
      }
    }
  }
  for (const item of items) {
    if (item.scale !== undefined && item.scale < 0.98) issues.push(`\`${item.id}\` is shrunk to ${Math.round(item.scale * 100)}% to fit its box`)
  }
  return issues.length > MOST ? [...issues.slice(0, MOST), `and ${issues.length - MOST} more`] : issues
}
