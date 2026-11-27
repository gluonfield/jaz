import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { snapshotWindow } from '@/lib/domSnapshot'
import { runSand, SAND_ARRIVE_MS } from './workspaceSwitchSand'

const FADE_MS = 140
// Time for the new workspace to draw itself before it is pictured.
const SETTLE_MS = 120
// The new workspace is not blown in before its name has been readable.
const NAMED_MS = 650

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// The passage between workspaces: the window steps back, turns to sand and is blown away,
// then the new workspace is blown in and settles into place, with its name
// held in the middle while the window is empty. `name` is the
// workspace being switched to, null when no switch is under way.
export function WorkspaceSwitchTransition({ name }: { name: string | null }) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<{ name: string; phase: 'leaving' | 'arriving' | 'done' } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const sand = useRef<Promise<ReturnType<typeof runSand>>>(Promise.resolve(null))
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
      const running = await sand.current
      if (running) {
        await wait(Math.max(SETTLE_MS, NAMED_MS - (performance.now() - started.current)))
        const arrival = await picture()
        if (cancelled) return
        running.arrive(arrival)
        setScene((current) => current && { ...current, phase: 'arriving' })
        await wait(SAND_ARRIVE_MS)
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
      canvas.current ? runSand(canvas.current, picture) : null,
    )
    sand.current = running
    return () => {
      sand.current = Promise.resolve(null)
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
      <p
        className={`absolute inset-0 grid place-items-center text-[28px] font-semibold tracking-[-0.02em] text-ink transition-[opacity,filter,scale] duration-300 ease-out starting:scale-95 starting:opacity-0 starting:blur-md ${
          scene.phase === 'leaving' ? 'delay-[350ms]' : 'scale-105 opacity-0 blur-md delay-200'
        }`}
      >
        {scene.name}
      </p>
    </div>,
    document.body,
  )
}
