import { KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import type { BrowserPasswordsController } from '@/components/browser/useBrowserPasswords'

function displayOrigin(origin: string): string {
  try {
    return new URL(origin).host
  } catch {
    return origin
  }
}

export function BrowserPasswordBanner({
  passwords,
  visible,
}: {
  passwords: BrowserPasswordsController
  visible: boolean
}) {
  const { api, state, busy, error, act } = passwords
  const pending = state.pending
  if (!api || !visible || !pending) {
    return null
  }
  const host = displayOrigin(pending.origin)
  const message = error || state.error
  return (
    <div
      role="status"
      aria-label="Browser passwords"
      className="flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-1.5 border-b border-border bg-surface/90 px-3 py-1.5 text-[12px] text-ink"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <KeyRound size={13} className="shrink-0 text-primary" aria-hidden />
        <span className="min-w-0 truncate">
          <span className="font-medium">{pending.update ? 'Update password' : 'Save password'}</span>
          {pending.username ? <span className="text-ink-2"> for {pending.username}</span> : null}
          {host ? <span className="text-ink-3"> on {host}</span> : null}
        </span>
      </div>
      {message ? <span role="alert" className="text-[11px] text-danger">{message}</span> : null}
      <div className="flex shrink-0 items-center gap-1.5">
        <Button
          size="sm"
          variant="ghost"
          className="h-6 px-2"
          disabled={busy}
          onClick={() => void act({ kind: 'dismiss', id: pending.id })}
        >
          Not now
        </Button>
        <Button
          size="sm"
          variant="primary"
          className="h-6 px-2.5"
          disabled={busy}
          onClick={() => void act({ kind: 'save', id: pending.id })}
        >
          {pending.update ? 'Update' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
