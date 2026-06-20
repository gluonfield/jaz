import { MessageSquareQuote } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

interface Pending {
  text: string
  sourceSeq?: number
  top: number
  left: number
}

// Reads the live text selection, and when it lands inside a rendered assistant
// message (`.chat-prose`) surfaces a floating "Add to chat" action over it. The
// selection is captured on pointer/key release so clicking the action doesn't
// race the browser clearing the range.
export function SelectionQuoteToolbar({
  scrollRef,
  onAdd,
}: {
  scrollRef: RefObject<HTMLElement | null>
  onAdd: (quote: { text: string; sourceSeq?: number }) => void
}) {
  const [pending, setPending] = useState<Pending | null>(null)
  const barRef = useRef<HTMLDivElement>(null)

  const evaluate = useCallback(() => {
    const selection = window.getSelection()
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
      setPending(null)
      return
    }
    const text = selection.toString().trim()
    if (!text) {
      setPending(null)
      return
    }
    const range = selection.getRangeAt(0)
    const container =
      range.commonAncestorContainer.nodeType === Node.ELEMENT_NODE
        ? (range.commonAncestorContainer as Element)
        : range.commonAncestorContainer.parentElement
    const prose = container?.closest('.chat-prose')
    if (!prose) {
      setPending(null)
      return
    }
    const seqEl = prose.closest('[data-message-seq]')
    const sourceSeq = seqEl ? Number(seqEl.getAttribute('data-message-seq')) : undefined
    const rect = range.getBoundingClientRect()
    setPending({
      text,
      ...(sourceSeq !== undefined && Number.isFinite(sourceSeq) ? { sourceSeq } : {}),
      top: rect.top,
      left: rect.left + rect.width / 2,
    })
  }, [])

  useEffect(() => {
    const onPointerUp = () => window.setTimeout(evaluate, 0)
    const onKeyUp = (event: KeyboardEvent) => {
      if (event.shiftKey || event.key === 'Shift') window.setTimeout(evaluate, 0)
    }
    const onPointerDown = (event: PointerEvent) => {
      if (barRef.current?.contains(event.target as Node)) return
      setPending(null)
    }
    const onSelectionChange = () => {
      const selection = window.getSelection()
      if (!selection || selection.isCollapsed) setPending(null)
    }
    document.addEventListener('pointerup', onPointerUp)
    document.addEventListener('keyup', onKeyUp)
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('selectionchange', onSelectionChange)
    return () => {
      document.removeEventListener('pointerup', onPointerUp)
      document.removeEventListener('keyup', onKeyUp)
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('selectionchange', onSelectionChange)
    }
  }, [evaluate])

  useEffect(() => {
    if (!pending) return
    const node = scrollRef.current
    const dismiss = () => setPending(null)
    node?.addEventListener('scroll', dismiss, { passive: true })
    window.addEventListener('resize', dismiss)
    return () => {
      node?.removeEventListener('scroll', dismiss)
      window.removeEventListener('resize', dismiss)
    }
  }, [pending, scrollRef])

  const add = () => {
    if (!pending) return
    onAdd({ text: pending.text, ...(pending.sourceSeq !== undefined ? { sourceSeq: pending.sourceSeq } : {}) })
    window.getSelection()?.removeAllRanges()
    setPending(null)
  }

  return createPortal(
    <AnimatePresence>
      {pending ? (
        <motion.div
          ref={barRef}
          initial={{ opacity: 0, y: 4, scale: 0.96 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: 4, scale: 0.96 }}
          transition={{ type: 'spring', duration: 0.22, bounce: 0 }}
          style={{
            top: Math.max(8, pending.top - 44),
            left: Math.min(Math.max(pending.left, 80), window.innerWidth - 80),
          }}
          className="fixed z-50 -translate-x-1/2"
        >
          <button
            type="button"
            // Pointer events on the bar must not collapse the selection before the click lands.
            onMouseDown={(event) => event.preventDefault()}
            onClick={add}
            className="flex items-center gap-1.5 rounded-full bg-ink px-3 py-1.5 text-xs font-medium text-bg shadow-[0_8px_30px_rgba(0,0,0,0.22)] transition-transform duration-150 hover:scale-[1.03]"
          >
            <MessageSquareQuote size={13} />
            Add to chat
          </button>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
