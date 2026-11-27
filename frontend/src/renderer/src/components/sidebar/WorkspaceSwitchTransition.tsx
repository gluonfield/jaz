import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { snapshotWindow } from '@/lib/domSnapshot'
import { PASSAGE_OPEN_MS, runPassage } from './workspaceSwitchPassage'

const FADE_MS = 140
// Time for the new workspace to draw itself before it is pictured.
const SETTLE_MS = 120
// The new workspace is not opened before its name has been readable.
const NAMED_MS = 1000
const NAME_FADE_MS = 140

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// The passage between workspaces: a frame of light round the window gathers onto
// the destination's name while the page dims, the name leaves, and the frame
// opens back out onto the new workspace with a nebula blooming round it.
// `name` is the workspace being switched to, null when no switch is under way.
export function WorkspaceSwitchTransition({ name }: { name: string | null }) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<{ name: string; phase: 'leaving' | 'landing' | 'done' } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const label = useRef<HTMLParagraphElement>(null)
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
        setScene((current) => current && { ...current, phase: 'landing' })
        await wait(NAME_FADE_MS)
        if (cancelled) return
        running.arrive(arrival)
        await wait(PASSAGE_OPEN_MS)
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
      canvas.current && label.current
        ? runPassage(canvas.current, picture, [label.current.offsetWidth, label.current.offsetHeight])
        : null,
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
