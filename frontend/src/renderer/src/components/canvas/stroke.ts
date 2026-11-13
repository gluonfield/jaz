import type { CanvasItem } from '@/lib/api/canvas'
import type { Rect } from './board'

export type Point = [x: number, y: number]
export const WEIGHTS = [2, 4, 8] as const
export interface Pen {
  color?: string
  weight: number
  highlight: boolean
}

// A smooth line through the points: each one bends the curve towards the
// middle of the next pair. A single point is a dot.
export function strokePath(points: Point[]): string {
  if (points.length < 2) return points.length ? `M${points[0][0]} ${points[0][1]}h0.01` : ''
  const middle = (a: Point, b: Point) => `${(a[0] + b[0]) / 2} ${(a[1] + b[1]) / 2}`
  const bends = points.slice(1, -1).map((point, index) => `Q${point[0]} ${point[1]} ${middle(point, points[index + 2])}`)
  const last = points.at(-1)!
  return `M${points[0][0]} ${points[0][1]}L${middle(points[0], points[1])}${bends.join('')}L${last[0]} ${last[1]}`
}

// The drawn line as an item: the box around it, and its points inside that
// box, leaving out those closer than `step` to the one before.
export function strokeItem(drawn: Point[], pen: Pen, step = 1.5): Partial<CanvasItem> & Rect {
  const kept = drawn.filter((point, index) => index === 0 || index === drawn.length - 1 || Math.hypot(point[0] - drawn[index - 1][0], point[1] - drawn[index - 1][1]) >= step)
  const x = Math.floor(Math.min(...kept.map((point) => point[0])))
  const y = Math.floor(Math.min(...kept.map((point) => point[1])))
  const round = (value: number) => Math.round(value * 10) / 10
  return {
    type: 'stroke',
    x,
    y,
    width: Math.max(1, Math.ceil(Math.max(...kept.map((point) => point[0])) - x)),
    height: Math.max(1, Math.ceil(Math.max(...kept.map((point) => point[1])) - y)),
    points: kept.map((point) => [round(point[0] - x), round(point[1] - y)]),
    ...(pen.color ? { color: pen.color } : {}),
    ...(pen.weight === 4 ? {} : { weight: pen.weight }),
    ...(pen.highlight ? { highlight: true } : {}),
  }
}

export function strokePoints(item: CanvasItem): Point[] {
  const points = Array.isArray(item.points) ? item.points : []
  return points.filter((point): point is Point => Array.isArray(point) && typeof point[0] === 'number' && typeof point[1] === 'number')
}

export function penOf(item: CanvasItem): Pen {
  return {
    color: typeof item.color === 'string' ? item.color : undefined,
    weight: typeof item.weight === 'number' && item.weight > 0 ? item.weight : 4,
    highlight: item.highlight === true,
  }
}
