import { useCallback, useEffect, useRef, useState } from 'react'
import { writeClipboard } from '@/lib/clipboard'

const COPIED_RESET_MS = 1500

// Copy `text` to the clipboard and flag `copied` for a short window. The flag
// resets whenever `text` changes so a stale "Copied" state never lingers on a
// button whose content has moved on (e.g. a code block still streaming).
export function useCopyAction(text: string) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const copy = useCallback(async () => {
    if (!(await writeClipboard(text))) return
    if (timer.current) clearTimeout(timer.current)
    setCopied(true)
    timer.current = setTimeout(() => setCopied(false), COPIED_RESET_MS)
  }, [text])

  useEffect(() => {
    setCopied(false)
  }, [text])

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current)
  }, [])

  return { copied, copy }
}
