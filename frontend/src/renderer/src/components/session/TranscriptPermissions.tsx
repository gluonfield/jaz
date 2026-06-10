import { useQueryClient } from '@tanstack/react-query'
import { Check, ChevronRight, LoaderCircle, X } from 'lucide-react'
import { useState } from 'react'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { hasPermissionSurface, normalized } from './TranscriptUtils'

function QuestionPermissionCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const queryClient = useQueryClient()
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)
  const [localAnswered, setLocalAnswered] = useState(false)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  if (!permission?.questions?.length) return null
  const questions = permission.questions

  const status = normalized(resolution?.status || permission.status)
  const answered = localAnswered || status === 'selected' || Boolean(resolution?.selected_option_id)
  const cancelled = status === 'cancelled'
  const locked = answered || cancelled || submitting
  const complete = questions.every((question) => answers[question.id]?.trim())

  // Settled questions collapse to a single line, codex-style.
  if ((answered || cancelled) && !open) {
    return (
      <button
        type="button"
        aria-expanded={false}
        onClick={() => setOpen(true)}
        className="inline-flex min-h-7 items-center gap-1.5 self-start rounded-full px-1 text-left font-mono text-[12px] text-ink-3 transition-colors hover:text-ink"
      >
        <ChevronRight size={12} className="shrink-0" aria-hidden />
        Asked {questions.length} question{questions.length === 1 ? '' : 's'}
        {cancelled ? ' · cancelled' : ''}
      </button>
    )
  }

  const setAnswer = (questionID: string, value: string) => {
    setAnswers((current) => ({ ...current, [questionID]: value }))
  }

  const submit = async () => {
    if (!complete || locked) return
    setSubmitting(true)
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        answers: Object.fromEntries(
          questions.map((question) => [
            question.id,
            { answers: [answers[question.id].trim()] },
          ]),
        ),
      })
      setLocalAnswered(true)
      await queryClient.invalidateQueries({ queryKey: keys.sessionMessages(event.session_id) })
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Question response failed.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="rounded-card border border-border bg-surface px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <p className="text-sm font-medium text-ink">{permission.title || 'Clarifying questions'}</p>
        {answered ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-ok">
            <Check className="size-3.5" aria-hidden />
            Answered
          </span>
        ) : cancelled ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-danger">
            <X className="size-3.5" aria-hidden />
            Cancelled
          </span>
        ) : null}
      </div>

      <div className="mt-3 flex flex-col gap-3">
        {questions.map((question, index) => {
          const selected = answers[question.id] ?? ''
          const options = question.options ?? []
          return (
            <div key={question.id} className="rounded-control bg-bg px-2.5 py-2">
              <div className="flex gap-2">
                <span className="mt-0.5 font-mono text-[11px] text-ink-3">{index + 1}</span>
                <div className="min-w-0 flex-1">
                  {question.header ? (
                    <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">
                      {question.header}
                    </p>
                  ) : null}
                  <p className="text-sm text-ink">{question.question}</p>
                  {options.length ? (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {options.map((option) => (
                        <button
                          key={option.label}
                          type="button"
                          disabled={locked}
                          onClick={() => setAnswer(question.id, option.label)}
                          className={`inline-flex min-h-8 items-center rounded-full border px-2.5 py-1 text-left text-[12px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${
                            selected === option.label
                              ? 'border-primary bg-primary-soft text-primary-strong'
                              : 'border-border bg-surface text-ink hover:border-primary hover:text-primary'
                          }`}
                          title={option.description || option.label}
                        >
                          {option.label}
                        </button>
                      ))}
                    </div>
                  ) : null}
                  {question.is_other || !options.length ? (
                    <input
                      type={question.is_secret ? 'password' : 'text'}
                      value={!options.length || !options.some((option) => option.label === selected) ? selected : ''}
                      disabled={locked}
                      placeholder={options.length ? 'Other answer...' : 'Answer...'}
                      className="mt-2 h-8 w-full rounded-full border border-border bg-surface px-3 text-[12px] text-ink placeholder:text-ink-3 disabled:cursor-not-allowed disabled:opacity-60"
                      onChange={(e) => setAnswer(question.id, e.target.value)}
                    />
                  ) : null}
                </div>
              </div>
            </div>
          )
        })}
      </div>

      {!answered && !cancelled ? (
        <div className="mt-3 flex justify-end">
          <button
            type="button"
            disabled={!complete || locked}
            onClick={() => void submit()}
            className="inline-flex h-8 items-center gap-1.5 rounded-full bg-primary px-3.5 text-[12px] font-medium text-on-primary transition hover:bg-primary-strong disabled:cursor-not-allowed disabled:bg-bg disabled:text-ink-3"
          >
            {submitting ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            Submit answers
          </button>
        </div>
      ) : null}
      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
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
