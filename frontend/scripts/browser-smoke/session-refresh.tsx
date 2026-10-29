import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueuedPromptList } from '@/components/session/QueuedPromptList'
import type { Session, SessionMessages } from '@/lib/api/types'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { useSessionHistory } from '@/lib/hooks/useSessionHistory'
import { useSessionQueue } from '@/lib/hooks/useSessionQueue'
import { keys } from '@/lib/query/keys'

export async function exerciseSessionRefresh(): Promise<void> {
  const originalFetch = window.fetch
  const originalEventSource = window.EventSource
  const id = 'session-refresh'
  let session: Session = {
    id, slug: id, title: 'Working', runtime: 'acp', status: 'running',
    created_at: '2026-10-29T12:00:00Z', updated_at: '2026-10-29T12:00:00Z', last_attention_at: '2026-10-29T12:00:00Z',
  }
  let messages: SessionMessages['messages'] = []
  const page = (): SessionMessages => ({ session: structuredClone(session), messages, events: [], history_revision: 1, latest_event_seq: 1, has_earlier: false })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const sources: EventTarget[] = []
  let hold = false
  let release: (() => void) | undefined
  let queue: ReturnType<typeof useSessionQueue>
  window.EventSource = class extends EventTarget {
    constructor() {
      super()
      sources.push(this)
    }
    close() {}
  } as unknown as typeof EventSource
  const emit = (type: string) => sources.at(-1)!.dispatchEvent(new MessageEvent('message', {
    data: JSON.stringify({ session_id: id, type, at: new Date().toISOString() }),
  }))
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname === '/v1/auth/check') return Response.json({})
    if (!url.pathname.startsWith(`/v1/sessions/${id}`)) return originalFetch(input, init)
    if (url.pathname.endsWith('/queue')) {
      const mutation = JSON.parse(String(init?.body))
      session = {
        ...session,
        updated_at: new Date().toISOString(),
        queued_messages: mutation.op === 'delete' ? [] : [{ ...mutation.message, id: 'acknowledged' }],
      }
      queueMicrotask(() => emit('assistant'))
      return Response.json(structuredClone(session))
    }
    const value = url.pathname.endsWith('/messages') ? page() : structuredClone(session)
    if (!hold) return Response.json(value)
    hold = false
    return new Promise<Response>((resolve, reject) => {
      release = () => resolve(Response.json(value))
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    })
  }
  function Thread() {
    const history = useSessionHistory(id, (message) => {
      throw new Error(message)
    })
    useSessionEvents(id, history.data?.latest_event_seq)
    queue = useSessionQueue({ sessionId: id, session: history.data?.session, streaming: false, onSend: async () => {} })
    return <>
      <p data-session>{history.data?.session.title} {history.data?.session.status}</p>
      <p data-messages>{history.data?.messages.map((message) => message.content).join('\n')}</p>
      <QueuedPromptList prompts={queue.queuedPrompts} onSteer={queue.onSteerQueuedPrompt} onDelete={queue.onDeleteQueuedPrompt} onEdit={queue.onEditQueuedPrompt} onReorder={queue.onReorderQueuedPrompts} />
    </>
  }
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean) => {
    const deadline = performance.now() + 3000
    while (!check()) {
      if (performance.now() > deadline) throw new Error('Session refresh: ' + check.toString())
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const visible = (text: string) => [...element.querySelectorAll('p')].some((node) => node.textContent === text)
  try {
    root.render(<StrictMode><QueryClientProvider client={client}><Thread /></QueryClientProvider></StrictMode>)
    await until(() => sources.length > 0 && visible('Working running'))
    hold = true
    emit('session')
    await until(() => Boolean(release))
    await queue!.onQueuePrompt('Keep this acknowledged instruction')
    await until(() => visible('Keep this acknowledged instruction') && !client.isFetching())
    release!()
    await new Promise((resolve) => setTimeout(resolve, 50))
    if (!visible('Keep this acknowledged instruction') || client.getQueryData<SessionMessages>(keys.sessionMessages(id))?.session.queued_messages?.length !== 1) {
      throw new Error('A stale session read erased the acknowledged queued message')
    }
    queue!.onDeleteQueuedPrompt('acknowledged')
    await until(() => !visible('Keep this acknowledged instruction'))
    await queue!.onQueuePrompt('Run after this turn')
    await until(() => visible('Run after this turn'))
    session = { ...session, title: 'Finished', status: 'idle', queued_messages: [] }
    messages = [{ seq: 1, role: 'user', from_user: true, content: 'Run after this turn', blocks: [], created_at: new Date().toISOString() }]
    emit('session')
    await until(() => visible('Finished idle') && !element.querySelector('button[aria-label="Delete queued prompt"]'))
    if (element.querySelector('[data-messages]')?.textContent !== 'Run after this turn') {
      throw new Error('A session refresh did not retain the consumed prompt in history')
    }
  } finally {
    release?.()
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    window.EventSource = originalEventSource
  }
}
