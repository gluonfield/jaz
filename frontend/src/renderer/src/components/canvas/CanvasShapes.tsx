import type { Shape } from './format'

// Each outline in a 100 by 100 box that stretches to the item. The first four
// are drawn with CSS on the board, where a stretched corner would show.
const PATHS: Record<Shape, string[]> = {
  rect: ['M1 1H99V99H1Z'],
  rounded: ['M16 1H84Q99 1 99 16V84Q99 99 84 99H16Q1 99 1 84V16Q1 1 16 1Z'],
  pill: ['M30 1H70C109 1 109 99 70 99H30C-9 99 -9 1 30 1Z'],
  ellipse: ['M50 1A49 49 0 1 0 50 99A49 49 0 1 0 50 1Z'],
  diamond: ['M50 1L99 50L50 99L1 50Z'],
  hexagon: ['M14 1H86L99 50L86 99H14L1 50Z'],
  parallelogram: ['M16 1H99L84 99H1Z'],
  document: ['M1 1H99V86C74 72 50 100 1 88Z'],
  database: ['M1 14C1 -3 99 -3 99 14V86C99 103 1 103 1 86Z', 'M1 14C1 31 99 31 99 14'],
  queue: ['M12 1H88C103 1 103 99 88 99H12C-3 99 -3 1 12 1Z', 'M88 1C73 1 73 99 88 99'],
  cloud: ['M26 90C4 90 -2 62 16 52C8 26 38 10 52 26C64 6 96 16 88 46C104 54 100 90 78 90Z'],
  person: ['M50 3A17 17 0 1 0 50 37A17 17 0 1 0 50 3Z', 'M6 99C6 54 94 54 94 99Z'],
}

export const CSS_SHAPES: readonly Shape[] = ['rect', 'rounded', 'pill', 'ellipse']

export function ShapeOutline({ shape }: { shape: Shape }) {
  return (
    <svg className="canvas-shape" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden>
      {PATHS[shape].map((d) => <path key={d} d={d} vectorEffect="non-scaling-stroke" />)}
    </svg>
  )
}

// The outline of an item about to be put down, in the box it will take. A note
// or a table is a plain box.
export function Ghost({ shape = 'rect', ...box }: { shape?: Shape; left: number; top: number; width: number; height: number }) {
  return (
    <span className="canvas-ghost" style={box}>
      <ShapeOutline shape={shape} />
    </span>
  )
}
