import { MessageSquareQuote } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

interface Pending {
  text: string
  top: number
  left: number
}

// Reads the live text selection, and when it lands inside a rendered assistant
// message (`.chat-prose`) surfaces a floating "Comment" action over it. Clicking
// it opens an inline comment bar, like a browser annotation, that attaches the
// quote plus the comment to the composer. The selection is captured on
// pointer/key release so the action doesn't race the browser clearing the range.
export function SelectionContextToolbar({
  scrollRef,
  onAdd,
}: {
  scrollRef: RefObject<HTMLElement | null>
  onAdd: (text: string, comment: string) => void
}) {
  const [pending, setPending] = useState<Pending | null>(null)
  const [composing, setComposing] = useState(false)
  const [comment, setComment] = useState('')
  const barRef = useRef<HTMLDivElement>(null)
  // Mirrors `composing` synchronously so the dismiss handlers below don't tear
  // the bar down when focusing the input collapses the text selection.
  const composingRef = useRef(false)

  const reset = useCallback(() => {
    composingRef.current = false
    setComposing(false)
    setComment('')
    setPending(null)
  }, [])

  const startComposing = useCallback(() => {
    composingRef.current = true
    setComposing(true)
  }, [])

  const evaluate = useCallback(() => {
    const selection = window.getSelection()
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
      reset()
      return
    }
    const text = selection.toString().trim()
    if (!text) {
      reset()
      return
    }
    const range = selection.getRangeAt(0)
    const container =
      range.commonAncestorContainer.nodeType === Node.ELEMENT_NODE
        ? (range.commonAncestorContainer as Element)
        : range.commonAncestorContainer.parentElement
    if (!container?.closest('.chat-prose')) {
      reset()
      return
    }
    const rect = range.getBoundingClientRect()
    composingRef.current = false
    setComposing(false)
    setComment('')
    setPending({ text, top: rect.top, left: rect.left + rect.width / 2 })
  }, [reset])

  useEffect(() => {
    const onPointerUp = (event: PointerEvent) => {
      if (composingRef.current || barRef.current?.contains(event.target as Node)) return
      window.setTimeout(evaluate, 0)
    }
    const onKeyUp = (event: KeyboardEvent) => {
      if (composingRef.current) return
      if (event.shiftKey || event.key === 'Shift') window.setTimeout(evaluate, 0)
    }
    const onPointerDown = (event: PointerEvent) => {
      if (barRef.current?.contains(event.target as Node)) return
      reset()
    }
    const onSelectionChange = () => {
      if (composingRef.current) return
      const selection = window.getSelection()
      if (!selection || selection.isCollapsed) reset()
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
  }, [evaluate, reset])

  useEffect(() => {
    if (!pending) return
    const node = scrollRef.current
    // Keep the bar anchored only while idle; tearing it down mid-comment would
    // drop what the user typed.
    const dismiss = () => {
      if (!composingRef.current) reset()
    }
    node?.addEventListener('scroll', dismiss, { passive: true })
    window.addEventListener('resize', dismiss)
    return () => {
      node?.removeEventListener('scroll', dismiss)
      window.removeEventListener('resize', dismiss)
    }
  }, [pending, reset, scrollRef])

  const add = () => {
    if (!pending) return
    onAdd(pending.text, comment.trim())
    window.getSelection()?.removeAllRanges()
    reset()
  }

  const halfWidth = composing ? 150 : 70
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
            left: Math.min(Math.max(pending.left, halfWidth), window.innerWidth - halfWidth),
          }}
          className="fixed z-50 -translate-x-1/2"
        >
          {composing ? (
            <div className="flex items-center gap-1 rounded-full border border-border bg-surface py-1 pr-1 pl-3.5 shadow-[0_8px_30px_rgba(0,0,0,0.22)]">
              <input
                autoFocus
                value={comment}
                onChange={(event) => setComment(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    add()
                  } else if (event.key === 'Escape') {
                    event.preventDefault()
                    reset()
                  }
                }}
                placeholder="Add a comment…"
                className="w-52 bg-transparent text-xs text-ink outline-none placeholder:text-ink-3"
              />
              <button
                type="button"
                // Keep focus (and the typed comment) in the input through the click.
                onMouseDown={(event) => event.preventDefault()}
                onClick={add}
                className="rounded-full bg-ink px-3 py-1.5 text-xs font-medium text-bg transition-transform duration-150 hover:scale-[1.03]"
              >
                Add
              </button>
            </div>
          ) : (
            <button
              type="button"
              // Pointer events on the bar must not collapse the selection before the click lands.
              onMouseDown={(event) => event.preventDefault()}
              onClick={startComposing}
              className="flex items-center gap-1.5 rounded-full bg-ink px-3 py-1.5 text-xs font-medium text-bg shadow-[0_8px_30px_rgba(0,0,0,0.22)] transition-transform duration-150 hover:scale-[1.03]"
            >
              <MessageSquareQuote size={13} />
              Comment
            </button>
          )}
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
