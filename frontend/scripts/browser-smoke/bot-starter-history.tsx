import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BotChat } from '@/components/bots/BotChat'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanelStateProvider } from '@/components/session/SidePanelState'
import { ThreadView } from '@/components/session/ThreadView'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, SessionEvent, SessionMessages } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { TitlebarProvider } from '@/lib/titlebar'
import { VoiceProvider } from '@/lib/voice/VoiceProvider'

export async function exerciseBotStarterHistory(): Promise<void> {
  const now = '2026-10-26T12:00:00Z'
  const bot: Bot = { id: 'starter-history', kind: 'bot', name: 'History Bot', avatar: { shape: 'circle', color: 'blue' }, unread: false, status: 'idle', created_at: now, updated_at: now }
  const fresh: SessionMessages = {
    session: { id: bot.id, slug: bot.id, source_type: 'bot', runtime: 'acp', status: 'idle', created_at: now, updated_at: now, last_attention_at: now },
    messages: [], events: [], has_earlier: false, history_revision: 1, latest_event_seq: 0,
  }
  const delivery: SessionEvent = { session_id: bot.id, seq: 25, at: now, type: 'room_message', room_message: { speaker: 'bot', name: bot.name, text: 'Your requested work is ready.' } }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  client.setQueryData(keys.computerSettings, { enabled: false })
  client.setQueryData(keys.sessionOverview(bot.id), { threads: [], subagents: [] })
  client.setQueryData(keys.sessionRepo(bot.id), { git: false })
  const loading = Promise.withResolvers<void>()
  const earlier = Promise.withResolvers<void>()
  let initialRequested = false
  let earlierRequested = false
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname === `/v1/sessions/${bot.id}/messages`) {
      if (url.searchParams.has('before_event_seq')) {
        earlierRequested = true
        await earlier.promise
        return Response.json({ ...fresh, events: [delivery], latest_event_seq: 25 })
      }
      initialRequested = true
      await loading.promise
      return Response.json({ ...fresh, events: [delivery], latest_event_seq: 25 })
    }
    if (url.pathname === `/v1/sessions/${bot.id}/events`) {
      return new Response('', { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.pathname.endsWith('/seen')) {
      return Response.json({ ok: true })
    }
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const route = createRootRoute({ component: () => (
    <TitlebarProvider><BrowserWorkspace><SidePanelStateProvider>
      <ThreadView sessionId={bot.id} openDetails={false} chat={(view) => <BotChat {...view} bot={bot} bots={[bot]} newBot={false} />} />
    </SidePanelStateProvider></BrowserWorkspace></TitlebarProvider>
  ) })
  const router = createRouter({ routeTree: route, history: createMemoryHistory({ initialEntries: ['/'] }) })
  const mount = () => flushSync(() => root.render(
    <QueryClientProvider client={client}><VoiceProvider><RouterProvider router={router} /></VoiceProvider></QueryClientProvider>,
  ))
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 4000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Bot starter history: ' + check.toString())
      }
      await new Promise(requestAnimationFrame)
    }
  }
  const starters = () => element.textContent?.includes('What should I take on first?') === true
  const noStarters = () => {
    if (starters()) {
      throw new Error('Established or unresolved history shows new-bot starters')
    }
  }
  const update = async (page: SessionMessages) => {
    client.setQueryData(keys.sessionMessages(bot.id), page)
    await new Promise(requestAnimationFrame)
    await new Promise(requestAnimationFrame)
  }
  try {
    mount()
    await until(() => initialRequested)
    noStarters()
    loading.resolve()
    await until(() => element.textContent?.includes(delivery.room_message!.text) === true)
    noStarters()
    await window.smoke.capture('bot-starters-established-desktop')
    flushSync(() => root.render(null))
    mount()
    await until(() => element.textContent?.includes(delivery.room_message!.text) === true)
    noStarters()

    await update({ ...fresh, has_earlier: true, before_event_seq: 25, latest_event_seq: 25 })
    noStarters()
    const viewport = element.querySelector<HTMLElement>('.overflow-y-auto')!
    viewport.scrollTop = 0
    viewport.dispatchEvent(new Event('scroll'))
    await until(() => earlierRequested)
    noStarters()
    earlier.resolve()
    await until(() => element.textContent?.includes(delivery.room_message!.text) === true)
    noStarters()

    await update({ ...fresh, latest_event_seq: 25 })
    noStarters()
    await update({ ...fresh, messages: [{ seq: 1, role: 'assistant', content: 'Private reasoning', blocks: [], created_at: now }] })
    noStarters()
    const question: SessionEvent = { session_id: bot.id, seq: 26, at: now, type: 'permission_request', permission: { id: 'scope', session_id: bot.id, status: 'pending', title: 'Which account?', questions: [{ id: 'account', question: 'Which account should I use?' }] } }
    await update({ ...fresh, session: { ...fresh.session, status: 'running' }, events: [question], latest_event_seq: 26, acp_permissions: [question.permission!] })
    await until(() => element.textContent?.includes('Which account should I use?') === true)
    noStarters()
    const answer = element.querySelector<HTMLInputElement>('input[aria-label="Which account should I use?"]')
    const submit = [...element.querySelectorAll('button')].find((button) => button.textContent === 'Submit')
    if (!answer || answer.matches(':disabled') || !submit || submit.disabled) {
      throw new Error('The real unanswered question lost its answer controls')
    }
    await window.smoke.resize(390, 844)
    await new Promise(requestAnimationFrame)
    await window.smoke.capture('bot-starters-real-question-mobile')

    await update(fresh)
    await until(starters)
    await window.smoke.capture('bot-starters-fresh-mobile')
    flushSync(() => root.render(null))
    mount()
    await until(starters)
    await window.smoke.resize(1200, 850)
  } finally {
    loading.resolve()
    earlier.resolve()
    root.unmount()
    element.remove()
    client.clear()
    window.fetch = originalFetch
  }
}
