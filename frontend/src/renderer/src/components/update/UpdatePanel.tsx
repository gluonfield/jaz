import { AnimatePresence, motion } from 'motion/react'
import { Download, RefreshCw, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { UpdateStatus } from '@shared/update'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { clientRuntime } from '@/lib/clientRuntime'

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
  const percent = status.state === 'downloading' ? Math.max(0, Math.min(100, Math.round(status.percent))) : 0
  const Icon = status.state === 'error' ? X : status.state === 'checking' || status.state === 'downloaded' ? RefreshCw : Download

  return (
    <motion.section
      role={status.state === 'error' ? 'alert' : 'status'}
      aria-live="polite"
      layout
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 3 }}
      transition={{ type: 'spring', duration: 0.24, bounce: 0 }}
      className="px-2.5 py-2 text-[13px] text-ink"
    >
      <div className="flex items-start gap-2">
        <div aria-hidden className={`mt-0.5 grid size-[18px] shrink-0 place-items-center ${status.state === 'error' ? 'text-danger' : 'text-ink-2'}`}>
          <Icon size={16} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0 font-medium">{text.title}</div>
            {status.state === 'downloading' ? <span className="shrink-0 text-[12px] tabular-nums text-ink-2">{percent}%</span> : null}
          </div>
          {status.state === 'downloading' ? (
            <div role="progressbar" aria-label={text.title} aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100} className="mt-2 h-0.5 overflow-hidden rounded-full bg-border">
              <div
                className="h-full origin-left rounded-full bg-primary transition-transform duration-300 motion-reduce:transition-none"
                style={{ transform: `scaleX(${percent / 100})` }}
              />
            </div>
          ) : <div className="mt-0.5 line-clamp-2 text-[12px] leading-4 text-ink-2">{text.detail}</div>}
        </div>
        <IconButton
          size="xs"
          aria-label="Dismiss update"
          title="Dismiss"
          onClick={onDismiss}
          className="-mt-0.5 -mr-1"
        >
          <X size={13} />
        </IconButton>
      </div>

      {status.state === 'downloaded' ? (
        <div className="mt-2 flex justify-end gap-1">
          <Button size="sm" variant="ghost" onClick={onDismiss}>
            Later
          </Button>
          <Button size="sm" variant="primary" onClick={() => void clientRuntime.installUpdate?.()}>
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
    if (!clientRuntime.getUpdateStatus || !clientRuntime.onUpdateStatus) return
    let mounted = true
    void clientRuntime.getUpdateStatus().then((next) => {
      if (mounted) setStatus(next)
    })
    const dispose = clientRuntime.onUpdateStatus((next) => setStatus(next))
    return () => {
      mounted = false
      dispose()
    }
  }, [])

  if (!clientRuntime.capabilities.updates) return null

  return (
    <AnimatePresence initial={false}>
      {status.state !== 'idle' && dismissedKey !== key ? (
        <UpdateNotice key={key} status={status} onDismiss={() => setDismissedKey(key)} />
      ) : null}
    </AnimatePresence>
  )
}
