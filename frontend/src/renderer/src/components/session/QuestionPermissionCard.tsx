import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Check, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type { ACPPermission, ACPQuestion, SessionEvent } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { normalized } from '@/components/session/TranscriptUtils'

type QuestionAnswer = {
  choices: string[]
  other: string
}

function optionText(label: string, description?: string) {
  return description ? `${label} — ${description}` : label
}

function answerText(question: ACPQuestion, value: string) {
  if (question.is_secret) return '••••••••'
  const option = question.options?.find((option) => option.label === value)
  return optionText(value, option?.description)
}

export function QuestionPermissionCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const queryClient = useQueryClient()
  const [answers, setAnswers] = useState<Record<string, QuestionAnswer>>({})
  const [submitting, setSubmitting] = useState(false)
  const [localAnswered, setLocalAnswered] = useState(false)
  const [error, setError] = useState('')
  const [index, setIndex] = useState(0)

  if (!permission?.questions?.length) return null
  const questions = permission.questions

  const status = normalized(resolution?.status || permission.status)
  const answered = localAnswered || status === 'selected' || Boolean(resolution?.selected_option_id)
  const cancelled = status === 'cancelled'
  const settled = answered || cancelled
  const locked = settled || submitting
  const valuesFor = (question: ACPQuestion) => {
    const answer = answers[question.id]
    const other = answer?.other.trim()
    return [...(answer?.choices ?? []), ...(other ? [other] : [])]
  }

  if (settled) {
    const saved = resolution?.answers ?? permission.answers
    return (
      <div className="flex flex-col gap-4 rounded-card border border-border bg-surface p-3">
        {questions.map((question) => {
          const values = saved?.[question.id] ?? (localAnswered ? valuesFor(question) : [])
          return (
            <div key={question.id}>
              <p className="text-sm font-medium leading-snug text-ink text-pretty">{question.question}</p>
              {values.length ? (
                <ul className="mt-2 flex flex-col gap-1.5">
                  {values.map((value, i) => (
                    <li key={i} className="flex items-start justify-between gap-3 text-[13px] leading-5 text-ink-2">
                      <span className="min-w-0 text-pretty wrap-anywhere">{answerText(question, value)}</span>
                      <Check className="mt-0.5 size-3.5 shrink-0 text-ink-3" aria-hidden />
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="mt-1 text-[13px] text-ink-3">
                  {cancelled ? 'Cancelled' : saved || localAnswered ? 'Skipped' : 'Answered'}
                </p>
              )}
            </div>
          )
        })}
      </div>
    )
  }

  const total = questions.length
  const safeIndex = Math.min(index, total - 1)
  const current = questions[safeIndex]
  const isFirst = safeIndex === 0
  const isLast = safeIndex === total - 1
  const options = current.options ?? []
  const selected = answers[current.id]?.choices ?? []
  const showOther = current.is_other || !options.length
  const otherValue = answers[current.id]?.other ?? ''

  const pickOption = (label: string) => {
    setAnswers((prev) => {
      const answer = prev[current.id]
      const choices = answer?.choices ?? []
      return {
        ...prev,
        [current.id]: {
          choices: current.multi_select
            ? choices.includes(label)
              ? choices.filter((value) => value !== label)
              : [...choices, label]
            : [label],
          other: current.multi_select ? answer?.other ?? '' : '',
        },
      }
    })
  }

  const submit = async () => {
    if (locked) return
    setSubmitting(true)
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        answers: Object.fromEntries(
          questions.map((question) => [
            question.id,
            { answers: valuesFor(question) },
          ]),
        ),
      })
      setLocalAnswered(true)
      await queryClient.invalidateQueries({ queryKey: keys.sessionMessages(event.session_id) })
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Question response failed.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="rounded-card border border-border bg-surface px-3 pt-3 pb-2">
      <p className="text-sm font-medium leading-snug text-ink text-pretty">{current.question}</p>

      <fieldset disabled={locked} className="-mx-2 mt-2 flex flex-col">
        <legend className="sr-only">{current.question}</legend>
        {options.map((option) => (
          <label
            key={option.label}
            className={`flex items-start gap-2.5 rounded-lg px-2 py-2.5 text-[13px] leading-5 text-ink transition-colors has-focus-visible:bg-list-hover ${locked ? 'cursor-not-allowed opacity-60' : 'cursor-pointer hover:bg-list-hover'}`}
          >
            <input
              type={current.multi_select ? 'checkbox' : 'radio'}
              name={`${permission.id}:${current.id}`}
              checked={selected.includes(option.label)}
              onChange={() => pickOption(option.label)}
              className="mt-0.75 size-3.5 shrink-0 accent-ink"
            />
            <span className="min-w-0 text-pretty wrap-anywhere">{optionText(option.label, option.description)}</span>
          </label>
        ))}

        {showOther ? (
          <input
            type={current.is_secret ? 'password' : 'text'}
            value={otherValue}
            aria-label={options.length ? 'Other answer' : current.question}
            placeholder={options.length ? 'Other answer…' : 'Type your answer…'}
            className="mt-1 h-10 w-full rounded-lg bg-bg px-2 text-[13px] text-ink placeholder:text-ink-3 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
            onChange={(e) => {
              const value = e.target.value
              setAnswers((prev) => ({
                ...prev,
                [current.id]: {
                  choices: current.multi_select ? prev[current.id]?.choices ?? [] : [],
                  other: value,
                },
              }))
            }}
            onKeyDown={(e) => {
              if (e.key !== 'Enter') return
              e.preventDefault()
              if (isLast) void submit()
              else setIndex(safeIndex + 1)
            }}
          />
        ) : null}
      </fieldset>

      <div className="mt-2 flex items-center justify-end gap-1">
        {total > 1 ? (
          <div
            role="progressbar"
            aria-label="Question progress"
            aria-valuemin={0}
            aria-valuemax={total}
            aria-valuenow={safeIndex + 1}
            aria-valuetext={`Question ${safeIndex + 1} of ${total}`}
            className="mr-auto h-1 w-16 overflow-hidden rounded-full bg-ink/10"
          >
            <div className="h-full rounded-full bg-ink-3" style={{ width: `${((safeIndex + 1) / total) * 100}%` }} />
          </div>
        ) : null}
        {isFirst ? null : (
          <button
            type="button"
            onClick={() => setIndex(safeIndex - 1)}
            className="relative inline-flex h-8 items-center gap-1 rounded-full pr-3 pl-2.5 text-[13px] text-ink-2 transition duration-150 after:absolute after:inset-x-0 after:-inset-y-1 hover:text-ink active:scale-[0.96]"
          >
            <ArrowLeft className="size-3.5" aria-hidden />
            Back
          </button>
        )}
        {!isLast ? (
          <button
            type="button"
            onClick={() => setIndex(safeIndex + 1)}
            className="relative inline-flex h-8 items-center gap-1 rounded-full bg-surface-2 pr-2.5 pl-3 text-[13px] font-medium text-ink transition duration-150 after:absolute after:inset-x-0 after:-inset-y-1 hover:bg-list-active active:scale-[0.96]"
          >
            Next
            <ArrowRight className="size-3.5" aria-hidden />
          </button>
        ) : (
          <button
            type="button"
            disabled={submitting}
            onClick={() => void submit()}
            className="relative inline-flex h-8 items-center gap-1.5 rounded-full bg-primary px-3.5 text-[13px] font-medium text-on-primary transition duration-150 after:absolute after:inset-x-0 after:-inset-y-1 hover:bg-primary-strong active:scale-[0.96] disabled:cursor-not-allowed disabled:bg-surface-2 disabled:text-ink-3"
          >
            {submitting ? <LoaderCircle className="size-3.5 animate-spin" aria-hidden /> : null}
            Submit
          </button>
        )}
      </div>

      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
    </div>
  )
}
