import { AnimatePresence, motion } from 'motion/react'
import { Check, Copy, X } from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { writeClipboard } from '@/lib/clipboard'

interface Toast {
  id: number
  message: string
  tone: 'ok' | 'danger'
}

const TOAST_AUTO_DISMISS_MS = 3_000
const actionClass =
  'relative grid size-8 shrink-0 place-items-center rounded-full text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]'
const toastMotion = {
  initial: { opacity: 0, y: 8, scale: 0.97 },
  animate: { opacity: 1, y: 0, scale: 1 },
  exit: { opacity: 0, y: 4, scale: 0.97 },
  transition: { type: 'spring', stiffness: 420, damping: 30 },
} as const

function ToastTimer() {
  return (
    <svg aria-hidden="true" viewBox="0 0 16 16" className="pointer-events-none absolute inset-1 -rotate-90 text-primary">
      <motion.circle
        cx="8"
        cy="8"
        r="6"
        fill="none"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.5"
        initial={{ pathLength: 1, opacity: 0.8 }}
        animate={{ pathLength: 0, opacity: 0.45 }}
        transition={{ duration: TOAST_AUTO_DISMISS_MS / 1000, ease: 'linear' }}
      />
    </svg>
  )
}

const ToastContext = createContext<(message: string, tone?: Toast['tone']) => void>(() => {})

export function useToast() {
  return useContext(ToastContext)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const [copiedId, setCopiedId] = useState<number | null>(null)
  const nextId = useRef(0)
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>())
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id)
    if (timer) clearTimeout(timer)
    timers.current.delete(id)
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback((message: string, tone: Toast['tone'] = 'ok') => {
    const id = nextId.current++
    const autoDismiss = tone !== 'danger'
    setToasts((prev) => [...prev, { id, message, tone }])
    if (autoDismiss) {
      timers.current.set(id, setTimeout(() => dismiss(id), TOAST_AUTO_DISMISS_MS))
    }
  }, [dismiss])

  const copy = useCallback(async (id: number, message: string) => {
    if (!(await writeClipboard(message))) return
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
    setCopiedId(id)
    copiedTimer.current = setTimeout(() => setCopiedId(null), 1500)
  }, [])

  useEffect(() => () => {
    for (const timer of timers.current.values()) clearTimeout(timer)
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
    timers.current.clear()
  }, [])

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-0 bottom-0 z-toast flex max-h-screen flex-col items-end gap-2 overflow-y-auto p-4">
        <AnimatePresence>
          {toasts.map((toast) => (
            <motion.div key={toast.id} role={toast.tone === 'danger' ? 'alert' : 'status'} layout {...toastMotion}>
              {toast.tone === 'danger' ? (
                <div className="pointer-events-auto max-w-[min(520px,calc(100vw-2rem))] rounded-card bg-surface px-3.5 py-3 text-sm leading-snug text-ink shadow-[var(--shadow-raised)] ring-1 ring-danger/25">
                  <div className="flex items-center gap-3">
                    <div className="flex min-w-0 flex-1 items-center gap-2">
                      <span className="size-1.5 shrink-0 rounded-full bg-danger" />
                      <span className="truncate text-[11px] font-semibold uppercase tracking-[0.08em] text-danger">
                        Error
                      </span>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        aria-label="Copy error"
                        title={copiedId === toast.id ? 'Copied' : 'Copy error'}
                        onClick={() => void copy(toast.id, toast.message)}
                        className={actionClass}
                      >
                        {copiedId === toast.id ? <Check size={14} /> : <Copy size={14} />}
                      </button>
                      <button
                        type="button"
                        aria-label="Dismiss notification"
                        title="Dismiss"
                        onClick={() => dismiss(toast.id)}
                        className={actionClass}
                      >
                        <X size={14} />
                      </button>
                    </div>
                  </div>
                  <span className="mt-1.5 block whitespace-pre-wrap break-words text-[13px] leading-[1.55] text-ink select-text">
                    {toast.message}
                  </span>
                </div>
              ) : (
                <div className="pointer-events-auto flex max-w-[min(520px,calc(100vw-2rem))] items-start gap-2 rounded-[var(--radius-control)] bg-bg px-3 py-2 text-sm leading-snug text-ink shadow-md ring-1 ring-border">
                  <span className="min-w-0 whitespace-pre-wrap break-words select-text">{toast.message}</span>
                  <button
                    type="button"
                    aria-label="Dismiss notification"
                    title="Dismiss"
                    onClick={() => dismiss(toast.id)}
                    className={`${actionClass} -my-1 -mr-1`}
                  >
                    <ToastTimer />
                    <X size={13} />
                  </button>
                </div>
              )}
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  )
}
