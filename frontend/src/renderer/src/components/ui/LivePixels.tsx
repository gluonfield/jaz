// Tiny square-pixel "working" indicator: a 5x5 dither grid whose pixels
// assemble into a cycling sequence of glyphs (ring, diamond, plus, cross,
// square, core), each in its own brand/rainbow color. Per-pixel staggered
// easing makes every glyph gather and scatter rather than crossfade. One
// interval mutates styles directly (no re-render); under
// prefers-reduced-motion or jaz-no-effects it renders a single static glyph.
// The resting floor is theme-aware via --live-pixel-rest so the mosaic
// stays visible on light surfaces.

import { useEffect, useRef } from 'react'

const GRID = 5
const STEP = 3 // 2px square + 1px gap
const TICK_MS = 60
const SHAPE_MS = 1100
const EASE = 0.22

const SHAPES = [
  ['.###.', '#...#', '#...#', '#...#', '.###.'], // ring
  ['..#..', '.#.#.', '#...#', '.#.#.', '..#..'], // diamond
  ['..#..', '..#..', '.###.', '..#..', '..#..'], // plus
  ['#...#', '.#.#.', '..#..', '.#.#.', '#...#'], // cross
  ['#####', '#...#', '#...#', '#...#', '#####'], // square
  ['.....', '.###.', '.###.', '.###.', '.....'], // core
].map((rows) => rows.join('').split('').map((cell) => cell === '#'))

const COLORS = [
  'var(--color-running)',
  'var(--color-rainbow-1)',
  'var(--color-rainbow-2)',
  'var(--color-rainbow-3)',
  'var(--color-rainbow-4)',
  'var(--color-rainbow-5)',
]

export function LivePixels({ className }: { className?: string }) {
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const root = ref.current
    if (!root) return
    const cells = Array.from(root.children) as HTMLElement[]
    const rest = Number.parseFloat(getComputedStyle(root).getPropertyValue('--live-pixel-rest')) || 0.12
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')

    const current = cells.map(() => rest)
    const target = cells.map(() => rest)
    const delay = cells.map(() => 0)

    const applyShape = (index: number) => {
      root.style.color = COLORS[index % COLORS.length]
      SHAPES[index].forEach((on, i) => {
        target[i] = on ? 1 : rest
        delay[i] = Math.floor(Math.random() * 5)
      })
    }

    if (reduced.matches || document.documentElement.classList.contains('jaz-no-effects')) {
      root.style.color = COLORS[0]
      SHAPES[0].forEach((on, i) => {
        cells[i].style.opacity = on ? '0.85' : String(rest)
      })
      return
    }

    let shape = 0
    let elapsed = 0
    applyShape(shape)
    const timer = window.setInterval(() => {
      if (reduced.matches || document.documentElement.classList.contains('jaz-no-effects')) return
      elapsed += TICK_MS
      if (elapsed >= SHAPE_MS) {
        elapsed = 0
        shape = (shape + 1) % SHAPES.length
        applyShape(shape)
      }
      cells.forEach((cell, i) => {
        if (delay[i] > 0) {
          delay[i] -= 1
        } else {
          current[i] += (target[i] - current[i]) * EASE
        }
        cell.style.opacity = current[i].toFixed(3)
      })
    }, TICK_MS)
    return () => window.clearInterval(timer)
  }, [])

  return (
    <span ref={ref} className={`live-pixels ${className ?? ''}`} aria-hidden>
      {Array.from({ length: GRID * GRID }, (_, i) => (
        <span
          key={i}
          className="live-pixel"
          style={{ left: (i % GRID) * STEP, top: Math.floor(i / GRID) * STEP }}
        />
      ))}
    </span>
  )
}
