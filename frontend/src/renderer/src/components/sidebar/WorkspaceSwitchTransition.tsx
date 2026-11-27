import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import type { CloudWorkspaceSummary } from '@/lib/cloudAccount'
import { workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'
import { WorkspaceMark } from './WorkspaceMark'
import { LIQUID_LEAVE_MS, runLiquid } from './workspaceSwitchLiquid'

// Long enough to read as leaving one place for another, however fast the switch itself is.
const MIN_MS = 1700
const EASE = [0.16, 1, 0.3, 1] as const

type Destination = { workspace: CloudWorkspaceSummary | null }
type Scene = Destination & { x: number; y: number; since: number; leaving: boolean }

// The passage between workspaces: a liquid floods the window from the
// switcher's mark, holds the destination's mark and name while the new
// workspace loads behind it, then drains away to reveal it.
export function WorkspaceSwitchTransition({ destination, anchor }: {
  destination: Destination | null
  anchor: RefObject<HTMLElement | null>
}) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<Scene | null>(null)
  const [flat, setFlat] = useState(false)
  const canvas = useRef<HTMLCanvasElement>(null)
  const liquid = useRef<ReturnType<typeof runLiquid>>(null)

  useEffect(() => {
    if (destination) {
      const rect = anchor.current?.getBoundingClientRect()
      setScene({
        ...destination,
        x: rect ? rect.left + rect.width / 2 : 24,
        y: rect ? rect.top + rect.height / 2 : 24,
        since: performance.now(),
        leaving: false,
      })
      return
    }
    if (!scene) return
    const hold = Math.max(0, MIN_MS - (performance.now() - scene.since))
    const leave = setTimeout(() => setScene((current) => current && { ...current, leaving: true }), hold)
    const end = setTimeout(() => setScene(null), hold + LIQUID_LEAVE_MS)
    return () => {
      clearTimeout(leave)
      clearTimeout(end)
    }
    // The scene only changes here, so depending on it would restart its own timers.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [destination, anchor])

  const since = scene?.since
  useEffect(() => {
    if (since === undefined || !canvas.current) return
    liquid.current = reduce ? null : runLiquid(canvas.current, { x: scene!.x, y: scene!.y })
    setFlat(!liquid.current)
    return () => liquid.current?.stop()
    // A scene's origin is fixed when it starts.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [since, reduce])

  const leaving = scene?.leaving
  useEffect(() => {
    if (leaving) liquid.current?.leave()
  }, [leaving])

  if (typeof document === 'undefined' || !scene) return null
  const name = scene.workspace ? workspaceDisplayName(scene.workspace) : 'Local'
  return createPortal(
    <div
      role="status"
      aria-label={`Switching to ${name}`}
      style={{ zIndex: 'calc(var(--z-modal) + 2)' }}
      className={`fixed inset-0 grid place-items-center transition-opacity duration-300 [-webkit-app-region:no-drag] ${flat ? 'bg-bg' : ''} ${flat && leaving ? 'opacity-0' : ''}`}
    >
      <canvas ref={canvas} className="absolute inset-0 size-full" />
      <AnimatePresence>
        {leaving ? null : (
          <motion.div
            className="relative flex flex-col items-center gap-5"
            initial={{ opacity: 0, scale: 0.86, filter: 'blur(12px)' }}
            animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
            exit={{ opacity: 0, scale: 1.12, filter: 'blur(12px)' }}
            transition={{ duration: 0.5, delay: 0.4, ease: EASE }}
          >
            <WorkspaceMark
              workspace={scene.workspace}
              className="size-[88px] rounded-[24px] text-[38px] shadow-[0_18px_60px_color-mix(in_oklab,var(--color-primary)_45%,transparent)]"
            />
            <p className="max-w-[70vw] truncate text-[22px] font-semibold text-ink">{name}</p>
          </motion.div>
        )}
      </AnimatePresence>
    </div>,
    document.body,
  )
}
