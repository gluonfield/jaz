import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { OPEN_MS } from '@/lib/frameLight'
import { runPassage } from './workspaceSwitchPassage'

// The new workspace is not opened before its name has been readable.
const NAMED_MS = 1150
const NAME_FADE_MS = 100

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// The passage between workspaces: a frame of light round the window gathers onto
// the destination's name while the page dims, then the name leaves as the frame
// bursts back out onto the new workspace with a nebula blooming round it.
// `name` is the workspace being switched to, null when no switch is under way.
// Reduced motion switches without it.
export function WorkspaceSwitchTransition({ name }: { name: string | null }) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<{ name: string; since: number; phase: 'leaving' | 'landing' } | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const label = useRef<HTMLParagraphElement>(null)
  const passage = useRef<ReturnType<typeof runPassage>>(null)

  const since = scene?.since
  useEffect(() => {
    if (since === undefined) return
    const running = runPassage(canvas.current!, [label.current!.offsetWidth, label.current!.offsetHeight])
    passage.current = running
    return () => {
      passage.current = null
      running?.stop()
    }
  }, [since])

  useEffect(() => {
    if (reduce) return
    if (name !== null) {
      setScene({ name, since: performance.now(), phase: 'leaving' })
      return
    }
    let cancelled = false
    void (async () => {
      const running = passage.current
      if (running) {
        await wait(NAMED_MS - (performance.now() - running.since))
        if (cancelled) return
        setScene((current) => current && { ...current, phase: 'landing' })
        running.arrive()
        await wait(OPEN_MS)
      }
      if (!cancelled) setScene(null)
    })()
    return () => {
      cancelled = true
    }
  }, [name, reduce])

  if (typeof document === 'undefined' || !scene) return null
  return createPortal(
    <div
      role="status"
      aria-label={`Switching to ${scene.name}`}
      style={{ zIndex: 'calc(var(--z-modal) + 2)' }}
      className="fixed inset-0 [-webkit-app-region:no-drag]"
    >
      <canvas ref={canvas} className="size-full" />
      <div className="absolute inset-0 grid place-items-center">
        <p
          ref={label}
          style={{ transitionDuration: `${NAME_FADE_MS}ms` }}
          className={`max-w-[60vw] truncate rounded-2xl bg-surface px-6 py-3 text-[20px] font-semibold tracking-[-0.02em] text-ink transition-opacity ease-out starting:opacity-0 ${
            scene.phase === 'leaving' ? 'delay-[300ms]' : 'opacity-0'
          }`}
        >
          {scene.name}
        </p>
      </div>
    </div>,
    document.body,
  )
}
