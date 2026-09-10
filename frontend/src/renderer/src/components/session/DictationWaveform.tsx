import { useEffect, useRef } from 'react'
import { useReducedMotion } from 'motion/react'

const BAR_COUNT = 73
const BAR_INTERVAL = 80

export function DictationWaveform({ level }: { level: number }) {
  const trackRef = useRef<SVGGElement>(null)
  const currentLevel = useRef(level)
  const reducedMotion = useReducedMotion()

  useEffect(() => {
    currentLevel.current = level
  }, [level])

  useEffect(() => {
    const track = trackRef.current
    if (!track) {
      return
    }
    const bars = Array.from(track.children)
    let levels = Array<number>(BAR_COUNT).fill(0)
    let sampledAt = performance.now()
    let previousFrame = sampledAt
    let peak = 0.025
    let frame = 0

    const draw = (now: number) => {
      const elapsed = Math.max(0, now - sampledAt)
      const count = Math.min(BAR_COUNT, Math.floor(elapsed / BAR_INTERVAL))
      if (count > 0) {
        levels = [...levels.slice(count), ...Array<number>(count).fill(currentLevel.current)]
        sampledAt = now - elapsed % BAR_INTERVAL
      }
      const blend = 1 - Math.exp(-Math.max(0, now - previousFrame) / 120)
      peak += (Math.max(0.025, ...levels) - peak) * blend
      previousFrame = now
      const offset = reducedMotion ? 0 : (elapsed % BAR_INTERVAL) / BAR_INTERVAL * 6
      track.setAttribute('transform', `translate(${-offset} 0)`)
      for (let index = 0; index < BAR_COUNT; index++) {
        const strength = Math.min(1, Math.max(0, (levels[index] / peak - 0.12) / 0.88))
        const height = 3 + strength * 28
        bars[index].setAttribute('y', String((32 - height) / 2))
        bars[index].setAttribute('height', String(height))
        bars[index].setAttribute('opacity', String((0.2 + strength * 0.65) * (0.5 + index / 144)))
      }
      frame = requestAnimationFrame(draw)
    }
    frame = requestAnimationFrame(draw)
    return () => cancelAnimationFrame(frame)
  }, [reducedMotion])

  return (
    <svg className="h-4 w-full overflow-hidden text-ink" viewBox="0 0 432 32" preserveAspectRatio="none" aria-hidden>
      <g ref={trackRef}>
        {Array.from({ length: BAR_COUNT }, (_, index) => (
          <rect key={index} x={index * 6 + 1} y={14.5} width={3} height={3} rx={1.5} fill="currentColor" opacity={0.1} />
        ))}
      </g>
    </svg>
  )
}
