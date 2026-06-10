import { AnimatePresence, motion } from 'motion/react'
import { createContext, useCallback, useContext, useRef, useState } from 'react'
import type { ReactNode } from 'react'

interface Toast {
  id: number
  message: string
  tone: 'ok' | 'danger'
}

const ToastContext = createContext<(message: string, tone?: Toast['tone']) => void>(() => {})

export function useToast() {
  return useContext(ToastContext)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(0)

  const push = useCallback((message: string, tone: Toast['tone'] = 'ok') => {
    const id = nextId.current++
    setToasts((prev) => [...prev, { id, message, tone }])
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id))
    }, 2_500)
  }, [])

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-4 bottom-4 z-toast flex flex-col items-end gap-2">
        <AnimatePresence>
          {toasts.map((toast) => (
            <motion.div
              key={toast.id}
              role="status"
              layout
              initial={{ opacity: 0, y: 8, scale: 0.97 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 4, scale: 0.97 }}
              transition={{ type: 'spring', stiffness: 420, damping: 30 }}
              className={`rounded-full border px-4 py-2 text-sm shadow-md ${
                toast.tone === 'ok'
                  ? 'border-border bg-bg text-ink'
                  : 'border-danger/30 bg-danger-soft text-danger'
              }`}
            >
              {toast.message}
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  )
}
