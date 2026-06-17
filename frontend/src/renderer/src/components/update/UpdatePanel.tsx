import { AnimatePresence, motion } from 'motion/react'
import { Download, RefreshCw, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { UpdateStatus } from '../../../../shared/update'
import { Button } from '@/components/ui/Button'

function updateKey(status: UpdateStatus): string {
  return `${status.state}:${'version' in status ? status.version ?? '' : ''}`
}

function updateText(status: Exclude<UpdateStatus, { state: 'idle' }>) {
  switch (status.state) {
    case 'available':
      return {
        title: 'Update available',
        detail: status.version ? `Jaz ${status.version} is downloading.` : 'A new Jaz update is downloading.',
      }
    case 'downloading':
      return {
        title: 'Downloading update',
        detail: `${Math.round(status.percent)}% downloaded`,
      }
    case 'downloaded':
      return {
        title: 'Update ready',
        detail: status.version ? `Jaz ${status.version} is ready.` : 'A Jaz update is ready.',
      }
    case 'error':
      return {
        title: 'Update failed',
        detail: status.message,
      }
  }
}

export function UpdatePanel() {
  const [status, setStatus] = useState<UpdateStatus>({ state: 'idle' })
  const [dismissedKey, setDismissedKey] = useState<string | null>(null)
  const key = useMemo(() => updateKey(status), [status])

  useEffect(() => {
    let mounted = true
    void window.jaz.getUpdateStatus().then((next) => {
      if (mounted) setStatus(next)
    })
    const dispose = window.jaz.onUpdateStatus((next) => setStatus(next))
    return () => {
      mounted = false
      dispose()
    }
  }, [])

  if (status.state === 'idle' || dismissedKey === key) return null

  const text = updateText(status)
  const percent = status.state === 'downloading' ? Math.round(status.percent) : 0

  return (
    <div className="pointer-events-none fixed top-16 right-4 z-toast w-[min(360px,calc(100vw-2rem))]">
      <AnimatePresence>
        <motion.section
          key={key}
          role={status.state === 'error' ? 'alert' : 'status'}
          aria-live="polite"
          initial={{ opacity: 0, y: -6, scale: 0.98 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: -4, scale: 0.98 }}
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          className="pointer-events-auto overflow-hidden rounded-card bg-surface p-3 text-sm text-ink shadow-raised backdrop-blur"
        >
          <div className="flex items-start gap-2.5">
            <div className="grid size-8 shrink-0 place-items-center rounded-[10px] bg-primary-soft text-primary">
              {status.state === 'downloaded' ? <RefreshCw size={16} /> : <Download size={16} />}
            </div>
            <div className="min-w-0 flex-1">
              <div className="font-medium text-ink">{text.title}</div>
              <div className="mt-0.5 line-clamp-2 text-[12px] leading-5 text-ink-2">{text.detail}</div>
            </div>
            <button
              type="button"
              aria-label="Dismiss update"
              title="Dismiss"
              onClick={() => setDismissedKey(key)}
              className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
            >
              <X size={15} />
            </button>
          </div>

          {status.state === 'downloading' ? (
            <div className="mt-3">
              <div className="h-1.5 overflow-hidden rounded-full bg-surface-2">
                <div
                  className="h-full origin-left rounded-full bg-primary transition-transform duration-300"
                  style={{ transform: `scaleX(${Math.max(0, Math.min(100, percent)) / 100})` }}
                />
              </div>
              <div className="mt-1.5 text-right text-[11px] tabular-nums text-ink-3">{percent}%</div>
            </div>
          ) : null}

          {status.state === 'downloaded' ? (
            <div className="mt-3 flex justify-end gap-1.5">
              <Button size="sm" variant="ghost" onClick={() => setDismissedKey(key)}>
                Later
              </Button>
              <Button size="sm" variant="primary" onClick={() => void window.jaz.installUpdate()}>
                Restart
              </Button>
            </div>
          ) : null}
        </motion.section>
      </AnimatePresence>
    </div>
  )
}
