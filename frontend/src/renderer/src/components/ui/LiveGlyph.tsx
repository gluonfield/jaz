// Tiny glyph-based "working" indicator: a single dingbat mark that blooms
// open and closed through a starburst sequence, stepping through the
// brand/rainbow ramp as it goes, with a soft glow at peak bloom. One
// interval swaps text content directly (no re-render); under
// prefers-reduced-motion or jaz-no-effects it renders a single static mark.

import { useEffect, useRef } from 'react'

const GLYPHS = ['✦', '✧', '✶', '✷', '✸', '✹', '✺', '✹', '✸', '✷', '✶', '✧']
const PEAK = new Set(['✸', '✹', '✺'])
const COLORS = [
  'var(--color-running)',
  'var(--color-rainbow-1)',
  'var(--color-rainbow-2)',
  'var(--color-rainbow-3)',
  'var(--color-rainbow-4)',
  'var(--color-rainbow-5)',
]
const STEP_MS = 120

export function LiveGlyph({ className }: { className?: string }) {
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const root = ref.current
    if (!root) return
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')
    const still = () => reduced.matches || document.documentElement.classList.contains('jaz-no-effects')

    if (still()) {
      root.textContent = GLYPHS[0]
      root.style.color = COLORS[0]
      return
    }

    let step = 0
    const timer = window.setInterval(() => {
      if (still()) return
      step += 1
      const glyph = GLYPHS[step % GLYPHS.length]
      root.textContent = glyph
      root.style.color = COLORS[step % COLORS.length]
      root.classList.toggle('live-glyph-peak', PEAK.has(glyph))
    }, STEP_MS)
    return () => window.clearInterval(timer)
  }, [])

  return (
    <span ref={ref} className={`live-glyph ${className ?? ''}`} aria-hidden>
      {GLYPHS[0]}
    </span>
  )
}
