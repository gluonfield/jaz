import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { snapshotWindow } from '@/lib/domSnapshot'
import { runSand, SAND_ARRIVE_MS } from './workspaceSwitchSand'

const FADE_MS = 180
// Time for the new workspace to draw itself before it is pictured.
const SETTLE_MS = 120

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// The passage between workspaces: the window turns to sand and is blown away,
// then the new workspace is blown in and settles into place. `name` is the
// workspace being switched to, null when no switch is under way.
export function WorkspaceSwitchTransition({ name }: { name: string | null }) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<{ name: string; leaving: boolean } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const sand = useRef<Promise<ReturnType<typeof runSand>>>(Promise.resolve(null))

  useEffect(() => {
    if (name !== null) {
      setScene({ name, leaving: false })
      return
    }
    let cancelled = false
    void (async () => {
      const running = await sand.current
      if (running) {
        await wait(SETTLE_MS)
        const picture = await snapshotWindow(window.devicePixelRatio, root.current)
        if (cancelled) return
        running.arrive(picture)
        await wait(SAND_ARRIVE_MS)
      }
      if (cancelled) return
      setScene((current) => current && { ...current, leaving: true })
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
    const running = snapshotWindow(window.devicePixelRatio, root.current).then((picture) =>
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
      className={`fixed inset-0 transition-opacity [-webkit-app-region:no-drag] ${scene.leaving ? 'opacity-0' : ''}`}
    >
      <canvas ref={canvas} className="size-full" />
    </div>,
    document.body,
  )
}
