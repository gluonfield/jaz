import { parseAlert, type Alert } from '@shared/notifications'

export type StreamTarget = { baseUrl: string; token: string }

const SILENCE_MS = 60_000
const MAX_RETRY_MS = 30_000

// Follows the backend's live alert stream, reconnecting after drops. The
// server never replays, so a reconnect only resumes what happens next.
export function createAlertStream(onAlert: (alert: Alert) => void, fetcher: typeof fetch = fetch) {
  let target: StreamTarget | null = null
  let request: AbortController | null = null
  let retry: NodeJS.Timeout | null = null
  let backoff = 1000

  async function listen(current: StreamTarget): Promise<void> {
    const controller = new AbortController()
    request = controller
    let silence = setTimeout(() => controller.abort(), SILENCE_MS)
    try {
      const response = await fetcher(`${current.baseUrl}/v1/notifications/stream`, {
        headers: { Accept: 'text/event-stream', ...(current.token ? { Authorization: `Bearer ${current.token}` } : {}) },
        signal: controller.signal,
      })
      if (!response.ok || !response.body) return
      backoff = 1000
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      for (;;) {
        const { done, value } = await reader.read()
        if (done) return
        clearTimeout(silence)
        silence = setTimeout(() => controller.abort(), SILENCE_MS)
        buffer += decoder.decode(value, { stream: true })
        const blocks = buffer.split('\n\n')
        buffer = blocks.pop() ?? ''
        for (const block of blocks) {
          const data = block.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5)).join('\n')
          const alert = data ? parseData(data) : null
          if (alert && target === current) onAlert(alert)
        }
      }
    } catch {
      return
    } finally {
      clearTimeout(silence)
      if (request === controller) request = null
      if (target === current) {
        retry = setTimeout(() => void listen(current), backoff)
        retry.unref()
        backoff = Math.min(backoff * 2, MAX_RETRY_MS)
      }
    }
  }

  function stop(): void {
    target = null
    request?.abort()
    request = null
    if (retry) clearTimeout(retry)
    retry = null
  }

  return {
    follow(next: StreamTarget | null): void {
      if (next && target && next.baseUrl === target.baseUrl && next.token === target.token) return
      stop()
      if (!next) return
      target = next
      backoff = 1000
      void listen(next)
    },
    stop,
  }
}

function parseData(data: string): Alert | null {
  try {
    return parseAlert(JSON.parse(data))
  } catch {
    return null
  }
}
