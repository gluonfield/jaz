import { type MouseEvent, type UIEvent, useCallback, useLayoutEffect, useRef, useState } from 'react'

const NEAR_BOTTOM_PX = 80

type ScrollViewport = Pick<HTMLDivElement, 'clientHeight' | 'scrollHeight' | 'scrollTop'>

function isNearBottom(el: ScrollViewport): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX
}

export function createThreadScrollState() {
  let following = true
  return {
    follow() {
      following = true
    },
    pause() {
      following = false
    },
    scroll(el: ScrollViewport) {
      following = isNearBottom(el)
      return !following
    },
    resize(el: ScrollViewport) {
      if (following) el.scrollTop = el.scrollHeight
      return !following
    },
  }
}

export function useThreadAutoScroll({ resetKey }: { resetKey: string }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const observerRef = useRef<ResizeObserver | null>(null)
  const scrollStateRef = useRef<ReturnType<typeof createThreadScrollState> | null>(null)
  if (!scrollStateRef.current) scrollStateRef.current = createThreadScrollState()
  const scrollState = scrollStateRef.current
  const [showScrollToBottom, setShowScrollToBottom] = useState(false)

  const pinToBottom = useCallback(() => {
    scrollState.follow()
    setShowScrollToBottom(false)
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [scrollState])

  // Re-pin on any content-box change (sent bubble, streaming deltas, composer
  // collapse, image/markdown reflow). Proxying growth through render counters
  // missed shifts that landed after a send, so sends sometimes stopped short.
  const attachScroll = useCallback((node: HTMLDivElement | null) => {
    observerRef.current?.disconnect()
    scrollRef.current = node
    const content = node?.firstElementChild
    if (!node || !content) return
    const observer = new ResizeObserver(() => {
      setShowScrollToBottom(scrollState.resize(node))
    })
    observer.observe(content, { box: 'border-box' })
    observerRef.current = observer
  }, [scrollState])

  useLayoutEffect(() => {
    pinToBottom()
  }, [resetKey, pinToBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    setShowScrollToBottom(scrollState.scroll(event.currentTarget))
  }, [scrollState])

  const onClickCapture = useCallback((event: MouseEvent<HTMLDivElement>) => {
    const target = event.target
    if (!(target instanceof Element) || !target.closest('button[aria-expanded="false"]')) return
    // A disclosure is a reading interaction: let it grow below its trigger
    // instead of treating its transition as new transcript output.
    scrollState.pause()
    setShowScrollToBottom(true)
  }, [scrollState])

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
