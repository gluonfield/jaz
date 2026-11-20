import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import type { EventBus } from 'pdfjs-dist/web/pdf_viewer.mjs'
import type { FindControls } from './FindBar'

type Matches = { current: number; total: number }

// usePDFFind drives pdf.js's find controller through the viewer's event bus.
export function usePDFFind(events: EventBus | undefined, container: HTMLElement | null): FindControls & { handleKeyDownCapture: (event: KeyboardEvent<HTMLElement>) => void } {
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [matches, setMatches] = useState<Matches>({ current: 0, total: 0 })

  useEffect(() => {
    if (!events) return
    const update = ({ matchesCount }: { matchesCount: Matches }) => setMatches(matchesCount)
    events.on('updatefindmatchescount', update)
    events.on('updatefindcontrolstate', update)
    return () => {
      events.off('updatefindmatchescount', update)
      events.off('updatefindcontrolstate', update)
    }
  }, [events])

  const find = useCallback((type: '' | 'again', findPrevious: boolean) => {
    events?.dispatch('find', { source: null, type, query, caseSensitive: false, entireWord: false, highlightAll: true, matchDiacritics: false, findPrevious })
  }, [events, query])

  useEffect(() => {
    if (open) find('', false)
  }, [open, find])

  useEffect(() => {
    if (open && events) return () => events.dispatch('findbarclose', { source: null })
  }, [open, events])

  const close = useCallback(() => {
    setOpen(false)
    container?.focus()
  }, [container])

  const handleKeyDownCapture = useCallback((event: KeyboardEvent<HTMLElement>) => {
    if (!(event.metaKey || event.ctrlKey) || event.shiftKey || event.altKey || event.key.toLowerCase() !== 'f') return
    event.preventDefault()
    event.stopPropagation()
    setOpen(true)
    requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    })
  }, [])

  return {
    active: matches.current,
    close,
    findAgain: (forward: boolean) => find('again', !forward),
    handleKeyDownCapture,
    inputRef,
    matches: matches.total,
    open,
    query,
    setQuery,
  }
}
