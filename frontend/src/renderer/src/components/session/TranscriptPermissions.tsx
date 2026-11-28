import { Check, LoaderCircle, X } from 'lucide-react'
import { useState } from 'react'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { useOverflowing } from '@/lib/hooks/useOverflowing'
import { MessageMarkdown } from './MessageMarkdown'
import { QuestionPermissionCard } from '@/components/session/QuestionPermissionCard'
import { VaultApprovalCard, VaultRequestCard } from '@/components/session/VaultCards'
import { hasPermissionSurface, isPlanApprovalPermission, normalized } from './TranscriptUtils'

// A plan-exit ("switch_mode") permission carries the proposed plan as markdown.
// Show it inline, collapsed past a few hundred px so a long plan never balloons
// the approval card.
function PlanPreview({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false)
  const [ref, overflowing] = useOverflowing([text, expanded])

  return (
    <div className="mt-2 rounded-control border border-border bg-bg px-2.5 py-2">
      <div ref={ref} className={`relative ${expanded ? '' : 'max-h-[280px] overflow-hidden'}`}>
        <div className="text-sm text-ink">
          <MessageMarkdown text={text} />
        </div>
        {!expanded && overflowing ? (
          <div
            className="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-b from-transparent to-bg"
            aria-hidden
          />
        ) : null}
      </div>
      {expanded || overflowing ? (
        <button
          type="button"
          onClick={() => setExpanded((value) => !value)}
          className="mt-1.5 inline-flex items-center text-[12px] font-medium text-ink-2 transition-colors hover:text-ink"
        >
          {expanded ? 'Show less' : 'Show full plan'}
        </button>
      ) : null}
    </div>
  )
}

export function PermissionCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const [localSelection, setLocalSelection] = useState('')
  const [submitting, setSubmitting] = useState('')
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  if (!permission) return null
  if (permission.vault_approval) {
    return <VaultApprovalCard event={event} resolution={resolution} />
  }
  if (permission.vault_request) {
    return <VaultRequestCard event={event} resolution={resolution} />
  }
  if (isPlanApprovalPermission(permission)) return null
  if (!hasPermissionSurface(permission)) return null
  if (permission.questions?.length) {
    return <QuestionPermissionCard event={event} resolution={resolution} />
  }

  const selected = localSelection || resolution?.selected_option_id || permission.selected_option_id || ''
  const status = normalized(resolution?.status || permission.status)
  const cancelled = status === 'cancelled'
  const locked = Boolean(selected) || cancelled || Boolean(submitting)

  const choose = async (optionID: string) => {
    setSubmitting(optionID)
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        option_id: optionID,
      })
      setLocalSelection(optionID)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Permission response failed.')
    } finally {
      setSubmitting('')
    }
  }

  const sendText = async () => {
    const trimmed = text.trim()
    if (!trimmed || locked) return
    setSubmitting('text')
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        text: trimmed,
      })
      setText('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Permission response failed.')
    } finally {
      setSubmitting('')
    }
  }

  return (
    <div className="rounded-card border border-border bg-surface px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-ink">{permission.title || 'Permission requested'}</p>
          {permission.locations?.length ? (
            <div className="mt-1 flex flex-wrap gap-1.5">
              {permission.locations.map((location) => (
                <span
                  key={`${location.path}:${location.line ?? 0}`}
                  className="rounded border border-border bg-bg px-1.5 py-px font-mono text-[11px] text-ink-2"
                >
                  {location.path}
                  {location.line ? `:${location.line}` : ''}
                </span>
              ))}
            </div>
          ) : null}
        </div>
        {selected ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-ok">
            <Check className="size-3.5" aria-hidden />
            {permission.options?.find((option) => option.id === selected)?.name || selected}
          </span>
        ) : cancelled ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-danger">
            <X className="size-3.5" aria-hidden />
            Cancelled
          </span>
        ) : null}
      </div>

      {permission.content?.trim() ? <PlanPreview text={permission.content} /> : null}

      {!selected && !cancelled && permission.options?.length ? (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {permission.options.map((option) => (
            <button
              key={option.id}
              type="button"
              disabled={locked}
              onClick={() => void choose(option.id)}
              className="inline-flex h-7 items-center gap-1.5 rounded-full border border-border bg-bg px-2.5 text-[12px] font-medium text-ink transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting === option.id ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
              ) : (
                <Check className="size-3.5" aria-hidden />
              )}
              {option.name}
            </button>
          ))}
        </div>
      ) : null}
      {!selected && !cancelled ? (
        <div className="mt-2 flex items-end gap-1.5">
          <textarea
            value={text}
            rows={1}
            disabled={locked}
            placeholder="Reply with details..."
            className="min-h-8 flex-1 resize-none rounded-control border border-border bg-bg px-2 py-1.5 text-[12px] text-ink placeholder:text-ink-3 disabled:cursor-not-allowed disabled:opacity-60"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void sendText()
              }
            }}
          />
          <button
            type="button"
            disabled={!text.trim() || locked}
            onClick={() => void sendText()}
            className="inline-flex h-8 items-center gap-1.5 rounded-full border border-border bg-bg px-2.5 text-[12px] font-medium text-ink transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting === 'text' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            Reply
          </button>
        </div>
      ) : null}
      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
    </div>
  )
}
