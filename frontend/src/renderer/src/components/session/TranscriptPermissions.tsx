import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Check, ChevronRight, LoaderCircle, X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { useOverflowing } from '@/lib/hooks/useOverflowing'
import { keys } from '@/lib/query/keys'
import { MessageMarkdown } from './MessageMarkdown'
import { hasPermissionSurface, isPlanApprovalPermission, normalized } from './TranscriptUtils'

function QuestionPermissionCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const queryClient = useQueryClient()
  const reduce = useReducedMotion()
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)
  const [localAnswered, setLocalAnswered] = useState(false)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  // Which question is on screen, and the direction we last moved (for the slide).
  const [index, setIndex] = useState(0)
  const [direction, setDirection] = useState(1)
  const advanceTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => () => clearTimeout(advanceTimer.current), [])

  if (!permission?.questions?.length) return null
  const questions = permission.questions

  const status = normalized(resolution?.status || permission.status)
  const answered = localAnswered || status === 'selected' || Boolean(resolution?.selected_option_id)
  const cancelled = status === 'cancelled'
  const settled = answered || cancelled
  const locked = settled || submitting
  const complete = questions.every((question) => answers[question.id]?.trim())

  // Settled questions collapse to a single line, codex-style.
  if (settled && !open) {
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

  const total = questions.length
  const safeIndex = Math.min(index, total - 1)
  const current = questions[safeIndex]
  const isFirst = safeIndex === 0
  const isLast = safeIndex === total - 1
  const options = current.options ?? []
  const selected = answers[current.id] ?? ''
  const showOther = current.is_other || !options.length
  const otherValue =
    !options.length || !options.some((option) => option.label === selected) ? selected : ''

  const setAnswer = (questionID: string, value: string) => {
    setAnswers((prev) => ({ ...prev, [questionID]: value }))
  }

  const goTo = (next: number) => {
    if (next < 0 || next >= total || next === safeIndex) return
    clearTimeout(advanceTimer.current)
    setDirection(next > safeIndex ? 1 : -1)
    setIndex(next)
  }

  // Picking an option settles the question and glides to the next one, so the
  // whole set answers in a quick rhythm of taps — last one waits for Submit.
  const pickOption = (label: string) => {
    if (locked) return
    setAnswer(current.id, label)
    if (isLast) return
    clearTimeout(advanceTimer.current)
    advanceTimer.current = setTimeout(() => goTo(safeIndex + 1), reduce ? 0 : 150)
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

  const slide = {
    enter: (dir: number) => ({ opacity: 0, x: reduce ? 0 : dir >= 0 ? 24 : -24, filter: 'blur(4px)' }),
    center: { opacity: 1, x: 0, filter: 'blur(0px)' },
    exit: (dir: number) => ({ opacity: 0, x: reduce ? 0 : dir >= 0 ? -16 : 16, filter: 'blur(4px)' }),
  }

  return (
    <div className="rounded-card border border-border bg-surface px-3 py-3">
      <div className="flex items-center justify-between gap-3">
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
        ) : (
          <span className="shrink-0 font-mono text-[11px] tabular-nums text-ink-3">
            {safeIndex + 1} / {total}
          </span>
        )}
      </div>

      <div className="relative mt-3 min-h-[124px]">
        <AnimatePresence mode="wait" custom={direction} initial={false}>
          <motion.div
            key={current.id}
            custom={direction}
            variants={slide}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ type: 'spring', duration: 0.18, bounce: 0 }}
          >
            {current.header ? (
              <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">
                {current.header}
              </p>
            ) : null}
            <p className="mt-0.5 text-[15px] leading-snug text-ink text-pretty">{current.question}</p>

            {options.length ? (
              <div className="mt-3 flex flex-col gap-1.5">
                {options.map((option) => {
                  const active = selected === option.label
                  return (
                    <button
                      key={option.label}
                      type="button"
                      disabled={locked}
                      onClick={() => pickOption(option.label)}
                      className={`flex min-h-9 w-full items-center rounded-control border px-3 py-1.5 text-left text-[12px] font-medium transition duration-150 active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-60 ${
                        active
                          ? 'border-primary bg-primary-soft text-primary-strong'
                          : 'border-border bg-bg text-ink hover:border-primary hover:text-primary'
                      }`}
                      title={option.description || option.label}
                    >
                      {option.label}
                    </button>
                  )
                })}
              </div>
            ) : null}

            {showOther ? (
              <input
                type={current.is_secret ? 'password' : 'text'}
                value={otherValue}
                disabled={locked}
                placeholder={options.length ? 'Other answer…' : 'Type your answer…'}
                className={`${options.length ? 'mt-1.5' : 'mt-3'} h-9 w-full rounded-control border border-border bg-bg px-3 text-[12px] text-ink transition-colors placeholder:text-ink-3 focus:border-primary focus:outline-none disabled:cursor-not-allowed disabled:opacity-60`}
                onChange={(e) => setAnswer(current.id, e.target.value)}
                onKeyDown={(e) => {
                  if (e.key !== 'Enter') return
                  e.preventDefault()
                  if (isLast) void submit()
                  else goTo(safeIndex + 1)
                }}
              />
            ) : null}
          </motion.div>
        </AnimatePresence>
      </div>

      <div className="mt-3 flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => goTo(safeIndex - 1)}
          disabled={isFirst}
          className="inline-flex h-8 items-center gap-1 rounded-full px-2.5 text-[12px] font-medium text-ink-2 transition duration-150 hover:text-ink active:scale-[0.96] disabled:pointer-events-none disabled:opacity-0"
        >
          <ArrowLeft className="size-3.5" aria-hidden />
          Back
        </button>

        {total > 1 ? (
          <div className="flex items-center gap-1.5">
            {questions.map((question, dotIndex) => {
              const dotAnswered = Boolean(answers[question.id]?.trim())
              const dotCurrent = dotIndex === safeIndex
              return (
                <button
                  key={question.id}
                  type="button"
                  aria-label={`Go to question ${dotIndex + 1}`}
                  aria-current={dotCurrent}
                  onClick={() => goTo(dotIndex)}
                  className="group grid place-items-center py-1.5"
                >
                  <motion.span
                    layout
                    transition={{ type: 'spring', duration: 0.2, bounce: 0 }}
                    className={`h-1.5 rounded-full transition-colors ${
                      dotCurrent
                        ? 'w-5 bg-primary'
                        : dotAnswered
                          ? 'w-1.5 bg-ink-3 group-hover:bg-ink-2'
                          : 'w-1.5 bg-border group-hover:bg-ink-3'
                    }`}
                  />
                </button>
              )
            })}
          </div>
        ) : (
          <span />
        )}

        {!settled && isLast ? (
          <button
            type="button"
            disabled={!complete || submitting}
            onClick={() => void submit()}
            className="inline-flex h-8 items-center gap-1.5 rounded-full bg-primary px-3.5 text-[12px] font-medium text-on-primary transition duration-150 hover:bg-primary-strong active:scale-[0.96] disabled:cursor-not-allowed disabled:bg-bg disabled:text-ink-3"
          >
            {submitting ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            Submit
          </button>
        ) : !isLast ? (
          <button
            type="button"
            onClick={() => goTo(safeIndex + 1)}
            className="inline-flex h-8 items-center gap-1 rounded-full border border-border bg-bg px-3 text-[12px] font-medium text-ink transition duration-150 hover:border-primary hover:text-primary active:scale-[0.96]"
          >
            Next
            <ArrowRight className="size-3.5" aria-hidden />
          </button>
        ) : (
          <span />
        )}
      </div>

      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
    </div>
  )
}

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
