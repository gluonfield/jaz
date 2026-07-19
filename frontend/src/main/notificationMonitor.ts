import {
  diffThreadCompletions,
  parseThreadCompletions,
  parseThreadNotificationConfig,
  type ThreadCompletion,
  type ThreadNotificationConfig,
} from '../shared/notifications'

const POLL_INTERVAL_MS = 15_000
const REQUEST_TIMEOUT_MS = 10_000

type EnabledConfig = Extract<ThreadNotificationConfig, { enabled: true }>

export function createThreadCompletionMonitor(
  onCompletion: (completion: ThreadCompletion) => void,
  fetcher: typeof fetch = fetch,
) {
  let config: EnabledConfig | null = null
  let history: Map<string, number> | null = null
  let polling = false
  let timer: NodeJS.Timeout | null = null

  async function poll(): Promise<void> {
    if (polling || !config) return
    const requestConfig = config
    polling = true
    try {
      const response = await fetcher(`${requestConfig.baseUrl}/v1/feed/completions`, {
        headers: requestConfig.token
          ? { Authorization: `Bearer ${requestConfig.token}` }
          : undefined,
        signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
      })
      if (!response.ok) return
      const items = parseThreadCompletions(await response.json())
      if (!items || config !== requestConfig) return
      const next = diffThreadCompletions(history, items)
      history = next.history
      for (const item of next.added) onCompletion(item)
    } catch {
      return
    } finally {
      polling = false
      if (config && config !== requestConfig) void poll()
    }
  }

  function stop(): void {
    config = null
    history = null
    if (timer) clearInterval(timer)
    timer = null
  }

  return {
    async configure(value: unknown): Promise<boolean> {
      const next = parseThreadNotificationConfig(value)
      if (!next) {
        stop()
        return false
      }
      if (!next.enabled) {
        stop()
        return true
      }
      if (config && config.baseUrl === next.baseUrl && config.token === next.token) return true
      const backendChanged = !config || config.baseUrl !== next.baseUrl
      config = next
      if (backendChanged) history = null
      if (!timer) {
        timer = setInterval(() => void poll(), POLL_INTERVAL_MS)
        timer.unref()
      }
      await poll()
      return true
    },
    poll,
    stop,
  }
}
