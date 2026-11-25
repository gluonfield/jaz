import { type KeyboardEvent, type RefObject, useCallback, useEffect, useRef } from 'react'
import { backwardDeleteRange } from './composerTokens'

// On macOS Electron, Cocoa's deleteBackward: command can occasionally stop
// attaching to Backspace while insertText: still works. Native deletion fires
// onChange synchronously before a macrotask; if it did not run, apply the
// deletion here so Backspace never silently stalls.
export function useBackspaceFallback({
  textareaRef,
  composingRef,
  onFallbackDelete,
}: {
  textareaRef: RefObject<HTMLTextAreaElement | null>
  composingRef: RefObject<boolean>
  onFallbackDelete: (next: string, caret: number) => void
}) {
  const inputSeqRef = useRef(0)
  const timerRef = useRef<number | null>(null)
  const warnedRef = useRef(false)

  const cancel = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  useEffect(() => cancel, [cancel])

  const markNativeEdit = useCallback(() => {
    inputSeqRef.current += 1
    cancel()
  }, [cancel])

  const schedule = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      const el = e.currentTarget
      const start = el.selectionStart
      const end = el.selectionEnd
      if (e.defaultPrevented || el.disabled || el.readOnly || start === null || end === null || (start === end && start <= 0)) {
        return
      }
      const snapshotValue = el.value
      const snapshotSeq = inputSeqRef.current
      const modifiers = { meta: e.metaKey, alt: e.altKey, ctrl: e.ctrlKey }
      cancel()
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null
        if (
          inputSeqRef.current !== snapshotSeq ||
          composingRef.current ||
          textareaRef.current !== el ||
          document.activeElement !== el ||
          el.disabled ||
          el.readOnly ||
          el.value !== snapshotValue ||
          el.selectionStart !== start ||
          el.selectionEnd !== end
        ) {
          return
        }
        const range = backwardDeleteRange(snapshotValue, start, end, modifiers)
        if (!range) return
        if (!warnedRef.current) {
          warnedRef.current = true
          console.warn('[composer] native Backspace produced no edit; applied fallback deletion', {
            start,
            end,
            length: snapshotValue.length,
            modifiers,
          })
        }
        inputSeqRef.current += 1
        onFallbackDelete(
          snapshotValue.slice(0, range.from) + snapshotValue.slice(range.to),
          range.from,
        )
      }, 0)
    },
    [cancel, composingRef, onFallbackDelete, textareaRef],
  )

  return { cancel, markNativeEdit, schedule }
}
