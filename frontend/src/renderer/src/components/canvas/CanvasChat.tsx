import { Maximize2, Minimize2 } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useCallback, useMemo, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'

const CHAT_PREF_KEY = 'jaz.canvasChat'
const CHAT_WIDTH_KEY = 'jaz.canvasChatWidth'

// A canvas thread can hide its chat, leaving only the composer over a
// full-width board. The choice holds across canvas threads, like the
// sidebar's, but the chat comes back while the agent waits on the user.
export function useCanvasChat(canvas: boolean, waiting: boolean, isMobile: boolean) {
  const [open, setOpen] = useState(() => localStorage.getItem(CHAT_PREF_KEY) !== 'hidden')
  const [preferredWidth, setPreferredWidth] = useState(() => {
    const saved = Number(localStorage.getItem(CHAT_WIDTH_KEY))
    return Number.isFinite(saved) && saved > 0 ? saved : 360
  })
  const [containerWidth, setContainerWidth] = useState(0)
  const [resizing, setResizing] = useState(false)
  const containerRef = useCallback((element: HTMLDivElement | null) => {
    if (!element) {
      return
    }
    const observer = new ResizeObserver(([entry]) => setContainerWidth(Math.round(entry.contentRect.width)))
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  const minWidth = Math.min(280, containerWidth || 280)
  const maxWidth = Math.max(minWidth, Math.min(720, (containerWidth || 960) - 240))
  const width = isMobile ? Math.min(360, containerWidth || 360) : Math.min(maxWidth, Math.max(minWidth, preferredWidth))
  const resize = useCallback((next: number) => {
    setPreferredWidth(next)
    localStorage.setItem(CHAT_WIDTH_KEY, String(next))
  }, [])
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
  return useMemo(() => ({ shown, toggle, width, minWidth, maxWidth, resize, resizing, setResizing, containerRef }), [shown, toggle, width, minWidth, maxWidth, resize, resizing, containerRef])
}

export function CanvasChatToggle({ chat }: { chat: ReturnType<typeof useCanvasChat> }) {
  const label = chat.shown ? 'Hide chat' : 'Show chat'
  return (
    <IconButton variant="ghost" size="md" aria-label={label} title={`${label} (⌘\\)`} onClick={chat.toggle}>
      <AnimatePresence initial={false}>
        <motion.span
          key={String(chat.shown)}
          className="[grid-area:1/1]"
          initial={{ opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
          animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
          exit={{ opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
        >
          {chat.shown ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </motion.span>
      </AnimatePresence>
    </IconButton>
  )
}
