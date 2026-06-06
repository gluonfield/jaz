const STATE_STYLES: Record<string, { dot: string; label: string }> = {
  starting: { dot: 'bg-running', label: 'starting' },
  running: { dot: 'bg-running', label: 'running' },
  idle: { dot: 'bg-ok', label: 'idle' },
  cancelled: { dot: 'bg-ink-3', label: 'cancelled' },
  failed: { dot: 'bg-danger', label: 'failed' },
}

export function AcpStateBadge({ state }: { state: string }) {
  const style = STATE_STYLES[state] ?? { dot: 'bg-ink-3', label: state }
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-bg px-2.5 py-0.5 text-[12px] text-ink-2">
      <span className={`size-2 rounded-full ${style.dot}`} />
      {style.label}
    </span>
  )
}
