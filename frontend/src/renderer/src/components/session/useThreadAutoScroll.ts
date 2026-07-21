import { type MouseEvent, type UIEvent, useCallback, useLayoutEffect, useRef, useState } from 'react'

const NEAR_BOTTOM_PX = 80

type ScrollViewport = Pick<HTMLDivElement, 'clientHeight' | 'scrollHeight' | 'scrollTop'>

function isNearBottom(el: ScrollViewport): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX
}

export function applyThreadResize(el: ScrollViewport, followBottom: boolean): boolean {
  if (followBottom) el.scrollTop = el.scrollHeight
  return !isNearBottom(el)
}

export function useThreadAutoScroll({ resetKey }: { resetKey: string }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const observerRef = useRef<ResizeObserver | null>(null)
  const followBottom = useRef(true)
  const [showScrollToBottom, setShowScrollToBottom] = useState(false)

  const pinToBottom = useCallback(() => {
    followBottom.current = true
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
      setShowScrollToBottom(applyThreadResize(node, followBottom.current))
    })
    observer.observe(content, { box: 'border-box' })
    observerRef.current = observer
  }, [])

  useLayoutEffect(() => {
    pinToBottom()
  }, [resetKey, pinToBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const nextNearBottom = isNearBottom(event.currentTarget)
    followBottom.current = nextNearBottom
    setShowScrollToBottom(!nextNearBottom)
  }, [])

  const onClickCapture = useCallback((event: MouseEvent<HTMLDivElement>) => {
    const target = event.target
    if (!(target instanceof Element) || !target.closest('button[aria-expanded="false"]')) return
    // A disclosure is a reading interaction: let it grow below its trigger
    // instead of treating its transition as new transcript output.
    followBottom.current = false
  }, [])

  return {
    scrollRef,
    attachScroll,
    showScrollToBottom,
    onScroll,
    onClickCapture,
    scrollToBottom: pinToBottom,
    pinToBottom,
  }
}
