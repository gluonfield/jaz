import { type UIEvent, useCallback, useLayoutEffect, useRef, useState } from 'react'

const NEAR_BOTTOM_PX = 80

function isNearBottom(el: HTMLDivElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX
}

export function useThreadAutoScroll({ resetKey }: { resetKey: string }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const observerRef = useRef<ResizeObserver | null>(null)
  const nearBottom = useRef(true)
  const [showScrollToBottom, setShowScrollToBottom] = useState(false)

  const pinToBottom = useCallback(() => {
    nearBottom.current = true
    setShowScrollToBottom(false)
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [])

  // Re-pin on any content-box change (sent bubble, streaming deltas, composer
  // collapse, image/markdown reflow). Proxying growth through render counters
  // missed shifts that landed after a send, so sends sometimes stopped short.
  const attachScroll = useCallback((node: HTMLDivElement | null) => {
    observerRef.current?.disconnect()
    scrollRef.current = node
    const content = node?.firstElementChild
    if (!node || !content) return
    const observer = new ResizeObserver(() => {
      if (nearBottom.current) node.scrollTop = node.scrollHeight
    })
    observer.observe(content, { box: 'border-box' })
    observerRef.current = observer
  }, [])

  useLayoutEffect(() => {
    pinToBottom()
  }, [resetKey, pinToBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const nextNearBottom = isNearBottom(event.currentTarget)
    nearBottom.current = nextNearBottom
    setShowScrollToBottom(!nextNearBottom)
  }, [])

  return { scrollRef, attachScroll, showScrollToBottom, onScroll, scrollToBottom: pinToBottom, pinToBottom }
}
