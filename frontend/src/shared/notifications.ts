export interface ThreadCompletion {
  id: string
  slug: string
  title: string
  completedAt: string
}

export type ThreadCompletionHistory = ReadonlyMap<string, number>

export type ThreadNotificationConfig =
  | { enabled: false }
  | { enabled: true; baseUrl: string; token: string }

export function parseThreadNotificationConfig(value: unknown): ThreadNotificationConfig | null {
  if (!value || typeof value !== 'object') return null
  const input = value as Record<string, unknown>
  if (input.enabled === false) return { enabled: false }
  if (
    input.enabled !== true ||
    typeof input.baseUrl !== 'string' ||
    typeof input.token !== 'string'
  ) {
    return null
  }
  try {
    const url = new URL(input.baseUrl.trim())
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    return { enabled: true, baseUrl: url.origin, token: input.token.trim() }
  } catch {
    return null
  }
}

export function parseThreadCompletions(value: unknown): ThreadCompletion[] | null {
  if (!value || typeof value !== 'object') return null
  const items = (value as Record<string, unknown>).items
  if (!Array.isArray(items)) return null
  const completions: ThreadCompletion[] = []
  for (const value of items) {
    if (!value || typeof value !== 'object') return null
    const item = value as Record<string, unknown>
    const id = typeof item.id === 'string' ? item.id.trim() : ''
    const slug = typeof item.slug === 'string' ? item.slug.trim() : ''
    const title = typeof item.title === 'string' ? item.title.trim() : ''
    const completedAt = typeof item.completed_at === 'string' ? item.completed_at.trim() : ''
    if (!id || !completedAt || Number.isNaN(Date.parse(completedAt))) return null
    completions.push({ id, slug, title, completedAt })
  }
  return completions
}

export function diffThreadCompletions(
  previous: ThreadCompletionHistory | null,
  items: ThreadCompletion[],
): { history: Map<string, number>; added: ThreadCompletion[] } {
  const history = new Map(previous ?? [])
  const added: ThreadCompletion[] = []
  for (const item of items) {
    const completedAt = Date.parse(item.completedAt)
    const knownAt = history.get(item.id)
    if (previous && (knownAt === undefined || completedAt > knownAt)) added.push(item)
    if (knownAt === undefined || completedAt > knownAt) history.set(item.id, completedAt)
  }
  return { history, added }
}

export function threadNotificationPath(threadId: string): string {
  return `/sessions/${encodeURIComponent(threadId)}`
}
