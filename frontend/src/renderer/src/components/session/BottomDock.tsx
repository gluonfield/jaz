import { type ReactNode, useLayoutEffect, useRef } from 'react'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'

export function BottomDock({
  before,
  children,
  compact,
  floating,
  onHeightChange,
}: {
  before?: ReactNode
  children: ReactNode
  // A canvas thread's chat is a narrow column.
  compact?: boolean
  // Over a canvas board: no fade into a thread, and the composer casts a shadow.
  floating?: boolean
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
      // The fade has to finish where the composer starts (pt-6). As a share of
      // the dock's height it was still half-transparent at that edge, so the
      // last line of the thread got sliced by the composer instead of fading
      // out under it.
      className={`pointer-events-none absolute inset-x-0 bottom-0 z-[1] pt-6 pb-5 ${floating ? '' : '[background:linear-gradient(to_bottom,transparent,var(--color-bg)_24px)]'}`}
    >
      <div className={`pointer-events-none relative ${compact ? 'px-3' : THREAD_COLUMN_CLASS}`}>
        <div className="pointer-events-auto absolute inset-x-0 bottom-full">{before}</div>
        <div className={`pointer-events-auto ${floating ? 'rounded-[12px] shadow-raised' : ''}`}>
          {children}
        </div>
      </div>
    </div>
  )
}
