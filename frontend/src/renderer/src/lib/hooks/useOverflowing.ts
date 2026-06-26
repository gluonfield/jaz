import { type RefObject, useEffect, useRef, useState } from 'react'

// Tracks whether the referenced element's content overflows its (typically
// max-height clamped) box, for collapse/"show more" affordances. Re-measures on
// mount, on element resize, and whenever `deps` change — a clamped box doesn't
// resize as its content grows, so callers pass whatever content affects height.
export function useOverflowing(deps: unknown[]): [RefObject<HTMLDivElement | null>, boolean] {
  const ref = useRef<HTMLDivElement | null>(null)
  const [overflowing, setOverflowing] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setOverflowing(el.scrollHeight > el.clientHeight + 2)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)
  return [ref, overflowing]
}
