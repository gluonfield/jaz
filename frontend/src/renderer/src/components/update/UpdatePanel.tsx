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
        title: status.error ? 'Restart failed' : 'Update ready',
        detail: status.error ?? (status.version ? `Jaz ${status.version} is ready.` : 'A Jaz update is ready.'),
      }
    case 'installing':
      return {
        title: 'Restarting Jaz',
        detail: 'Preparing the update. Jaz will reopen automatically.',
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
  onInstall,
}: {
  status: VisibleUpdateStatus
  onDismiss: () => void
  onInstall: () => void
}) {
  const text = updateText(status)
  const percent = status.state === 'downloading' ? Math.max(0, Math.min(100, Math.round(status.percent))) : 0
  const installing = status.state === 'installing'
  const failed = status.state === 'error' || (status.state === 'downloaded' && !!status.error)
  const Icon = failed ? X : status.state === 'checking' || status.state === 'downloaded' || installing ? RefreshCw : Download

  return (
    <motion.section
      role={failed ? 'alert' : 'status'}
      aria-live="polite"
      layout
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 3 }}
      transition={{ type: 'spring', duration: 0.24, bounce: 0 }}
      className="px-2.5 py-2 text-[13px] text-ink"
    >
      <div className="flex items-start gap-2">
        <div aria-hidden className={`mt-0.5 grid size-[18px] shrink-0 place-items-center ${failed ? 'text-danger' : 'text-ink-2'}`}>
          <Icon size={16} className={installing ? 'animate-spin motion-reduce:animate-none' : undefined} />
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
        {!installing ? <IconButton
          size="xs"
          aria-label="Dismiss update"
          title="Dismiss"
          onClick={onDismiss}
          className="-mt-0.5 -mr-1"
        >
          <X size={13} />
        </IconButton> : null}
      </div>

      {status.state === 'downloaded' || installing ? (
        <div className="mt-2 flex justify-end gap-1">
          {!installing ? <Button size="sm" variant="ghost" onClick={onDismiss}>
            Later
          </Button> : null}
          <Button size="sm" variant="primary" disabled={installing} aria-busy={installing} onClick={onInstall}>
            {installing ? 'Restarting…' : status.error ? 'Try again' : 'Restart Jaz'}
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

  async function install(): Promise<void> {
    if (status.state !== 'downloaded' || !clientRuntime.installUpdate) return
    const ready = status
    setStatus({ state: 'installing', version: ready.version })
    try {
      const result = await clientRuntime.installUpdate()
      if (!result.ok) setStatus({ ...ready, error: result.error ?? 'Could not restart Jaz.' })
    } catch (err) {
      setStatus({ ...ready, error: err instanceof Error ? err.message : String(err) })
    }
  }

  useEffect(() => {
    if (!clientRuntime.getUpdateStatus || !clientRuntime.onUpdateStatus) return
    let mounted = true
    let received = false
    const dispose = clientRuntime.onUpdateStatus((next) => {
      received = true
      setStatus(next)
    })
    void clientRuntime.getUpdateStatus().then((next) => {
      if (mounted && !received) setStatus(next)
    })
    return () => {
      mounted = false
      dispose()
    }
  }, [])

  if (!clientRuntime.capabilities.updates) return null

  return (
    <AnimatePresence initial={false}>
      {status.state !== 'idle' && dismissedKey !== key ? (
        <UpdateNotice status={status} onDismiss={() => setDismissedKey(key)} onInstall={() => void install()} />
      ) : null}
    </AnimatePresence>
  )
}
