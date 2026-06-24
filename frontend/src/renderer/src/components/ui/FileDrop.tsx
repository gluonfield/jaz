import { Paperclip } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import {
  type HTMLAttributes,
  type RefObject,
  createContext,
  forwardRef,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react'
import { dataTransferHasFiles } from './fileTransfer'

// File dropping, in one place: the app-shell guard that keeps a stray drop from
// navigating the window to its file:// URL, the scoped drop hook, and the
// local overlay. Guard and zones coordinate through
// event phases — the guard denies in capture (dropEffect 'none'), an active
// drop zone re-allows in bubble ('copy'). Text drags pass through untouched
// so dragging a selection into an input stays native.

const FileDropScopeContext = createContext<string | undefined>(undefined)

export const FileDropScope = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(function FileDropScope(
  { children, ...props },
  ref,
) {
  const id = useId()
  return (
    <FileDropScopeContext.Provider value={id}>
      <div {...props} ref={ref} data-file-drop-scope={id}>
        {children}
      </div>
    </FileDropScopeContext.Provider>
  )
})

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

export function useFileDropTarget<T extends HTMLElement>({
  disabled = false,
  onDrop,
}: {
  disabled?: boolean
  onDrop: (files: File[]) => void
}): { dropTargetRef: RefObject<T | null>; dragging: boolean } {
  const scope = useContext(FileDropScopeContext)
  const dropTargetRef = useRef<T>(null)
  const [dragging, setDragging] = useState(false)
  useEffect(() => {
    const target = dropTargetRef.current
    if (!target) return
    if (disabled) return
    // dragenter/dragleave fire per child; the counter nets out nested targets
    // so only leaving this scope (or dropping) clears its overlay.
    let depth = 0
    let watchdog: number | undefined
    const clear = () => {
      window.clearTimeout(watchdog)
      depth = 0
      setDragging(false)
    }
    const ownsDrag = (event: DragEvent) => {
      const hover = document.elementFromPoint(event.clientX, event.clientY)
      if (!hover) return false
      if (scope) return hover.closest<HTMLElement>('[data-file-drop-scope]')?.dataset.fileDropScope === scope
      return target.contains(hover)
    }
    const syncDrag = (event: DragEvent) => {
      if (!ownsDrag(event)) {
        clear()
        return false
      }
      setDragging(true)
      armWatchdog()
      return true
    }
    // A hovering drag re-fires dragover every ~350ms, but Chromium sometimes
    // loses the final dragleave; without a heartbeat the overlay would stick
    // after the drag is gone.
    const armWatchdog = () => {
      window.clearTimeout(watchdog)
      watchdog = window.setTimeout(clear, 1000)
    }
    const onDragEnter = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      depth += 1
      if (syncDrag(event)) event.preventDefault()
    }
    const onDragOver = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      if (!syncDrag(event)) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    }
    const onDragLeave = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      depth = Math.max(0, depth - 1)
      if (depth === 0) clear()
    }
    const onTargetDrop = (event: DragEvent) => {
      if (!dataTransferHasFiles(event.dataTransfer)) return
      if (!ownsDrag(event)) {
        clear()
        return
      }
      event.preventDefault()
      const dropped = Array.from(event.dataTransfer?.files ?? [])
      clear()
      if (dropped.length > 0) onDrop(dropped)
    }
    window.addEventListener('dragenter', onDragEnter)
    window.addEventListener('dragover', onDragOver)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', onTargetDrop)
    return () => {
      clear()
      window.removeEventListener('dragenter', onDragEnter)
      window.removeEventListener('dragover', onDragOver)
      window.removeEventListener('dragleave', onDragLeave)
      window.removeEventListener('drop', onTargetDrop)
    }
  }, [disabled, onDrop, scope])
  return { dropTargetRef, dragging }
}

export function FileDropOverlay({ visible }: { visible: boolean }) {
  const reducedMotion = useReducedMotion()
  return (
    <AnimatePresence initial={false}>
      {visible ? (
        <motion.div
          key="drop-overlay"
          className="pointer-events-none absolute inset-0 z-file-drop rounded-[14px] bg-bg/68 backdrop-blur-[1.5px]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.18, ease: 'easeOut' }}
        >
          <div
            aria-hidden
            className="absolute inset-0 rounded-[14px] shadow-[inset_0_0_0_1.5px_var(--color-primary),0_10px_35px_rgba(0,0,0,0.16)]"
          />
          <motion.div
            className="grid h-full min-h-24 place-items-center p-3"
            initial={reducedMotion ? { opacity: 0 } : { opacity: 0, y: 8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reducedMotion ? { opacity: 0 } : { opacity: 0, y: 5, scale: 0.985 }}
            transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          >
            <div className="flex items-center gap-2 rounded-full bg-surface px-4 py-2.5 shadow-[0_2px_8px_rgba(0,0,0,0.08),0_12px_30px_rgba(0,0,0,0.16)]">
              <Paperclip size={15} className="shrink-0 text-primary" />
              <span className="text-sm font-medium text-ink">Drop files to attach</span>
            </div>
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}
