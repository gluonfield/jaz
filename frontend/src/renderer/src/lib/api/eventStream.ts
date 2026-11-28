import type { ResourceTicket } from '@shared/connectionSession'
import { apiBaseUrl, apiEventSourceUrl, get, isCloudApiSession, onApiSessionChange, setResourceTicket } from '@/lib/api/client'
import { fetchAuthMe } from '@/lib/api/cloudAuth'

export function openEventStream(
  path: string,
  handlers: Record<string, (event: MessageEvent) => void>,
  onConnection?: (connected: boolean) => void,
): () => void {
  let source: EventSource | undefined
  let lastEventID = ''
  let retry: ReturnType<typeof setTimeout> | undefined
  const controller = new AbortController()
  const { signal } = controller

  async function connect(): Promise<void> {
    try {
      const ticket = isCloudApiSession()
        ? (await fetchAuthMe(undefined, { signal })).ticket
        : (await get<{ resource_ticket?: ResourceTicket }>('/v1/auth/check', { signal })).resource_ticket
      if (signal.aborted) return
      setResourceTicket(apiBaseUrl(), ticket)
      const url = new URL(apiEventSourceUrl(path))
      if (lastEventID) {
        url.searchParams.set('after_seq', lastEventID)
      }
      source = new EventSource(url.toString())
      for (const [name, handler] of Object.entries(handlers)) {
        source.addEventListener(name, ((event: MessageEvent) => {
          if (signal.aborted) return
          if (event.lastEventId) {
            lastEventID = event.lastEventId
          }
          handler(event)
        }) as EventListener)
      }
      source.onopen = () => {
        if (!signal.aborted) onConnection?.(true)
      }
      source.onerror = reconnect
    } catch {
      reconnect()
    }
  }

  function reconnect(): void {
    if (signal.aborted) return
    source?.close()
    retry = setTimeout(() => void connect(), 2_000)
    onConnection?.(false)
  }

  const unsubscribe = onApiSessionChange(close)
  function close(): void {
    controller.abort()
    clearTimeout(retry)
    source?.close()
    unsubscribe()
  }
  void connect()
  return close
}
