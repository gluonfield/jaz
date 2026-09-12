import { useEffect, useId, useRef, useState } from 'react'
import { BotEngine } from '@/lib/vendor/bloub/engine'
import { EXPRESSION_BY_ID } from '@/lib/vendor/bloub/expressions'
import type { StateId } from '@/lib/vendor/bloub/states'
import { audioLevel } from '@/lib/voice/audioLevel'

const restingFrame = new BotEngine(100).sample(0)

export function VoiceVisualizer({ analyser, active, reducedMotion, connecting = false, working = false, level = 0, size = 88 }: {
  analyser: AnalyserNode | null
  active: boolean
  reducedMotion: boolean
  connecting?: boolean
  working?: boolean
  level?: number
  size?: number
}) {
  const id = useId()
  const input = useRef({ analyser, active, connecting, working, level })
  const redraw = useRef(() => {})
  const [frame, setFrame] = useState({ pose: restingFrame, scale: 1, state: 'idle' as StateId })
  useEffect(() => {
    input.current = { analyser, active, connecting, working, level }
    if (reducedMotion) {
      redraw.current()
    }
  }, [analyser, active, connecting, working, level, reducedMotion])

  useEffect(() => {
    const engine = new BotEngine(100)
    let previous = 0
    let clock = 0
    let mode = ''
    let modeSince = 0
    let amplitude = 0
    let raf = 0
    let samples = new Uint8Array(0)
    const draw = (now: number) => {
      const dt = previous ? Math.min((now - previous) / 1000, 0.064) : 0
      previous = now
      clock += reducedMotion ? 0 : dt
      const { analyser, active, connecting, working, level } = input.current
      if (analyser && samples.length !== analyser.fftSize) {
        samples = new Uint8Array(analyser.fftSize)
      }
      const target = reducedMotion ? 0 : analyser ? audioLevel(analyser, samples) : level
      amplitude += (target - amplitude) * (1 - Math.exp(-dt / 0.09))
      const nextMode = connecting ? 'connecting' : !active ? 'muted' : working ? 'working' : 'ready'
      if (mode !== nextMode) {
        mode = nextMode
        modeSince = clock
        engine.setExpression(mode === 'muted' ? EXPRESSION_BY_ID.get('somnolent')! : null, clock - (reducedMotion ? 1 : 0))
      }
      const elapsed = clock - modeSince
      let state: StateId = 'idle'
      if (mode === 'connecting') {
        state = elapsed % 6 < 3.6 ? 'orbit' : 'hexagon'
      } else if (mode === 'working') {
        state = elapsed % 6 < 3 ? 'hexagon' : 'orbit'
      } else if (mode === 'ready' && (elapsed < 1.4 || amplitude > 0.08 || (engine.state === 'wide' && amplitude > 0.03))) {
        state = 'wide'
      }
      if (engine.state !== state) {
        engine.setState(state, clock - (reducedMotion ? 1 : 0))
      }
      setFrame({ pose: engine.sample(clock), scale: 1 + amplitude * 0.09, state })
      if (!reducedMotion) {
        raf = requestAnimationFrame(draw)
      }
    }
    redraw.current = () => draw(performance.now())
    redraw.current()
    return () => {
      cancelAnimationFrame(raf)
      redraw.current = () => {}
    }
  }, [reducedMotion])

  const { pose } = frame
  return (
    <svg width={size} height={size} viewBox="-160 -160 320 320" fill="currentColor" aria-hidden="true" data-avatar="bloub" data-state={frame.state}>
      <defs>
        <mask id={`${id}-body`} maskUnits="userSpaceOnUse" x="-160" y="-160" width="320" height="320">
          <path data-body d={pose.bodyPath} fill="white" />
          {pose.eyes.map((eye, i) => <path key={i} data-eye d={eye.d} transform={eye.matrix} opacity={eye.alpha} fill="black" />)}
        </mask>
        {pose.arcs.map((arc) => (
          <linearGradient key={arc.id} id={`${id}-${arc.id}`} gradientUnits="userSpaceOnUse" x1={arc.grad.x1} y1={arc.grad.y1} x2={arc.grad.x2} y2={arc.grad.y2}>
            {arc.grad.stops.map((color, i) => <stop key={i} offset={i / (arc.grad.stops.length - 1)} stopColor={color} />)}
          </linearGradient>
        ))}
      </defs>
      <g transform={`scale(${frame.scale})`}>
        <g fill="none" strokeLinecap="round">
          {pose.arcs.map((arc) => <path key={arc.id} d={arc.back} stroke={`url(#${id}-${arc.id})`} strokeWidth={arc.width} opacity={arc.opacity} />)}
        </g>
        <g opacity={pose.bodyAlpha}>
          <path d={pose.bodyPath} fill="var(--color-bg)" />
          <rect x="-160" y="-160" width="320" height="320" mask={`url(#${id}-body)`} />
        </g>
        <g fill="none" strokeLinecap="round">
          {pose.arcs.map((arc) => <path key={arc.id} d={arc.front} stroke={`url(#${id}-${arc.id})`} strokeWidth={arc.width} opacity={arc.opacity} />)}
        </g>
      </g>
    </svg>
  )
}
