import { type UIEvent, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'

const NEAR_BOTTOM_PX = 80

function isNearBottom(el: HTMLDivElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX
}

export function useThreadAutoScroll({ resetKey }: { resetKey: string }) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const nearBottom = useRef(true)
  const [showScrollToBottom, setShowScrollToBottom] = useState(false)

  const pinToBottom = useCallback(() => {
    nearBottom.current = true
    setShowScrollToBottom(false)
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [])

  useLayoutEffect(() => {
    pinToBottom()
  }, [resetKey, pinToBottom])

  // Follow the true bottom whenever content or viewport height changes while
  // pinned. Observing the real layout catches every growth source — new
  // messages, streaming deltas, the liveness indicator, async markdown/code/
  // image reflow, composer height — instead of proxying height through a
  // dependency list that misses late reflows and leaves the view short.
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const observer = new ResizeObserver(() => {
      if (nearBottom.current) el.scrollTop = el.scrollHeight
    })
    observer.observe(el)
    for (const child of el.children) observer.observe(child)
    return () => observer.disconnect()
  }, [resetKey])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const nextNearBottom = isNearBottom(event.currentTarget)
    nearBottom.current = nextNearBottom
    setShowScrollToBottom(!nextNearBottom)
  }, [])

  return { scrollRef, showScrollToBottom, onScroll, scrollToBottom: pinToBottom, pinToBottom }
}
