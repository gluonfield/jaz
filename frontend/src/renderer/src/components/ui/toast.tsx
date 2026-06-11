import { AnimatePresence, motion } from 'motion/react'
import { X } from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'

interface Toast {
  id: number
  message: string
  tone: 'ok' | 'danger'
}

const TOAST_AUTO_DISMISS_MS = 30_000

const ToastContext = createContext<(message: string, tone?: Toast['tone']) => void>(() => {})

export function useToast() {
  return useContext(ToastContext)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(0)
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>())

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id)
    if (timer) clearTimeout(timer)
    timers.current.delete(id)
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback((message: string, tone: Toast['tone'] = 'ok') => {
    const id = nextId.current++
    setToasts((prev) => [...prev, { id, message, tone }])
    timers.current.set(id, setTimeout(() => dismiss(id), TOAST_AUTO_DISMISS_MS))
  }, [dismiss])

  useEffect(() => () => {
    for (const timer of timers.current.values()) clearTimeout(timer)
    timers.current.clear()
  }, [])

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-4 bottom-4 z-toast flex max-h-[calc(100vh-2rem)] flex-col items-end gap-2 overflow-y-auto">
        <AnimatePresence>
          {toasts.map((toast) => (
            <motion.div
              key={toast.id}
              role={toast.tone === 'danger' ? 'alert' : 'status'}
              layout
              initial={{ opacity: 0, y: 8, scale: 0.97 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 4, scale: 0.97 }}
              transition={{ type: 'spring', stiffness: 420, damping: 30 }}
              className={`pointer-events-auto flex max-w-[min(520px,calc(100vw-2rem))] items-start gap-2 rounded-[var(--radius-control)] border px-3 py-2 text-sm leading-snug shadow-md ${
                toast.tone === 'ok'
                  ? 'border-border bg-bg text-ink'
                  : 'border-danger/30 bg-danger-soft text-danger'
              }`}
            >
              <span className="min-w-0 whitespace-pre-wrap break-words">{toast.message}</span>
              <button
                type="button"
                aria-label="Dismiss notification"
                title="Dismiss"
                onClick={() => dismiss(toast.id)}
                className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-current opacity-65 transition-opacity hover:opacity-100"
              >
                <X size={13} />
              </button>
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  )
}
