import { type ReactNode, useLayoutEffect, useRef } from 'react'
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
      className="pointer-events-none absolute inset-x-0 bottom-0 z-[1] bg-gradient-to-b from-transparent to-bg to-45% pt-6 pb-5"
    >
      <div className={`pointer-events-none relative ${THREAD_COLUMN_CLASS}`}>
        <div className="pointer-events-auto absolute inset-x-0 bottom-full">{before}</div>
        <div className="pointer-events-auto">
          {children}
        </div>
      </div>
    </div>
  )
}
