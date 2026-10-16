import { useCallback, useMemo, useState } from 'react'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'

const CHAT_PREF_KEY = 'jaz.canvasChat'

// A canvas thread can hide its chat to give the board the whole width. The
// choice holds across canvas threads, like the sidebar's, but the chat comes
// back while the agent waits on the user's answer.
export function useCanvasChat(canvas: boolean, waiting: boolean) {
  const [open, setOpen] = useState(() => localStorage.getItem(CHAT_PREF_KEY) !== 'hidden')
  const shown = !canvas || open || waiting
  const toggle = useCallback(() => {
    localStorage.setItem(CHAT_PREF_KEY, shown ? 'hidden' : 'open')
    setOpen(!shown)
  }, [shown])
  useWindowEvent('keydown', (event) => {
    if (!(event.metaKey || event.ctrlKey) || event.shiftKey || event.altKey || event.code !== 'Backslash' || event.defaultPrevented || modalDialogOpen()) return
    event.preventDefault()
    toggle()
  }, canvas)
  return useMemo(() => ({ shown, toggle }), [shown, toggle])
}
