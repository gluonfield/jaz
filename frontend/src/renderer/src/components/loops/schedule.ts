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

// A compact label (e.g. "Daily · 9:00 AM") for loop rows and cards.
export function compactSchedule(expr: string, paused: boolean): string {
  if (paused) return 'Manual'
  const draft = parseExpr(expr)
  const time = formatTime(draft.time)
  switch (draft.preset) {
    case 'hourly':
      return 'Hourly'
    case 'daily':
      return `Daily · ${time}`
    case 'weekdays':
      return `Weekdays · ${time}`
    case 'weekly':
      return `${WEEKDAY_LABELS[draft.weekday].slice(0, 3)} · ${time}`
    default:
      return expr.trim()
  }
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
