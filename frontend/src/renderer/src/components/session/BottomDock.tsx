import { type ReactNode, useLayoutEffect, useRef } from 'react'
import { motion } from 'motion/react'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'

export function BottomDock({
  before,
  children,
  onHeightChange,
}: {
  before?: ReactNode
  children: ReactNode
  onHeightChange?: (height: number) => void
}) {
  const ref = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el || !onHeightChange) return
    const update = () => onHeightChange(Math.ceil(el.getBoundingClientRect().height))
    update()
    const observer = new ResizeObserver(update)
    observer.observe(el)
    return () => observer.disconnect()
  }, [onHeightChange])

  return (
    <div
      ref={ref}
      className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-b from-transparent to-bg to-45% pt-6 pb-5"
    >
      <motion.div
        className={`pointer-events-none ${THREAD_COLUMN_CLASS}`}
        initial={{ y: 12, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ type: 'spring', stiffness: 380, damping: 32 }}
      >
        <div className="pointer-events-auto">
          {before}
          {children}
        </div>
      </motion.div>
    </div>
  )
}
