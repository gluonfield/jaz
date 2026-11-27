import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { snapshotWindow } from '@/lib/domSnapshot'
import { PASSAGE_EXPAND_MS, PASSAGE_SWAP_MS, runPassage } from './workspaceSwitchPassage'

const FADE_MS = 140
// Time for the new workspace to draw itself before it is pictured.
const SETTLE_MS = 120
// The new workspace is not blown in before its name has been readable.
const NAMED_MS = 900
const NAME_FADE_MS = 160

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// The passage between workspaces: light runs around the window's rim, the window
// shrinks to a card in front of a nebula with the destination's name on it, a
// band of light turns it into the new workspace, and it grows back. The name
// leaves just before the window grows. `name` is the workspace being switched
// to, null when no switch is under way.
export function WorkspaceSwitchTransition({ name }: { name: string | null }) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<{ name: string; phase: 'leaving' | 'landing' | 'done' } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const passage = useRef<Promise<ReturnType<typeof runPassage>>>(Promise.resolve(null))
  const started = useRef(0)

  // The window without this overlay or the menu that is closing behind the switch.
  const picture = () =>
    snapshotWindow(window.devicePixelRatio, (element) => element === root.current || element.hasAttribute('data-escape-surface'))

  useEffect(() => {
    if (name !== null) {
      started.current = performance.now()
      setScene({ name, phase: 'leaving' })
      return
    }
    let cancelled = false
    void (async () => {
      const running = await passage.current
      if (running) {
        await wait(Math.max(SETTLE_MS, NAMED_MS - (performance.now() - started.current)))
        const arrival = await picture()
        if (cancelled) return
        running.arrive(arrival)
        await wait(PASSAGE_SWAP_MS - NAME_FADE_MS)
        if (cancelled) return
        setScene((current) => current && { ...current, phase: 'landing' })
        await wait(PASSAGE_EXPAND_MS + NAME_FADE_MS)
      }
      if (cancelled) return
      setScene((current) => current && { ...current, phase: 'done' })
      await wait(FADE_MS)
      if (!cancelled) setScene(null)
    })()
    return () => {
      cancelled = true
    }
  }, [name])

  const shown = scene !== null
  useEffect(() => {
    if (!shown || reduce) return
    const running = picture().then((picture) =>
      canvas.current ? runPassage(canvas.current, picture) : null,
    )
    passage.current = running
    return () => {
      passage.current = Promise.resolve(null)
      void running.then((started) => started?.stop())
    }
  }, [shown, reduce])

  if (typeof document === 'undefined' || !scene) return null
  return createPortal(
    <div
      ref={root}
      role="status"
      aria-label={`Switching to ${scene.name}`}
      style={{ zIndex: 'calc(var(--z-modal) + 2)', transitionDuration: `${FADE_MS}ms` }}
      className={`fixed inset-0 transition-opacity [-webkit-app-region:no-drag] ${scene.phase === 'done' ? 'opacity-0' : ''}`}
    >
      <canvas ref={canvas} className="size-full" />
      <div className="absolute inset-0 grid place-items-center">
        <p
          style={{ transitionDuration: `${NAME_FADE_MS}ms` }}
          className={`max-w-[60vw] truncate rounded-2xl border border-border bg-surface px-6 py-3.5 text-[22px] font-semibold tracking-[-0.02em] text-ink shadow-[0_12px_40px_rgb(0_0_0/0.18)] transition-[opacity,scale] ease-out starting:scale-90 starting:opacity-0 ${
            scene.phase === 'leaving' ? 'delay-[350ms]' : 'scale-95 opacity-0'
          }`}
        >
          {scene.name}
        </p>
      </div>
    </div>,
    document.body,
  )
}
