import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { agentLabel } from '@/lib/agentLabel'
import type { Session } from '@/lib/api/types'

// Runtime tag: a pill carrying the *identity* (the ACP agent, prettified, or
// the native provider) tinted by runtime, with the model as muted mono detail
// beside it. `compact` (sidebar) drops the model — it still surfaces on hover.
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
  const isACP = session.runtime === 'acp'
  const agent = session.runtime_ref?.agent
  // Agent names arrive as slugs; prettify so the tag reads like a product name
  // rather than a raw identifier.
  const name = isACP ? agentLabel(agent) : session.model_provider || 'native'
  const title = fullModelLabel ? `${name} · ${fullModelLabel}` : name
  // Known agents wear their brand mark instead of a text tag; native sessions
  // (and any agent we don't have a mark for) keep the prettified name pill.
  const showLogo = isACP && hasAgentLogo(agent ?? '')
  // Agent-backed sessions get the emphasized brand-soft fill (the same pill
  // the loops list uses for its active state); native stays a quieter neutral.
  const pillTone = isACP ? 'bg-primary-soft text-primary-strong' : 'bg-surface-2 text-ink-2'

  return (
    <span title={title} className={`inline-flex min-w-0 items-center gap-1.5 ${className}`}>
      {showLogo ? (
        // Bare brand mark, tinted to ink — no chip behind it. Keeps the text
        // pill's px-1.5 so the sidebar's leading negative margin still lands the
        // mark where the name used to sit.
        <span className="inline-flex shrink-0 items-center px-1.5 text-ink">
          <AgentLogo agent={agent ?? ''} size={16} />
        </span>
      ) : (
        <span
          className={`shrink-0 rounded-full px-1.5 py-[3px] text-[11px] leading-none font-medium ${pillTone} ${
            truncate ? 'max-w-[11rem] truncate' : ''
          }`}
        >
          {name}
        </span>
      )}
      {!compact && modelLabel ? (
        <span
          className={`font-mono text-[11px] text-ink-3 ${
            truncate ? 'min-w-0 truncate' : 'whitespace-nowrap'
          }`}
        >
          {modelLabel}
        </span>
      ) : null}
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
