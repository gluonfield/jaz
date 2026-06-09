import { CalendarClock } from 'lucide-react'
import {
  type ScheduleDraft,
  type SchedulePreset,
  SCHEDULE_PRESETS,
  WEEKDAY_LABELS,
  cronFromDraft,
  describeSchedule,
  nextRuns,
} from './schedule'

const fieldClass =
  'rounded-control border border-border bg-bg px-2.5 py-1.5 text-[13px] text-ink outline-none transition-colors duration-150 focus:border-primary focus:ring-2 focus:ring-primary/15'

function formatRun(d: Date): string {
  return d.toLocaleString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

export function SchedulePicker({
  value,
  disabled,
  onChange,
}: {
  value: ScheduleDraft
  disabled?: boolean
  onChange: (next: ScheduleDraft) => void
}) {
  const set = (patch: Partial<ScheduleDraft>) => onChange({ ...value, ...patch })
  const showTime = value.preset === 'daily' || value.preset === 'weekdays' || value.preset === 'weekly'
  const runs = value.preset === 'manual' ? [] : nextRuns(cronFromDraft(value), 3)

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-1 sm:grid-cols-6">
        {SCHEDULE_PRESETS.map((preset) => {
          const active = value.preset === preset.value
          return (
            <button
              key={preset.value}
              type="button"
              disabled={disabled}
              onClick={() => set({ preset: preset.value })}
              aria-pressed={active}
              className={`rounded-control border px-2 py-1.5 text-[12px] font-medium transition-colors duration-150 disabled:opacity-50 ${
                active
                  ? 'border-primary bg-primary-soft text-primary-strong'
                  : 'border-border bg-bg text-ink-2 hover:border-ink-3/40 hover:text-ink'
              }`}
            >
              {preset.label}
            </button>
          )
        })}
      </div>

      {(showTime || value.preset === 'weekly') && (
        <div className="flex flex-wrap items-center gap-2">
          {value.preset === 'weekly' && (
            <select
              aria-label="Day of week"
              disabled={disabled}
              value={String(value.weekday)}
              onChange={(e) => set({ weekday: Number(e.target.value) })}
              className={`${fieldClass} cursor-pointer pr-7 disabled:opacity-50`}
            >
              {WEEKDAY_LABELS.map((label, index) => (
                <option key={label} value={index}>
                  {label}
                </option>
              ))}
            </select>
          )}
          {showTime && (
            <label className="flex items-center gap-2 text-[13px] text-ink-2">
              <span>At</span>
              <input
                type="time"
                disabled={disabled}
                value={value.time}
                onChange={(e) => set({ time: e.target.value })}
                className={fieldClass}
              />
            </label>
          )}
        </div>
      )}

      {value.preset === 'custom' && (
        <input
          type="text"
          spellCheck={false}
          disabled={disabled}
          value={value.expr}
          onChange={(e) => set({ expr: e.target.value })}
          placeholder="*/30 * * * *  (min hour day month weekday)"
          className={`w-full font-mono ${fieldClass}`}
        />
      )}

      <div className="rounded-control border border-border/70 bg-surface px-3 py-2">
        <div className="flex items-center gap-1.5 text-[12px] font-medium text-ink-2">
          <CalendarClock size={13} className="text-ink-3" />
          {describeSchedule(value)}
        </div>
        {value.preset !== 'manual' ? (
          <div className="mt-1.5 flex flex-col gap-0.5 pl-[18px]">
            {runs.length > 0 ? (
              runs.map((run) => (
                <span key={run.getTime()} className="text-[12px] tabular-nums text-ink-3">
                  {formatRun(run)}
                </span>
              ))
            ) : (
              <span className="text-[12px] text-ink-3">
                {value.preset === 'custom'
                  ? 'Enter a valid 5-field cron expression to preview runs.'
                  : 'No upcoming runs.'}
              </span>
            )}
          </div>
        ) : null}
      </div>
    </div>
  )
}

export type { ScheduleDraft, SchedulePreset }
