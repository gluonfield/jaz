import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import type { CloudWorkspaceSummary } from '@/lib/cloudAccount'
import { workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'
import { WorkspaceMark } from './WorkspaceMark'

// Long enough to read as leaving one place for another, however fast the switch itself is.
const MIN_MS = 1150
const EASE = [0.16, 1, 0.3, 1] as const
const GRID_LINE = 'color-mix(in oklab, var(--color-primary) 55%, transparent)'
const GRID = `linear-gradient(${GRID_LINE} 1px, transparent 1px), linear-gradient(90deg, ${GRID_LINE} 1px, transparent 1px)`

type Destination = { workspace: CloudWorkspaceSummary | null }
type Scene = Destination & { x: number; y: number; since: number }

// The passage between workspaces. The window irises closed from the switcher's
// mark, the destination's mark travels to the centre over a floor that streams
// past, and the new workspace is revealed once it has loaded behind.
export function WorkspaceSwitchTransition({ destination, anchor }: {
  destination: Destination | null
  anchor: RefObject<HTMLElement | null>
}) {
  const reduce = useReducedMotion()
  const [scene, setScene] = useState<Scene | null>(null)

  useEffect(() => {
    if (destination) {
      const rect = anchor.current?.getBoundingClientRect()
      setScene({
        ...destination,
        x: rect ? rect.left + rect.width / 2 : 24,
        y: rect ? rect.top + rect.height / 2 : 24,
        since: performance.now(),
      })
      return
    }
    if (!scene) return
    const timer = setTimeout(() => setScene(null), Math.max(0, MIN_MS - (performance.now() - scene.since)))
    return () => clearTimeout(timer)
    // The scene only changes here, so depending on it would restart its own timer.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [destination, anchor])

  if (typeof document === 'undefined') return null
  return createPortal(
    <AnimatePresence>
      {scene ? (
        <motion.div
          key="scene"
          role="status"
          aria-label={`Switching to ${scene.workspace ? workspaceDisplayName(scene.workspace) : 'Local'}`}
          style={{ zIndex: 'calc(var(--z-modal) + 2)' }}
          className="fixed inset-0 grid place-items-center overflow-hidden bg-bg [-webkit-app-region:no-drag]"
          initial={reduce ? { opacity: 0 } : { clipPath: `circle(0px at ${scene.x}px ${scene.y}px)` }}
          animate={reduce ? { opacity: 1 } : { clipPath: `circle(150vmax at ${scene.x}px ${scene.y}px)` }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, scale: 1.08, filter: 'blur(14px)' }}
          transition={{ duration: reduce ? 0.2 : 0.55, ease: EASE }}
        >
          {reduce ? null : (
            <>
              <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,color-mix(in_oklab,var(--color-primary)_22%,transparent),transparent_62%)]" />
              {[0, 1].map((side) => (
                <div
                  key={side}
                  className={`absolute inset-x-0 h-1/2 [perspective:520px] ${side ? 'top-0 -scale-y-100' : 'bottom-0'}`}
                >
                  <motion.div
                    className="absolute inset-x-[-60%] bottom-0 h-[220%] origin-bottom [mask-image:linear-gradient(to_top,black,transparent_78%)] [transform:rotateX(74deg)]"
                    style={{ backgroundImage: GRID, backgroundSize: '64px 64px' }}
                    animate={{ backgroundPositionY: [0, 64] }}
                    transition={{ duration: 0.38, ease: 'linear', repeat: Infinity }}
                  />
                </div>
              ))}
              {[0, 1, 2].map((ring) => (
                <motion.span
                  key={ring}
                  className="absolute size-[34vmin] rounded-full border border-primary/70"
                  initial={{ scale: 0.25, opacity: 0 }}
                  animate={{ scale: 4.2, opacity: [0, 0.7, 0] }}
                  transition={{ duration: 1.5, delay: 0.1 + ring * 0.3, ease: 'easeOut', repeat: Infinity }}
                />
              ))}
              <motion.span
                className="absolute inset-y-0 w-[36vw] -skew-x-[18deg] bg-gradient-to-r from-transparent via-primary/25 to-transparent"
                initial={{ x: '-90vw' }}
                animate={{ x: '90vw' }}
                transition={{ duration: 0.9, delay: 0.15, ease: EASE }}
              />
            </>
          )}
          <div className="relative flex flex-col items-center gap-5">
            <motion.div
              initial={reduce ? false : { x: scene.x - window.innerWidth / 2, y: scene.y - window.innerHeight / 2 + 22, scale: 0.4 }}
              animate={{ x: 0, y: 0, scale: 1 }}
              transition={{ type: 'spring', duration: 0.75, bounce: 0.18 }}
              className="rounded-[24px] shadow-[0_0_0_1px_color-mix(in_oklab,var(--color-primary)_55%,transparent),0_0_70px_color-mix(in_oklab,var(--color-primary)_60%,transparent)]"
            >
              <WorkspaceMark workspace={scene.workspace} className="size-[88px] rounded-[24px] text-[38px]" />
            </motion.div>
            <motion.p
              className="max-w-[70vw] truncate text-[22px] font-semibold text-ink"
              initial={reduce ? false : { opacity: 0, y: 10, letterSpacing: '0.35em', filter: 'blur(8px)' }}
              animate={{ opacity: 1, y: 0, letterSpacing: '-0.01em', filter: 'blur(0px)' }}
              transition={{ duration: 0.6, delay: 0.22, ease: EASE }}
            >
              {scene.workspace ? workspaceDisplayName(scene.workspace) : 'Local'}
            </motion.p>
          </div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
