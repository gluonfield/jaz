import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode, useLayoutEffect, useRef } from 'react'
import { createRoot } from 'react-dom/client'
import { ChatLog } from '@/components/bots/ChatLog'
import { Composer } from '@/components/session/Composer'
import { useLiveSessionSend } from '@/components/session/useLiveSessionSend'
import { liveOptimisticUserMessage } from '@/components/session/liveTranscript'
import { optimisticTranscriptMessages } from '@/lib/optimisticUserMessage'
import { botChat, type ChatEntry } from '@/lib/bots'
import { useSessionHistory } from '@/lib/hooks/useSessionHistory'
import { keys } from '@/lib/query/keys'
import { exerciseSessionRefresh } from './session-refresh'
import { exerciseBotArtifactDelivery } from './bot-artifact-delivery'

export async function exerciseChatHistory(): Promise<void> {
  const bot = { id: 'bot-1', name: 'Sync Bot', avatar: { shape: 'circle', color: 'blue' }, kind: 'bot' } as never
  const entries: ChatEntry[] = Array.from({ length: 150 }, (_, i) => ({
    kind: 'bot', key: `entry-${i}`, at: new Date(Date.UTC(2026, 9, 1) + i * 60_000).toISOString(), botId: 'bot-1', name: 'Sync Bot', text: `Reply ${i}`,
  }))
  let earlierLoads = 0
  const paging = { hasEarlier: true, loading: false, loadEarlier: async () => {
    earlierLoads += 1
    return false
  } }
  function Chat() {
    const scrollRef = useRef<HTMLDivElement>(null)
    useLayoutEffect(() => {
      scrollRef.current!.scrollTop = scrollRef.current!.scrollHeight
    }, [])
    return <div ref={scrollRef} style={{ height: 300, overflowY: 'auto' }}>
      <ChatLog entries={entries} bots={[bot]} named={false} working={[]} scrollRef={scrollRef} paging={paging} />
    </div>
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Chat history: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const shown = () => [...element.querySelectorAll('p')].filter((paragraph) => paragraph.textContent?.startsWith('Reply ')).map((paragraph) => paragraph.textContent)
  const viewport = () => element.firstElementChild as HTMLDivElement
  const topShown = () => [...element.querySelectorAll('p')].find((paragraph) => paragraph.textContent?.startsWith('Reply ') && paragraph.getBoundingClientRect().bottom > viewport().getBoundingClientRect().top)?.textContent
  try {
    root.render(<StrictMode><QueryClientProvider client={new QueryClient()}><Chat /></QueryClientProvider></StrictMode>)
    await until(() => shown().length > 0)
    viewport().scrollTop = viewport().scrollHeight
    await new Promise((resolve) => setTimeout(resolve, 100))
    if (shown().length !== 14 || shown()[0] !== 'Reply 136' || shown().at(-1) !== 'Reply 149') {
      throw new Error(`A bot chat rendered ${shown().length} entries instead of the newest 14`)
    }
    viewport().scrollTop = 0
    await until(() => shown().length === 38)
    if (topShown() !== 'Reply 136') {
      throw new Error(`Revealing earlier entries at the very top moved the reader from Reply 136 to ${topShown()}`)
    }
    while (shown().length < 150) {
      const count = shown().length
      viewport().scrollTop = 0
      await until(() => shown().length > count)
    }
    if (earlierLoads !== 0) {
      throw new Error('Earlier history was fetched while loaded entries were still hidden')
    }
    viewport().scrollTop = 0
    await until(() => earlierLoads === 1)
  } finally {
    root.unmount()
    element.remove()
  }
  await exercisePagedHistory()
  await exerciseBotArtifactDelivery()
  await exerciseSessionRefresh()
}

async function exercisePagedHistory() {
  const originalFetch = window.fetch
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const bot = { id: 'paged-bot', name: 'Bot' }
  const event = (seq: number, at = '2026-10-01T12:00:00.000Z') => ({
    session_id: bot.id, seq, type: 'room_message', at,
    room_message: { speaker: 'bot' as const, name: 'Bot', text: `Message ${seq}` },
  })
  const page = {
    session: { id: bot.id, source_type: 'bot' }, messages: [],
    events: Array.from({ length: 14 }, (_, index) => event(index + 25)),
    has_earlier: true, before_event_seq: 25, history_revision: 1, latest_event_seq: 38,
  }
  let requests = 0
  let release: (() => void) | undefined
  let arrived = false
  let saved = false
  let sendBody: ReadableStreamDefaultController<Uint8Array> | undefined
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname === `/v1/sessions/${bot.id}/messages:stream`) {
      return new Response(new ReadableStream<Uint8Array>({ start: (controller) => {
        sendBody = controller
      } }), {
        headers: { 'Content-Type': 'text/event-stream' },
      })
    }
    if (url.pathname !== `/v1/sessions/${bot.id}/messages`) {
      return originalFetch(input, init)
    }
    requests += 1
    if (url.searchParams.has('before_event_seq')) {
      await new Promise<void>((resolve) => {
        release = resolve
      })
      return Response.json({ ...page, events: Array.from({ length: 24 }, (_, index) => event(index + 1)), has_earlier: false, before_event_seq: 0 })
    }
    return Response.json({ ...page, messages: saved ? [{ seq: 1, role: 'user', from_user: true, content: 'Keep going', blocks: [], created_at: new Date().toISOString() }] : [], events: arrived ? [...page.events, event(39, new Date().toISOString())] : page.events, latest_event_seq: arrived ? 39 : 38 })
  }
  function Chat() {
    const scrollRef = useRef<HTMLDivElement>(null)
    const history = useSessionHistory(bot.id, (message) => {
      throw new Error(message)
    })
    const live = useLiveSessionSend({ sessionId: bot.id, onCriticalError: (message) => {
      throw new Error(message)
    } })
    const messages = optimisticTranscriptMessages(history.data?.messages ?? [], live.live ? liveOptimisticUserMessage(live.live) : null)
    const entries = botChat(messages, history.data?.events ?? [], bot, []).entries
    const opened = useRef(false)
    useLayoutEffect(() => {
      if (opened.current || !entries.length) {
        return
      }
      scrollRef.current!.scrollTop = scrollRef.current!.scrollHeight
      opened.current = true
    }, [entries.length])
    return <div ref={scrollRef} style={{ height: 300, overflowY: 'auto' }}>
      <ChatLog entries={entries} bots={[]} named={false} working={[]} scrollRef={scrollRef} paging={history.paging} />
      <Composer streaming={live.streaming} onSend={live.send} onStop={live.abort} />
    </div>
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean) => {
    const deadline = performance.now() + 4000
    while (!check()) {
      if (performance.now() > deadline) {
        throw new Error('Paged chat: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const paragraph = (text: string) => [...element.querySelectorAll('p')].find((node) => node.textContent === text)
  try {
    root.render(<StrictMode><QueryClientProvider client={client}><Chat /></QueryClientProvider></StrictMode>)
    await until(() => Boolean(paragraph('Message 38')))
    if (requests > 2 || release) {
      throw new Error('Opening a bot eagerly fetched earlier pages')
    }
    if (element.querySelector('[class*="message-arrive"]')) {
      throw new Error('Historical messages animate on open')
    }
    const viewport = element.firstElementChild as HTMLDivElement
    viewport.scrollTop = viewport.scrollHeight
    await new Promise((resolve) => setTimeout(resolve, 80))
    viewport.scrollTop = 0
    await until(() => Boolean(release))
    const anchor = paragraph('Message 25')!
    const top = anchor.getBoundingClientRect().top
    arrived = true
    await client.refetchQueries({ queryKey: keys.sessionMessages(bot.id) })
    await until(() => Boolean(paragraph('Message 39')))
    if (!anchor.isConnected || Math.abs(anchor.getBoundingClientRect().top - top) > 1) {
      throw new Error('A new message moved the reader while an earlier page was loading')
    }
    release!()
    await until(() => Boolean(paragraph('Message 1')))
    if (!anchor.isConnected || Math.abs(anchor.getBoundingClientRect().top - top) > 1 || !paragraph('Message 39')) {
      throw new Error(`Prepending history moved the reader or lost the new arrival: connected=${anchor.isConnected}, before=${top}, after=${anchor.getBoundingClientRect().top}, latest=${Boolean(paragraph('Message 39'))}`)
    }
    if (!paragraph('Message 39')?.closest('[class*="message-arrive"]')) {
      throw new Error('A live message lost its arrival animation')
    }
    await window.smoke.capture('bot-chat-earlier-history')
    const textarea = element.querySelector<HTMLTextAreaElement>('textarea')!
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea, 'Keep going')
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
    await until(() => Boolean(element.querySelector('button[aria-label="Send message"]:not(:disabled)')))
    element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.click()
    await until(() => Boolean(paragraph('Keep going')) && Boolean(sendBody))
    const optimistic = paragraph('Keep going')!
    saved = true
    sendBody!.enqueue(new TextEncoder().encode('data: {"type":"accepted"}\n\ndata: {"type":"done"}\n\n'))
    sendBody!.close()
    await until(() => !element.querySelector('button[aria-label="Stop response"]'))
    if (paragraph('Keep going') !== optimistic || textarea.value) {
      throw new Error('Saving a sent message replaced its bubble or restored its composer text')
    }
  } finally {
    release?.()
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
  }
}
