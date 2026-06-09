import type { Session } from '@/lib/api/types'

// `compact` drops the model and shows only the provider/agent name — used in
// the cramped sidebar rows, where the full model still surfaces on hover.
export function RuntimeBadge({
  session,
  className = '',
  compact = false,
  truncate = true,
}: {
  session: Session
  className?: string
  compact?: boolean
  truncate?: boolean
}) {
  const model = compactModel(session.model)
  const modelLabel = withReasoningEffort(model, session.reasoning_effort)
  const fullModelLabel = session.model
    ? withReasoningEffort(session.model, session.reasoning_effort)
    : ''
  // Sidebar rows are cramped, so they clamp + truncate; roomier spots (the
  // titlebar) opt out and show the full provider · model label.
  const clamp = truncate ? 'min-w-0 max-w-[11rem] truncate' : 'whitespace-nowrap'
  const base = `inline-block ${clamp} rounded px-1.5 py-px font-mono text-[11px]`
  if (session.runtime === 'acp') {
    const agent = session.runtime_ref?.agent ?? 'acp'
    return (
      <span
        title={fullModelLabel ? `${agent}: ${fullModelLabel}` : agent}
        className={`${base} text-accent-strong bg-accent-soft ${className}`}
      >
        {!compact && modelLabel ? `${agent} · ${modelLabel}` : agent}
      </span>
    )
  }
  const provider = session.model_provider || 'native'
  return (
    <span
      title={fullModelLabel ? `${provider}: ${fullModelLabel}` : provider}
      className={`${base} text-ink-2 bg-surface-2 ${className}`}
    >
      {modelLabel ? `${provider} · ${modelLabel}` : provider}
    </span>
  )
}

function compactModel(model?: string): string {
  if (!model) return ''
  const parts = model.split('/').filter(Boolean)
  return parts.at(-1) ?? model
}

function withReasoningEffort(model: string, effort?: string): string {
  if (!model) return ''
  return effort ? `${model}/${effort}` : model
}
