import { AnimatePresence, motion } from 'motion/react'
import { Download, RefreshCw, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { UpdateStatus } from '../../../../shared/update'
import { Button } from '@/components/ui/Button'

type VisibleUpdateStatus = Exclude<UpdateStatus, { state: 'idle' }>

function updateKey(status: UpdateStatus): string {
  return `${status.state}:${'version' in status ? status.version ?? '' : ''}`
}

function updateText(status: VisibleUpdateStatus) {
  switch (status.state) {
    case 'available':
      return {
        title: 'Update available',
        detail: status.version ? `Jaz ${status.version} is downloading.` : 'A new Jaz update is downloading.',
      }
    case 'checking':
      return {
        title: 'Checking for updates',
        detail: 'Looking for the latest Jaz release.',
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

function UpdateNotice({
  status,
  onDismiss,
}: {
  status: VisibleUpdateStatus
  onDismiss: () => void
}) {
  const text = updateText(status)
  const percent = status.state === 'downloading' ? Math.round(status.percent) : 0
  const iconTone =
    status.state === 'error' ? 'bg-danger-soft text-danger' : 'bg-primary-soft text-primary'

  return (
    <motion.section
      role={status.state === 'error' ? 'alert' : 'status'}
      aria-live="polite"
      layout
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 3 }}
      transition={{ type: 'spring', duration: 0.24, bounce: 0 }}
      className="overflow-hidden rounded-card bg-bg/70 p-2.5 text-[13px] text-ink ring-1 ring-border/70"
    >
      <div className="flex items-start gap-2">
        <div className={`mt-0.5 grid size-7 shrink-0 place-items-center rounded-full ${iconTone}`}>
          {status.state === 'error' ? (
            <X size={14} />
          ) : status.state === 'checking' || status.state === 'downloaded' ? (
            <RefreshCw size={14} />
          ) : (
            <Download size={14} />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium text-ink">{text.title}</div>
          <div className="mt-0.5 line-clamp-2 text-[12px] leading-4 text-ink-2">{text.detail}</div>
        </div>
        <button
          type="button"
          aria-label="Dismiss update"
          title="Dismiss"
          onClick={onDismiss}
          className="-mt-1 -mr-1 grid size-7 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
        >
          <X size={14} />
        </button>
      </div>

      {status.state === 'downloading' ? (
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-surface-2">
          <div
            className="h-full origin-left rounded-full bg-primary transition-transform duration-300"
            style={{ transform: `scaleX(${Math.max(0, Math.min(100, percent)) / 100})` }}
          />
        </div>
      ) : null}

      {status.state === 'downloaded' ? (
        <div className="mt-2 flex justify-end gap-1">
          <Button size="sm" variant="ghost" onClick={onDismiss}>
            Later
          </Button>
          <Button size="sm" variant="primary" onClick={() => void window.jaz.installUpdate()}>
            Restart Jaz
          </Button>
        </div>
      ) : null}
    </motion.section>
  )
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

  return (
    <AnimatePresence initial={false}>
      {status.state !== 'idle' && dismissedKey !== key ? (
        <UpdateNotice key={key} status={status} onDismiss={() => setDismissedKey(key)} />
      ) : null}
    </AnimatePresence>
  )
}
