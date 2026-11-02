import type { Alert, AlertState } from '@shared/notifications'

const RANK: Record<AlertState, number> = { needs_you: 0, stopped: 1, done: 2, reply: 3 }

// A thread that repeats replaces its own row; rows read needs you, stopped,
// done, then replies, newest first.
export function mergeRows(rows: readonly Alert[], alert: Alert): Alert[] {
  return [alert, ...rows.filter((row) => row.sessionId !== alert.sessionId)].sort(
    (a, b) => RANK[a.state] - RANK[b.state] || b.at - a.at,
  )
}

// Notched MacBooks released before the Dynamic Island MacBook Pro. Anything
// else may be a Dynamic Island Mac, which Jaz never draws over.
const NOTCHED = new Set([
  'MacBookPro18,1', 'MacBookPro18,2', 'MacBookPro18,3', 'MacBookPro18,4',
  'Mac14,2', 'Mac14,5', 'Mac14,6', 'Mac14,9', 'Mac14,10', 'Mac14,15',
  'Mac15,3', 'Mac15,6', 'Mac15,7', 'Mac15,8', 'Mac15,9', 'Mac15,10', 'Mac15,11', 'Mac15,12', 'Mac15,13',
  'Mac16,1', 'Mac16,5', 'Mac16,6', 'Mac16,7', 'Mac16,8', 'Mac16,12', 'Mac16,13',
])

// Where the island may draw on a display: hugging a known notch, as a capsule
// on an external display or a pre-notch MacBook's own, or nowhere.
export function islandShape(model: string, internal: boolean, notched: boolean): 'notch' | 'capsule' | null {
  if (!internal) return 'capsule'
  if (NOTCHED.has(model)) return notched ? 'notch' : null
  return model.startsWith('MacBook') && !notched ? 'capsule' : null
}
