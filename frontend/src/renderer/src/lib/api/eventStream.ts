import { apiEventSourceUrl, apiFetch, apiBaseUrl, apiSessionRevision, onApiSessionChange, setResourceTicket } from '@/lib/api/client'

export function openEventStream(
  path: string,
  handlers: Record<string, (event: MessageEvent) => void>,
  onConnection?: (connected: boolean) => void,
): () => void {
  let closed = false
  let source: EventSource | undefined
  let lastEventID = ''
  let retry: ReturnType<typeof setTimeout> | undefined
  const controller = new AbortController()
  const base = apiBaseUrl()
  const revision = apiSessionRevision()

  async function connect(): Promise<void> {
    try {
      const response = await apiFetch('/v1/auth/check', { signal: controller.signal })
      if (!response.ok) {
        throw new Error('Connection authentication failed')
      }
      const auth = await response.json() as { resource_ticket?: { token: string; expires_at: string } }
      if (closed || apiBaseUrl() !== base || apiSessionRevision() !== revision) {
        return
      }
      setResourceTicket(base, auth.resource_ticket)
      const url = new URL(apiEventSourceUrl(path))
      if (lastEventID) {
        url.searchParams.set('after_seq', lastEventID)
      }
      source = new EventSource(url.toString())
      for (const [name, handler] of Object.entries(handlers)) {
        source.addEventListener(name, ((event: MessageEvent) => {
          if (closed || apiSessionRevision() !== revision) return
          if (event.lastEventId) {
            lastEventID = event.lastEventId
          }
          handler(event)
        }) as EventListener)
      }
      source.onopen = () => {
        if (!closed) onConnection?.(true)
      }
      source.onerror = reconnect
    } catch {
      reconnect()
    }
  }

  function reconnect(): void {
    source?.close()
    onConnection?.(false)
    if (!closed) {
      retry = setTimeout(() => {
        void connect()
      }, 2_000)
    }
  }

  const unsubscribe = onApiSessionChange(close)
  function close(): void {
    closed = true
    controller.abort()
    clearTimeout(retry)
    source?.close()
    unsubscribe()
  }
  void connect()
  return close
}
