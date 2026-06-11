import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'

const FIND_HIGHLIGHT = 'jaz-thread-find'
const ACTIVE_HIGHLIGHT = 'jaz-thread-find-active'
const HIGHLIGHT_STYLE_ID = 'jaz-thread-find-highlight-style'

type HighlightConstructor = new (...ranges: Range[]) => unknown
type HighlightRegistry = {
  set(name: string, highlight: unknown): void
  delete(name: string): void
}

function highlightSupport():
  | { registry: HighlightRegistry; Highlight: HighlightConstructor }
  | undefined {
  const css = CSS as typeof CSS & { highlights?: HighlightRegistry }
  const Highlight = (window as Window & { Highlight?: HighlightConstructor }).Highlight
  if (!css.highlights || !Highlight) return undefined
  return { registry: css.highlights, Highlight }
}

function clearHighlights(): void {
  const support = highlightSupport()
  support?.registry.delete(FIND_HIGHLIGHT)
  support?.registry.delete(ACTIVE_HIGHLIGHT)
}

function ensureHighlightStyle(): void {
  if (document.getElementById(HIGHLIGHT_STYLE_ID)) return
  const style = document.createElement('style')
  style.id = HIGHLIGHT_STYLE_ID
  style.textContent = `
::highlight(${FIND_HIGHLIGHT}) {
  background: color-mix(in oklab, var(--color-accent) 34%, transparent);
  color: inherit;
}
::highlight(${ACTIVE_HIGHLIGHT}) {
  background: var(--color-accent);
  color: var(--color-on-primary);
}`
  document.head.append(style)
}

function textNodeParent(node: Node): HTMLElement | null {
  const parent = node.parentNode instanceof HTMLElement ? node.parentNode : null
  if (!parent || parent.closest('[data-thread-find-ignore], input, textarea, select, script, style')) {
    return null
  }
  return parent
}

function matchRanges(root: HTMLElement, query: string): Range[] {
  const needle = query.toLocaleLowerCase()
  if (!needle) return []

  const ranges: Range[] = []
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      const parent = textNodeParent(node)
      if (!parent || !node.textContent?.trim()) return NodeFilter.FILTER_REJECT
      if (parent.offsetParent === null && getComputedStyle(parent).position !== 'fixed') {
        return NodeFilter.FILTER_REJECT
      }
      return NodeFilter.FILTER_ACCEPT
    },
  })

  let node = walker.nextNode()
  while (node) {
    const text = node.textContent ?? ''
    const haystack = text.toLocaleLowerCase()
    let offset = haystack.indexOf(needle)
    while (offset !== -1) {
      const range = document.createRange()
      range.setStart(node, offset)
      range.setEnd(node, offset + query.length)
      ranges.push(range)
      offset = haystack.indexOf(needle, offset + needle.length)
    }
    node = walker.nextNode()
  }
  return ranges
}

function paintHighlights(ranges: Range[], activeIndex: number): boolean {
  const support = highlightSupport()
  if (!support) return false
  ensureHighlightStyle()
  const passive = ranges.filter((_, index) => index !== activeIndex)
  support.registry.set(FIND_HIGHLIGHT, new support.Highlight(...passive))
  support.registry.set(ACTIVE_HIGHLIGHT, new support.Highlight(ranges[activeIndex]))
  return true
}

function scrollRangeIntoView(range: Range): void {
  const parent =
    range.startContainer instanceof Element
      ? range.startContainer
      : range.startContainer.parentNode instanceof HTMLElement
        ? range.startContainer.parentNode
        : null
  parent?.scrollIntoView({ block: 'center', inline: 'nearest' })
}

function modalOpen(): boolean {
  return Boolean(document.querySelector('[role="dialog"][aria-modal="true"]'))
}

export function useThreadFind(contentKey: string) {
  const rootRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const rangesRef = useRef<Range[]>([])
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const [matchCount, setMatchCount] = useState(0)
  const [rangeVersion, setRangeVersion] = useState(0)
  const [domVersion, setDomVersion] = useState(0)

  const trimmedQuery = query.trim()
  const activeMatch = matchCount ? Math.min(activeIndex, matchCount - 1) + 1 : 0

  const focusInput = useCallback(() => {
    requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    })
  }, [])

  const openFind = useCallback(() => {
    setOpen(true)
    focusInput()
  }, [focusInput])

  const closeFind = useCallback(() => {
    setOpen(false)
    clearHighlights()
  }, [])

  const findNext = useCallback(() => {
    setActiveIndex((index) => (matchCount ? (index + 1) % matchCount : 0))
  }, [matchCount])

  const findPrevious = useCallback(() => {
    setActiveIndex((index) => (matchCount ? (index - 1 + matchCount) % matchCount : 0))
  }, [matchCount])

  useEffect(() => {
    setActiveIndex(0)
  }, [trimmedQuery])

  useEffect(() => {
    if (!open) return
    const root = rootRef.current
    if (!root) return
    let frame = 0
    const observer = new MutationObserver(() => {
      if (frame) return
      frame = requestAnimationFrame(() => {
        frame = 0
        setDomVersion((version) => version + 1)
      })
    })
    observer.observe(root, { characterData: true, childList: true, subtree: true })
    return () => {
      if (frame) cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [open])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (modalOpen()) return
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'f') {
        event.preventDefault()
        event.stopPropagation()
        openFind()
        return
      }
      if (!open || event.defaultPrevented) return
      if (event.key === 'Escape') {
        event.preventDefault()
        closeFind()
        return
      }
      if (event.key === 'Enter' && document.activeElement === inputRef.current) {
        event.preventDefault()
        if (event.shiftKey) findPrevious()
        else findNext()
      }
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [closeFind, findNext, findPrevious, open, openFind])

  useLayoutEffect(() => {
    clearHighlights()
    const root = rootRef.current
    const ranges = open && trimmedQuery && root ? matchRanges(root, trimmedQuery) : []
    rangesRef.current = ranges
    setMatchCount(ranges.length)
    setActiveIndex((index) => (ranges.length ? Math.min(index, ranges.length - 1) : 0))
    setRangeVersion((version) => version + 1)
    return clearHighlights
  }, [contentKey, domVersion, open, trimmedQuery])

  useLayoutEffect(() => {
    clearHighlights()
    if (!open || !trimmedQuery) return

    const ranges = rangesRef.current
    if (!ranges.length) return

    const index = Math.min(activeIndex, ranges.length - 1)
    const highlighted = paintHighlights(ranges, index)
    const activeRange = ranges[index]
    scrollRangeIntoView(activeRange)
    if (!highlighted) {
      const selection = window.getSelection()
      selection?.removeAllRanges()
      selection?.addRange(activeRange)
    }
  }, [activeIndex, open, rangeVersion, trimmedQuery])

  useEffect(() => clearHighlights, [])

  return useMemo(
    () => ({
      rootRef,
      inputRef,
      open,
      query,
      activeMatch,
      matchCount,
      setQuery,
      openFind,
      closeFind,
      findNext,
      findPrevious,
    }),
    [activeMatch, closeFind, findNext, findPrevious, matchCount, open, openFind, query],
  )
}

export type ThreadFindController = ReturnType<typeof useThreadFind>
