import { Paperclip } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { dataTransferHasFiles } from './fileTransfer'

// Window-level file dropping, in one place: the app-shell guard that keeps a
// stray drop from navigating the window to its file:// URL, the page-level
// drop hook, and the full-screen overlay. Guard and zones coordinate through
// event phases — the guard denies in capture (dropEffect 'none'), an active
// drop zone re-allows in bubble ('copy'). Text drags pass through untouched
// so dragging a selection into an input stays native.

/** Install once at startup, before any drop zone mounts. */
export function installFileDropGuard() {
  window.addEventListener(
    'dragover',
    (event) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
    },
    true,
  )
  window.addEventListener(
    'drop',
    (event) => {
      if (dataTransferHasFiles(event.dataTransfer)) event.preventDefault()
    },
    true,
  )
}

/** Accept file drops anywhere in the window — drop intent is page-level, and
    window scope means the overlay also covers the sidebar and panels. Assumes
    one drop zone per page. Returns whether a file drag is in progress. */
export function useWindowFileDrop({
  disabled = false,
  onDrop,
}: {
  disabled?: boolean
  onDrop: (files: File[]) => void
}): boolean {
  const [dragging, setDragging] = useState(false)
  useEffect(() => {
    if (disabled) return
    // dragenter/dragleave fire per element; the counter nets out nested
    // targets so only leaving the window (or dropping) clears the overlay.
    let depth = 0
    let watchdog: number | undefined
    const clear = () => {
      window.clearTimeout(watchdog)
      depth = 0
      setDragging(false)
    }
    // A hovering drag re-fires dragover every ~350ms, but Chromium sometimes
    // loses the final dragleave at a window edge; without a heartbeat the
    // overlay would stick after the drag is gone.
    const armWatchdog = () => {
      window.clearTimeout(watchdog)
      watchdog = window.setTimeout(clear, 1000)
    }
    const onDragEnter = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      event.preventDefault()
      depth += 1
      setDragging(true)
      armWatchdog()
    }
    const onDragOver = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
      armWatchdog()
    }
    const onDragLeave = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      depth = Math.max(0, depth - 1)
      if (depth === 0) clear()
    }
    const onWindowDrop = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      event.preventDefault()
      const dropped = Array.from(event.dataTransfer?.files ?? [])
      clear()
      if (dropped.length > 0) onDrop(dropped)
    }
    window.addEventListener('dragenter', onDragEnter)
    window.addEventListener('dragover', onDragOver)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', onWindowDrop)
    return () => {
      clear()
      window.removeEventListener('dragenter', onDragEnter)
      window.removeEventListener('dragover', onDragOver)
      window.removeEventListener('dragleave', onDragLeave)
      window.removeEventListener('drop', onWindowDrop)
    }
  }, [disabled, onDrop])
  return dragging
}

/** Full-screen "drop to attach" affordance for useWindowFileDrop. Portaled:
    callers often sit under transform-animated ancestors, which re-anchor
    fixed positioning. Pointer-transparent: drops are handled on the window,
    and a purely visual overlay can never leave the app click-dead. */
export function FileDropOverlay({ visible }: { visible: boolean }) {
  const reducedMotion = useReducedMotion()
  return createPortal(
    <AnimatePresence>
      {visible ? (
        <motion.div
          key="drop-overlay"
          className="pointer-events-none fixed inset-0 z-file-drop bg-bg/60 backdrop-blur-[2px]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.18, ease: 'easeOut' }}
        >
          <div
            aria-hidden
            className="absolute inset-2.5 rounded-[14px] shadow-[inset_0_0_0_1.5px_var(--color-primary)]"
          />
          <motion.div
            className="grid h-full place-items-center"
            initial={reducedMotion ? { opacity: 0 } : { opacity: 0, y: 10, scale: 0.97 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reducedMotion ? { opacity: 0 } : { opacity: 0, y: 6, scale: 0.98 }}
            transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          >
            <div className="flex items-center gap-2.5 rounded-full bg-surface px-5 py-3 shadow-[0_2px_8px_rgba(0,0,0,0.08),0_12px_40px_rgba(0,0,0,0.18)]">
              <Paperclip size={16} className="shrink-0 text-primary" />
              <span className="text-sm font-medium text-ink">Drop files to attach</span>
            </div>
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
