import { type UIEvent, useCallback, useLayoutEffect, useRef, useState } from 'react'

const NEAR_BOTTOM_PX = 80

function isNearBottom(el: HTMLDivElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX
}

export function useThreadAutoScroll({
  resetKey,
  itemCount,
  liveSize,
  bottomInset,
}: {
  resetKey: string
  itemCount: number
  liveSize: number
  bottomInset: number
}) {
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
    if (nearBottom.current) pinToBottom()
  }, [bottomInset, itemCount, liveSize, pinToBottom])

  useLayoutEffect(() => {
    pinToBottom()
  }, [resetKey, pinToBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const nextNearBottom = isNearBottom(event.currentTarget)
    nearBottom.current = nextNearBottom
    setShowScrollToBottom(!nextNearBottom)
  }, [])

  return { scrollRef, showScrollToBottom, onScroll, scrollToBottom: pinToBottom, pinToBottom }
}
