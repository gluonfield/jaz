import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import type { SessionEvent, SessionMessages } from '@/lib/api/types'
import { useSessionHistory } from '@/lib/hooks/useSessionHistory'

export async function exerciseHistoryHydration() {
  const originalFetch = window.fetch
  const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
  const until = async (check: () => boolean) => {
    const deadline = performance.now() + 4000
    while (!check()) {
      if (performance.now() > deadline) throw new Error('History hydration did not settle')
      await sleep(10)
    }
  }
  const results: object[] = []
  for (const scenario of ['many', 'none', 'error', 'abort']) {
    const id = 'hydration-' + scenario
    const at = '2026-11-02T10:00:00Z'
    const reads: { before: number; turns: number }[] = []
    const errors: string[] = []
    let aborted = false
    let history: ReturnType<typeof useSessionHistory>
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
    const element = document.createElement('div')
    document.body.append(element)
    const root = createRoot(element)
    window.fetch = async (input, init) => {
      const url = new URL(String(input), location.href)
      if (url.pathname !== `/v1/sessions/${id}/messages`) return originalFetch(input, init)
      const before = Number(url.searchParams.get('before_event_seq'))
      reads.push({ before, turns: Number(url.searchParams.get('turns')) })
      if (before && scenario === 'error') return new Response('Fixture unavailable', { status: 503 })
      if (before && scenario === 'abort') {
        return new Promise((_resolve, reject) => init?.signal?.addEventListener('abort', () => {
          aborted = true
          reject(new DOMException('Aborted', 'AbortError'))
        }, { once: true }))
      }
      const end = before || 1026
      const start = Math.max(1, end - 256)
      const events: SessionEvent[] = Array.from({ length: end - start }, (_, index) => {
        const seq = start + index
        return seq === 1 && scenario !== 'none'
          ? { session_id: id, seq, at, type: 'room_message', room_message: { speaker: 'bot', name: 'Bot', text: 'Initial reply' } }
          : { session_id: id, seq, at, type: 'acp', acp: { id: 'worker-' + seq, parent_id: id, state: 'idle' } }
      })
      const page: SessionMessages = {
        session: { id, slug: id, source_type: 'bot', runtime: 'acp', status: 'idle', created_at: at, updated_at: at, last_attention_at: at },
        messages: [], events, has_earlier: start > 1, before_event_seq: start > 1 ? start : undefined,
        latest_event_seq: 1025, history_revision: 1, history_target_event_seq: before || scenario === 'none' ? undefined : 1,
      }
      return Response.json(page)
    }
    function Fixture() {
      history = useSessionHistory(id, (message) => errors.push(message))
      return <button onClick={history.paging.loadEarlier}>Load earlier</button>
    }
    try {
      flushSync(() => root.render(<StrictMode><QueryClientProvider client={client}><Fixture /></QueryClientProvider></StrictMode>))
      await until(() => Boolean(history?.data))
      if (scenario === 'many') {
        await until(() => history.data?.events.some((event) => event.room_message?.text === 'Initial reply') === true)
        const cursors = reads.filter((read) => read.before).map((read) => read.before)
        if (cursors.join() !== '770,514,258,2' || history.data!.events.length !== 1025 || history.data!.history_target_event_seq !== 1) {
          throw new Error('Automatic history hydration stopped early or lost merged history: ' + JSON.stringify(cursors))
        }
      } else if (scenario === 'none') {
        await sleep(80)
        if (reads.some((read) => read.before)) throw new Error('A bot with no conversation walked its metadata history')
        element.querySelector('button')!.click()
        await until(() => reads.some((read) => read.before))
        if (reads.find((read) => read.before)?.turns !== 24) throw new Error('The public noargs history action lost its manual page size')
      } else if (scenario === 'error') {
        await until(() => errors.length > 0)
        await sleep(80)
        if (errors.length !== 1 || reads.filter((read) => read.before).length !== 1) throw new Error('Automatic hydration retried a failed page without new history')
      } else {
        await until(() => reads.some((read) => read.before))
        flushSync(() => root.render(null))
        await until(() => aborted)
        if (errors.length) throw new Error('Cancelled hydration surfaced a load error')
      }
      results.push({ scenario, reads, errors: errors.length, aborted })
    } finally {
      root.unmount()
      client.clear()
      element.remove()
      window.fetch = originalFetch
    }
  }
  console.warn(JSON.stringify({ historyHydration: results }))
}
