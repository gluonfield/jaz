import type { Loop, LoopTrigger } from '@/lib/api/types'

// Maps the user-friendly schedule presets to 5-field cron expressions and back.
// "Manual" is not a cron shape — it is represented by a paused loop that only
// runs on demand, so it preserves whatever expression the loop already had.

export type SchedulePreset = 'manual' | 'hourly' | 'daily' | 'weekdays' | 'weekly' | 'custom'

export interface ScheduleDraft {
  preset: SchedulePreset
  time: string // "HH:MM" 24h
  weekday: number // 0=Sun .. 6=Sat
  expr: string // raw cron, used for custom and preserved as a manual baseline
}

export const WEEKDAY_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

export const SCHEDULE_PRESETS: { value: SchedulePreset; label: string }[] = [
  { value: 'manual', label: 'Manual' },
  { value: 'hourly', label: 'Hourly' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekdays', label: 'Weekdays' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'custom', label: 'Custom' },
]

const DEFAULT_TIME = '09:00'

export function localTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

export function defaultScheduleDraft(): ScheduleDraft {
  return { preset: 'daily', time: DEFAULT_TIME, weekday: 1, expr: `0 9 * * *` }
}

function splitTime(time: string): [number, number] {
  const [h, m] = time.split(':').map((part) => Number.parseInt(part, 10))
  return [Number.isFinite(h) ? h : 9, Number.isFinite(m) ? m : 0]
}

function pad(n: number): string {
  return n.toString().padStart(2, '0')
}

export function cronFromDraft(draft: ScheduleDraft): string {
  const [h, m] = splitTime(draft.time)
  switch (draft.preset) {
    case 'manual':
      return draft.expr.trim() || `${m} ${h} * * *`
    case 'hourly':
      return '0 * * * *'
    case 'daily':
      return `${m} ${h} * * *`
    case 'weekdays':
      return `${m} ${h} * * 1-5`
    case 'weekly':
      return `${m} ${h} * * ${draft.weekday}`
    case 'custom':
      return draft.expr.trim()
  }
}

// Rebuilds the editable draft from a stored loop. A paused loop is shown as
// Manual; otherwise the preset is inferred from the cron shape.
export function draftFromLoop(expr: string, paused: boolean): ScheduleDraft {
  const parsed = parseExpr(expr)
  return paused ? { ...parsed, preset: 'manual' } : parsed
}

function parseExpr(expr: string): ScheduleDraft {
  const fields = expr.trim().split(/\s+/)
  const fallback: ScheduleDraft = { preset: 'custom', time: DEFAULT_TIME, weekday: 1, expr }
  if (fields.length !== 5) return fallback
  const [min, hour, dom, mon, dow] = fields
  if (min === '0' && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return { preset: 'hourly', time: DEFAULT_TIME, weekday: 1, expr }
  }
  const m = Number(min)
  const h = Number(hour)
  const numeric = Number.isInteger(m) && Number.isInteger(h) && dom === '*' && mon === '*'
  if (numeric) {
    const time = `${pad(h)}:${pad(m)}`
    if (dow === '*') return { preset: 'daily', time, weekday: 1, expr }
    if (dow === '1-5') return { preset: 'weekdays', time, weekday: 1, expr }
    if (/^[0-6]$/.test(dow)) return { preset: 'weekly', time, weekday: Number(dow), expr }
  }
  return fallback
}

export function isManual(preset: SchedulePreset): boolean {
  return preset === 'manual'
}

// A short human description of when a draft runs, for inline confirmation.
export function describeSchedule(draft: ScheduleDraft): string {
  const time12 = formatTime(draft.time)
  switch (draft.preset) {
    case 'manual':
      return 'Runs only when you trigger it'
    case 'hourly':
      return 'Runs at the top of every hour'
    case 'daily':
      return `Runs every day at ${time12}`
    case 'weekdays':
      return `Runs Monday–Friday at ${time12}`
    case 'weekly':
      return `Runs every ${WEEKDAY_LABELS[draft.weekday]} at ${time12}`
    case 'custom':
      return draft.expr.trim() ? `Cron: ${draft.expr.trim()}` : 'Enter a cron expression'
  }
}

// A short readable label for when a schedule runs, such as "Every 2 hours" or
// "Weekdays · 9:00 AM", or '' when it is too irregular to say briefly; the
// next run time then speaks for it.
export function compactSchedule(expr: string, paused: boolean): string {
  if (paused) return 'Manual'
  const [min, hour, dom, mon, dow, extra] = expr.trim().split(/\s+/)
  if (dow === undefined || extra !== undefined || dom !== '*' || mon !== '*') return ''
  const days = dow === '*' ? '' : dow === '1-5' ? 'Weekdays' : /^[0-6]$/.test(dow) ? `${WEEKDAY_LABELS[Number(dow)]}s` : undefined
  const often = frequency(min, hour)
  if (days === undefined || !often) return ''
  return [days || (often.daily ? 'Daily' : ''), often.text].filter(Boolean).join(' · ')
}

// How often within a day a schedule runs: at fixed times, or repeating.
function frequency(min: string, hour: string): { text: string; daily: boolean } | undefined {
  if (hour === '*') {
    const step = /^\*\/(\d+)$/.exec(min)
    return min === '0' ? { text: 'Hourly', daily: false } : step ? { text: every(step[1], 'minute'), daily: false } : undefined
  }
  if (!/^\d+$/.test(min)) return undefined
  if (/^\d+(,\d+)*$/.test(hour)) {
    return { text: hour.split(',').map((h) => formatTime(`${h}:${min}`)).join(', '), daily: true }
  }
  const repeat = /^(?:\*|(\d+)-(\d+))(?:\/(\d+))?$/.exec(hour)
  if (!repeat) return undefined
  const [, from, to, step] = repeat
  const text = step ? every(step, 'hour') : 'Hourly'
  return { text: from ? `${text}, ${hourLabel(from)}–${hourLabel(to)}` : text, daily: false }
}

function every(count: string, unit: string): string {
  return count === '1' ? `Every ${unit}` : `Every ${count} ${unit}s`
}

function hourLabel(hour: string): string {
  const h = Number(hour)
  return `${h % 12 === 0 ? 12 : h % 12} ${h < 12 ? 'AM' : 'PM'}`
}

interface CronField {
  set: Set<number>
  star: boolean
}

// Parses one cron field (supports *, a, a-b, a-b/n, */n, and comma lists) into
// the set of values it matches. Returns null on anything we can't read.
function parseField(field: string, min: number, max: number): CronField | null {
  const set = new Set<number>()
  let star = false
  for (const part of field.split(',')) {
    const [rangePart, stepPart] = part.split('/')
    const step = stepPart ? Number.parseInt(stepPart, 10) : 1
    if (!Number.isInteger(step) || step < 1) return null
    let lo = min
    let hi = max
    if (rangePart !== '*') {
      const [a, b] = rangePart.split('-')
      lo = Number.parseInt(a, 10)
      hi = b !== undefined ? Number.parseInt(b, 10) : lo
      if (!Number.isInteger(lo) || !Number.isInteger(hi)) return null
    } else {
      star = true
    }
    if (lo < min || hi > max || lo > hi) return null
    for (let v = lo; v <= hi; v += step) set.add(v)
  }
  return set.size ? { set, star } : null
}

// Computes the next `count` run times for a 5-field cron, in local time.
// Loops always store the browser timezone, so local stepping matches the server.
export function nextRuns(expr: string, count: number, from = new Date()): Date[] {
  const fields = expr.trim().split(/\s+/)
  if (fields.length !== 5) return []
  const min = parseField(fields[0], 0, 59)
  const hour = parseField(fields[1], 0, 23)
  const dom = parseField(fields[2], 1, 31)
  const mon = parseField(fields[3], 1, 12)
  // Normalize Sunday: cron allows 0 or 7, JS getDay() uses 0.
  const dow = parseField(fields[4].replace(/7/g, '0'), 0, 6)
  if (!min || !hour || !dom || !mon || !dow) return []

  const out: Date[] = []
  const d = new Date(from.getTime())
  d.setSeconds(0, 0)
  d.setMinutes(d.getMinutes() + 1)
  const limit = 366 * 24 * 60
  for (let i = 0; i < limit && out.length < count; i++) {
    const domMatch = dom.set.has(d.getDate())
    const dowMatch = dow.set.has(d.getDay())
    const dayOk = dom.star || dow.star ? domMatch && dowMatch : domMatch || dowMatch
    if (min.set.has(d.getMinutes()) && hour.set.has(d.getHours()) && mon.set.has(d.getMonth() + 1) && dayOk) {
      out.push(new Date(d.getTime()))
    }
    d.setMinutes(d.getMinutes() + 1)
  }
  return out
}

function formatTime(time: string): string {
  const [h, m] = splitTime(time)
  const period = h < 12 ? 'AM' : 'PM'
  const hour12 = h % 12 === 0 ? 12 : h % 12
  return `${hour12}:${pad(m)} ${period}`
}

const SOURCES: Record<Exclude<LoopTrigger['kind'], 'webhook'>, string> = {
  gmail: 'Gmail',
  whatsapp: 'WhatsApp',
  telegram: 'Telegram',
  slack: 'Slack',
}

// catchesUp marks a routine that makes up for runs missed while Jaz was off.
export const catchesUp = ({ when_missed }: Pick<Loop, 'when_missed'>) =>
  when_missed === 'catch_up' ? ' · catches up if missed' : ''

// When a loop runs, and whether it makes up for missed runs.
export function loopWhen(loop: Loop, paused = false): string {
  const when = loopTiming(loop, paused)
  return loop.trigger?.kind === 'webhook' ? when : when + catchesUp(loop)
}

// When a loop runs: on what triggers it, or on its schedule.
export function loopTiming({ trigger, schedule }: Pick<Loop, 'trigger' | 'schedule'>, paused = false): string {
  if (!trigger) return compactSchedule(schedule.expr, paused)
  if (trigger.kind === 'webhook') return 'Webhook'
  const where = trigger.kind === 'slack' && trigger.subject ? ` in ${trigger.subject}` : ''
  return `New ${SOURCES[trigger.kind]}${trigger.from ? ` from ${trigger.from}` : ''}${where}`
}

export const CADENCES = ['Through the day', 'Daily', 'Weekdays', 'Weekly', 'When something arrives', 'Other'] as const
export type Cadence = (typeof CADENCES)[number]

// How often a loop runs, as its list groups it.
export function loopCadence({ trigger, schedule }: Pick<Loop, 'trigger' | 'schedule'>): Cadence {
  if (trigger) return 'When something arrives'
  const [, hour, dom, mon, dow, extra] = schedule.expr.trim().split(/\s+/)
  if (dow === undefined || extra !== undefined || dom !== '*' || mon !== '*') return 'Other'
  if (!/^\d+(,\d+)*$/.test(hour)) return 'Through the day'
  if (dow === '*') return 'Daily'
  if (dow === '1-5') return 'Weekdays'
  return /^[0-6]$/.test(dow) ? 'Weekly' : 'Other'
}
