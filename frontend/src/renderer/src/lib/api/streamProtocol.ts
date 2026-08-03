import type { ToolCallJSON } from './types'

export interface AgentStreamEvent {
  type: 'accepted' | 'delta' | 'reasoning' | 'tool_call' | 'tool_result' | 'error' | 'done' | (string & {})
  delta?: string
  reasoning?: string
  tool_call?: ToolCallJSON
  tool_name?: string
  result?: string
  error?: string
  at?: string
}

export interface SessionMessageStream {
  accepted: Promise<void>
  finished: Promise<void>
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

export function readSessionMessageStream(
  response: Promise<Response>,
  onEvent: (event: AgentStreamEvent) => void,
): SessionMessageStream {
  const acceptance = deferred<void>()
  let acknowledged = false
  void acceptance.promise.catch(() => undefined)

  const finished = (async () => {
    const res = await response
    if (!res.ok || !res.body) {
      let detail = `${res.status} ${res.statusText}`
      try {
        const body = (await res.json()) as { error?: string }
        if (body.error) detail = body.error
      } catch {
        // keep status text
      }
      throw new Error(detail)
    }

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    const handleFrame = (frame: string) => {
      for (const line of frame.split('\n')) {
        if (!line.startsWith('data: ')) continue
        let event: AgentStreamEvent
        try {
          event = JSON.parse(line.slice(6)) as AgentStreamEvent
        } catch {
          continue
        }
        if (event.type === 'accepted' && !acknowledged) {
          acknowledged = true
          acceptance.resolve()
        }
        onEvent(event)
        if (event.type === 'error') {
          throw new Error(event.error || 'Something went wrong.')
        }
      }
    }

    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let sep = buffer.indexOf('\n\n')
      while (sep !== -1) {
        handleFrame(buffer.slice(0, sep))
        buffer = buffer.slice(sep + 2)
        sep = buffer.indexOf('\n\n')
      }
    }
    if (buffer.trim()) handleFrame(buffer)
    if (!acknowledged) throw new Error('Message was not accepted by the server.')
  })().catch((cause: unknown) => {
    const error = cause instanceof Error ? cause : new Error('Something went wrong.')
    if (!acknowledged) acceptance.reject(error)
    throw error
  })

  return { accepted: acceptance.promise, finished }
}
