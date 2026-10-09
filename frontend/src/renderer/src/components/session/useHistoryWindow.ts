import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import type { HistoryPaging } from '@/lib/hooks/useSessionHistory'

function useHistoryScroll({
  scrollRef,
  firstKey,
  hasMore,
  onLoadMore,
}: {
  scrollRef: RefObject<HTMLDivElement | null>
  firstKey: string | undefined
  hasMore: boolean
  onLoadMore: () => void
}) {
  const historyRef = useRef<HTMLDivElement>(null)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const anchor = useRef<{ element: Element; top: number } | null>(null)
  const loadMore = useRef(onLoadMore)

  useLayoutEffect(() => {
    loadMore.current = onLoadMore
  })

  useLayoutEffect(() => {
    const viewport = scrollRef.current
    const history = historyRef.current
    const first = history?.firstElementChild
    if (!history || !first) return
    const contentTop = (element: Element) => element.getBoundingClientRect().top - history.getBoundingClientRect().top
    const previous = anchor.current
    // Native anchoring also preserves the reading position through later image
    // reflow, but Chromium skips it at scroll offset 0, where a reader would be
    // left on the oldest revealed item with the sentinel still in view.
    const nativeAnchoring = CSS.supports('overflow-anchor', 'auto') && Boolean(viewport?.scrollTop)
    if (viewport && !nativeAnchoring && previous && previous.element !== first && previous.element.isConnected) {
      viewport.scrollTop += contentTop(previous.element) - previous.top
    }
    anchor.current = { element: first, top: contentTop(first) }
  }, [firstKey, scrollRef])

  useEffect(() => {
    const viewport = scrollRef.current
    const sentinel = sentinelRef.current
    if (!viewport || !sentinel || !hasMore) return
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) loadMore.current()
    }, { root: viewport, rootMargin: '240px 0px 0px' })
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [firstKey, hasMore, scrollRef])

  return { historyRef, sentinelRef }
}

// Renders the newest `initial` of `count` items and reveals `batch` more as the
// reader scrolls up, fetching earlier pages once every loaded item is shown.
export function useHistoryWindow({
  scrollRef,
  count,
  initial,
  batch,
  keyAt,
  paging,
  showAll = false,
}: {
  scrollRef: RefObject<HTMLDivElement | null>
  count: number
  initial: number
  batch: number
  keyAt: (index: number) => string | undefined
  paging?: HistoryPaging
  showAll?: boolean
}) {
  const [visible, setVisible] = useState(initial)

  useEffect(() => {
    setVisible((current) => Math.min(count, Math.max(current, initial)))
  }, [count, initial])

  const start = showAll ? 0 : Math.max(0, count - visible)
  const reveal = () => {
    if (start > 0) {
      setVisible((current) => Math.min(count, current + batch))
      return
    }
    if (!paging || paging.loading) return
    void paging.loadEarlier().then((loaded) => {
      if (loaded) setVisible(Number.MAX_SAFE_INTEGER)
    })
  }
  const scroll = useHistoryScroll({
    scrollRef,
    firstKey: keyAt(start),
    hasMore: start > 0 || Boolean(paging?.hasEarlier),
    onLoadMore: reveal,
  })
  return { start, ...scroll }
}
