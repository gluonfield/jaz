import type { Session } from '@/lib/api/types'

export function RuntimeBadge({ session }: { session: Session }) {
  if (session.runtime === 'acp') {
    return (
      <span className="rounded px-1.5 py-px font-mono text-[11px] text-accent-strong bg-accent-soft">
        {session.runtime_ref?.agent ?? 'acp'}
      </span>
    )
  }
  return (
    <span className="rounded px-1.5 py-px font-mono text-[11px] text-ink-2 bg-surface-2">
      native
    </span>
  )
}
