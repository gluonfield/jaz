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
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

// Go marshals zero-value times as "0001-01-01T00:00:00Z" (a large negative
// epoch) even with `omitempty`; treat those — and unset values — as no time.
export function hasTime(iso?: string): boolean {
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
