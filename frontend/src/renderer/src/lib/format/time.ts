const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

export function relativeTime(iso: string, now = Date.now()): string {
  const then = new Date(iso).getTime()
  // Treat unset/zero-value times (Go marshals these as "0001-01-01T00:00:00Z",
  // a large negative epoch) as no time rather than a far-past date.
  if (!Number.isFinite(then) || then <= 0) return ''
  const diff = Math.max(0, now - then)
  if (diff < MINUTE) return 'now'
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)}m`
  if (diff < DAY) return `${Math.floor(diff / HOUR)}h`
  if (diff < 7 * DAY) return `${Math.floor(diff / DAY)}d`
  return shortDate(iso)
}

// "Jun 16" — the app's standard compact calendar label.
export function shortDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

// Go marshals zero-value times as "0001-01-01T00:00:00Z" (a large negative
// epoch) even with `omitempty`; treat those — and unset values — as no time.
// A type guard so callers can use the narrowed value without casting back to string.
export function hasTime(iso?: string): iso is string {
  if (!iso) return false
  const ms = new Date(iso).getTime()
  return Number.isFinite(ms) && ms > 0
}

export function fullTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function messageTime(iso: string, now = Date.now()): string {
  return new Date(iso).toDateString() === new Date(now).toDateString() ? clockTime(iso) : fullTime(iso)
}

export function clockTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

export function formatDuration(ms: number | undefined): string {
  if (ms === undefined) return ''
  const seconds = Math.max(1, Math.floor(ms / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  return `${Math.floor(minutes / 60)}h`
}
