import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, type RefObject, useLayoutEffect, useRef, useState } from 'react'

export const EASE = [0.2, 0, 0, 1] as const
export const SWAP = { duration: 0.22, ease: EASE }

const roll = {
  enter: (direction: number) => ({ y: `${direction * 100}%`, opacity: 0 }),
  rest: { y: 0, opacity: 1 },
  exit: (direction: number) => ({ y: `${direction * -100}%`, opacity: 0 }),
}
const pop = {
  enter: { scale: 0.4, opacity: 0, filter: 'blur(3px)' },
  rest: { scale: 1, opacity: 1, filter: 'blur(0px)' },
}

// Which way the value last moved, so entering content follows it.
export function useDirection(index: number): number {
  const [last, setLast] = useState({ index, direction: 1 })
  if (last.index === index) {
    return last.direction
  }
  const next = { index, direction: index > last.index ? 1 : -1 }
  setLast(next)
  return next.direction
}

// Eases an auto-sized element to its new width whenever its content changes.
export function useWidthTransition(ref: RefObject<HTMLElement | null>, content: string) {
  const reduceMotion = useReducedMotion()
  const settled = useRef<number>(undefined)
  const running = useRef<Animation>(undefined)
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) {
      return
    }
    const from = running.current?.playState === 'running' ? element.offsetWidth : settled.current
    running.current?.cancel()
    const to = element.offsetWidth
    settled.current = to
    if (!reduceMotion && from !== undefined && from !== to) {
      running.current = element.animate({ width: [`${from}px`, `${to}px`] }, { duration: 240, easing: `cubic-bezier(${EASE.join()})` })
    }
  }, [ref, content, reduceMotion])
}

// Text that rolls to its next value along the direction it moved.
export function Roll({ value, direction, className = '' }: { value: string; direction: number; className?: string }) {
  return (
    <span className="relative inline-flex h-4 shrink-0 overflow-hidden leading-4">
      <AnimatePresence mode="popLayout" initial={false} custom={direction}>
        <motion.span
          key={value}
          custom={direction}
          variants={roll}
          initial="enter"
          animate="rest"
          exit="exit"
          transition={SWAP}
          className={`truncate ${className}`}
        >
          {value}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}

// An icon that swaps for the next one in place.
export function Pop({ id, children }: { id: string; children: ReactNode }) {
  return (
    <AnimatePresence mode="popLayout" initial={false}>
      <motion.span key={id} variants={pop} initial="enter" animate="rest" exit="enter" transition={SWAP} className="grid shrink-0 place-items-center">
        {children}
      </motion.span>
    </AnimatePresence>
  )
}

export function Tick({ on }: { on: boolean }) {
  return (
    <svg viewBox="0 0 24 24" className="size-[13px] shrink-0 text-ink-3" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <motion.path
        d="M4 12l5 5L20 6"
        initial={false}
        animate={{ pathLength: on ? 1 : 0, opacity: on ? 1 : 0 }}
        transition={{ duration: on ? 0.26 : 0.1, ease: EASE }}
      />
    </svg>
  )
}
