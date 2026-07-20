// Tiny square-pixel "working" indicator: a 3x3 dither grid where pixels
// ignite at random positions in random brand/rainbow colors and decay back
// to a faint resting mosaic — living pixel noise, not a spinner. One
// interval mutates styles directly (no re-render); motion stops under
// prefers-reduced-motion or jaz-no-effects, leaving a static dim grid.
// The resting floor is theme-aware via --live-pixel-rest so the mosaic
// stays visible on light surfaces.

import { useEffect, useRef } from 'react'

const GRID = 3
const STEP = 5 // 3px square + 2px gap
const TICK_MS = 120
const DECAY = 0.72
const IGNITE = 0.07
const SPARKS = [
  'currentColor',
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
    const energy = new Array<number>(cells.length).fill(0)
    const rest = Number.parseFloat(getComputedStyle(root).getPropertyValue('--live-pixel-rest')) || 0.12
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')
    const timer = window.setInterval(() => {
      if (reduced.matches || document.documentElement.classList.contains('jaz-no-effects')) return
      cells.forEach((cell, i) => {
        if (Math.random() < IGNITE) {
          energy[i] = 1
          cell.style.background = SPARKS[Math.floor(Math.random() * SPARKS.length)]
        } else {
          energy[i] *= DECAY
        }
        cell.style.opacity = (rest + (1 - rest) * energy[i]).toFixed(3)
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
