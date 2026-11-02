import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { flushSync } from 'react-dom'
import { BotChat } from '@/components/bots/BotChat'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanelStateProvider } from '@/components/session/SidePanelState'
import { ThreadView } from '@/components/session/ThreadView'
import { ToastProvider } from '@/components/ui/toast'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, SessionEvent, SessionMessages } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { TitlebarProvider } from '@/lib/titlebar'
import { VoiceProvider } from '@/lib/voice/VoiceProvider'

export async function exerciseBotChat() {
  const backend = await window.smoke.backend()
  const { id } = await fetch(backend + '/exercise-bot-chat').then((response) => response.json()) as { id: string }
  const previousBase = apiBaseUrl()
  setApiBaseUrl(backend)
  const bot: Bot = { id, kind: 'bot', name: 'Paint Bot', avatar: { shape: 'circle', color: 'blue' }, unread: false, status: 'idle', created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z' }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  client.setQueryData(keys.computerSettings, { enabled: false })
  client.setQueryData(keys.sessionOverview(id), { threads: [], subagents: [] })
  client.setQueryData(keys.sessionRepo(id), { git: false })
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const route = createRootRoute({ component: () => <TitlebarProvider><BrowserWorkspace><SidePanelStateProvider>
    <ThreadView sessionId={id} openDetails={false} chat={(view) => <BotChat {...view} bot={bot} bots={[bot]} newBot={false} />} />
  </SidePanelStateProvider></BrowserWorkspace></TitlebarProvider> })
  const router = createRouter({ routeTree: route, history: createMemoryHistory({ initialEntries: ['/'] }) })
  let strictSetups = 0
  let strictCleanups = 0
  function StrictProbe() {
    useEffect(() => {
      strictSetups++
      return () => { strictCleanups++ }
    }, [])
    return null
  }
  const mount = () => flushSync(() => root.render(<StrictMode><StrictProbe /><QueryClientProvider client={client}><ToastProvider><VoiceProvider><RouterProvider router={router} /></VoiceProvider></ToastProvider></QueryClientProvider></StrictMode>))
  const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
  const until = async (label: string, check: () => boolean) => {
    const deadline = performance.now() + 5000
    while (!check()) {
      if (performance.now() > deadline) throw new Error('Bot chat: ' + label)
      await sleep(20)
    }
  }
  const paragraph = (text: string) => [...element.querySelectorAll('p')].find((node) => node.textContent === text)
  const viewport = () => element.querySelector<HTMLDivElement>('.scrollbar-quiet.overflow-y-auto')!
  const page = () => client.getQueryData<SessionMessages>(keys.sessionMessages(id))
  const bubbles = () => [...element.querySelectorAll<HTMLElement>('[class~="group/message"]')]
  const painted = async (text: string, name: string) => {
    const node = paragraph(text)
    const bounds = node?.getBoundingClientRect()
    const view = viewport().getBoundingClientRect()
    if (!node || !bounds || bounds.top < view.top || bounds.bottom > view.bottom - 100 || bounds.width < 20) {
      throw new Error(`Bot chat: ${name} is not visibly inside the viewport: ${JSON.stringify(bounds)}`)
    }
    const bubble = node.closest('[class~="group/message"]')!
    if (bubble.classList.contains('flex-row-reverse') || getComputedStyle(node).visibility !== 'visible') {
      throw new Error('Bot chat: response lost its author or visibility')
    }
    const paint = await window.smoke.paint(name, { x: bounds.x, y: bounds.y, width: bounds.width, height: bounds.height })
    if (paint.contrast < 60 || paint.inkPixels < 20) {
      throw new Error(`Bot chat: ${name} has DOM text but no painted text: ${JSON.stringify(paint)}`)
    }
    return { text, bounds: bounds.toJSON(), paint, scrollTop: viewport().scrollTop }
  }
  const emit = async (text: string) => {
    const since = Date.now()
    const response = await fetch(backend + '/exercise-bot-chat/' + encodeURIComponent(text), { method: 'POST' })
    if (!response.ok) throw new Error(await response.text())
    const event = await response.json() as SessionEvent
    await until('SSE did not reach the production cache', () => client.getQueryData<SessionEvent[]>(keys.sessionEvents(id))?.some((candidate) => candidate.seq === event.seq) === true)
    await until('SSE did not reach the DOM', () => Boolean(paragraph(text)))
    await sleep(450)
    return { seq: event.seq, since }
  }
  try {
    const openedAt = Date.now()
    mount()
    await until('initial HTTP page did not render', () => Boolean(page()) && bubbles().length > 0)
    await sleep(450)
    if (!paragraph('Bot response 19')) {
      await window.smoke.paint('bot-chat-before-scroll')
      const initial = { humans: bubbles().filter((node) => node.classList.contains('flex-row-reverse')).length, botEvents: page()!.events.filter((event) => event.room_message).length }
      await fetch(backend + '/exercise-bot-chat-release', { method: 'POST' })
      for (let attempt = 0; attempt < 12 && !paragraph('Bot response 19'); attempt++) {
        viewport().scrollTop = viewport().scrollHeight
        await sleep(30)
        viewport().scrollTop = 0
        await sleep(160)
      }
      await until('scrolling did not fetch the historical replies', () => Boolean(paragraph('Bot response 19')))
      viewport().scrollTop = viewport().scrollHeight
      await sleep(100)
      await window.smoke.paint('bot-chat-after-scroll')
      throw new Error(`Bot chat: historical replies missing before scrolling: ${JSON.stringify(initial)}; scrolling fetched them`)
    }
    if (strictSetups !== 2 || strictCleanups !== 1) throw new Error('Bot chat fixture is not running development StrictMode')
    const initial = await painted('Bot response 19', 'bot-chat-initial')
    if (initial.paint.paintedAt < openedAt) throw new Error('Initial response used a stale painted frame')
    if (element.textContent?.includes('Private worker coordination')) throw new Error('Bot chat: internal authorship leaked')
    const history = bubbles()
    const liveSeq = await emit('Live response without scrolling')
    const live = await painted('Live response without scrolling', 'bot-chat-live')
    if (live.paint.frames <= initial.paint.frames || live.paint.paintedAt < liveSeq.since) throw new Error('Live response used a stale painted frame')
    if (history.some((node) => !node.isConnected)) throw new Error('Bot chat: live delivery replaced history nodes')
    viewport().scrollTop -= 240
    await sleep(80)
    const readerTop = viewport().scrollTop
    const anchor = [...element.querySelectorAll('p')].find((node) => node.getBoundingClientRect().top >= 20 && node.textContent?.startsWith('Bot response'))!
    const anchorBottom = anchor.getBoundingClientRect().bottom
    await emit('New response while reading history')
    if (Math.abs(viewport().scrollTop - readerTop) > 1 || Math.abs(anchor.getBoundingClientRect().bottom - anchorBottom) > 1) {
      throw new Error('Bot chat: live delivery moved the reader')
    }
    const readerScrollDelta = viewport().scrollTop - readerTop
    const readerAnchorDelta = anchor.getBoundingClientRect().bottom - anchorBottom
    viewport().scrollTop = 0
    await until('earlier history never started', () => Boolean(element.querySelector('[aria-busy="true"]')))
    const first = [...element.querySelectorAll('p')].find((node) => node.textContent?.startsWith('Bot response'))!
    const bottom = first.getBoundingClientRect().bottom
    await fetch(backend + '/exercise-bot-chat-release', { method: 'POST' })
    await until('earlier history did not load', () => page()!.messages.length === 20)
    await sleep(100)
    if (!first.isConnected || Math.abs(first.getBoundingClientRect().bottom - bottom) > 1) {
      throw new Error('Bot chat: loading earlier history moved its anchor')
    }
    if (Array.from({ length: 20 }, (_, index) => `Bot response ${String(index).padStart(2, '0')}`).some((text) => !paragraph(text)) || page()!.messages.some((message) => !message.from_user)) {
      throw new Error('Bot chat: paging lost replies or human authorship')
    }
    for (const node of bubbles()) {
      const text = node.querySelector('p')?.textContent ?? ''
      if (text.startsWith('Human request') !== node.classList.contains('flex-row-reverse') || bubbles().filter((other) => other.querySelector('p')?.textContent === text).length !== 1) {
        throw new Error('Bot chat: history was duplicated or reclassified: ' + text)
      }
    }
    const earlierAnchorDelta = first.getBoundingClientRect().bottom - bottom
    flushSync(() => root.render(null))
    const reopenedAt = Date.now()
    mount()
    await until('warm StrictMode remount lost the reply', () => Boolean(paragraph('New response while reading history')))
    await sleep(450)
    const warm = await painted('New response while reading history', 'bot-chat-warm')
    if (warm.paint.frames <= live.paint.frames || warm.paint.paintedAt < reopenedAt) throw new Error('Warm remount used a stale painted frame')
    console.warn(JSON.stringify({ botChatPaint: { strictSetups, strictCleanups, initial, live: { ...live, deliveredSeq: liveSeq.seq }, warm, readerScrollDelta, readerAnchorDelta, earlierAnchorDelta, historyMessages: page()!.messages.length, historyReplies: page()!.events.filter((event) => event.room_message).length, liveReplies: client.getQueryData<SessionEvent[]>(keys.sessionEvents(id))?.filter((event) => event.room_message).length } }))
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    setApiBaseUrl(previousBase)
  }
}
