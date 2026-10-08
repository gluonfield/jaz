import { mock } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'

mock.module('@/lib/theme', () => ({ useTheme: () => ({ resolved: 'light' }) }))
mock.module('@/components/ui/Modal', () => ({ Modal: () => null }))
mock.module('@/lib/appearance', () => ({
  useEffectsEnabled: () => false,
  useInlineDiffs: () => false,
  useInlineShellCommands: () => false,
}))
mock.module('@/lib/clientRuntime', () => ({
  DEFAULT_API_BASE_URL: 'http://127.0.0.1:5299',
  clientRuntime: { platform: 'browser', defaultApiBaseUrl: () => 'http://127.0.0.1:5299' },
}))
globalThis.localStorage = { getItem: () => null }

const { ChatLog } = await import('./ChatLog')
const history = { scrollRef: { current: null }, paging: { hasEarlier: false, loading: false, loadEarlier: async () => false } }
const { botChat } = await import('@/lib/bots')
const client = new QueryClient()
const self = { id: 'bot-1', name: 'Bot' }
const attachments = [
  { type: 'attachment', id: 'text-1', name: 'pasted-text-1.txt', mime_type: 'text/plain' },
  { type: 'attachment', id: 'image-1', name: 'screenshot.png', mime_type: 'image/png' },
]
const html = ['', 'Read these'].map((text) => {
  const messages = [{
    seq: 1,
    role: 'user',
    content: text,
    created_at: '2026-10-06T09:00:00Z',
    blocks: [{ type: 'text', text }, ...attachments],
  }]
  const { entries } = botChat(messages, [], self, [])
  return renderToStaticMarkup(createElement(QueryClientProvider, { client },
    createElement(ChatLog, { entries, bots: [], named: false, working: [], ...history }),
  ))
})
const bots = [
  { id: 'b', name: 'Business Opportunist', avatar: { shape: 'circle', color: 'purple' } },
  { id: 'c', name: 'Planner', avatar: { shape: 'blob', color: 'blue' } },
  { id: 'd', name: 'Shared name' },
  { id: 'e', name: 'Shared name' },
  { id: 'outside', name: 'Business Opportunist' },
  { id: 'f', name: 'R&D', avatar: { shape: 'circle', color: 'green' } },
  { id: 'g', name: 'Research_Lab', avatar: { shape: 'circle', color: 'blue' } },
  { id: 'h', name: 'R&amp;D', avatar: { shape: 'circle', color: 'orange' } },
]
const { botsQuery } = await import('@/lib/api/bots')
client.setQueryData(botsQuery.queryKey, bots)
const messages = [
  '[@Business Opportunist] Please check this.',
  '[@Business Opportunist](bot:c) Explicit ID wins.',
  '[@Shared name] [@Missing] remain text.',
  '`[@Business Opportunist]` and `[@Business Opportunist](bot:b)`',
  '```text\n[@Business Opportunist](bot:b)\n```',
  '[@Business Opportunist](https://example.com)',
  '[@R&amp;D] and [@Research\\_Lab]',
  '[@R&amp;D](bot:f) and [@Research\\_Lab](bot:g) and [@R\\&amp;D]',
]
const events = messages.map((text, index) => ({
  seq: index + 1,
  type: 'room_message',
  at: '2026-10-06T09:00:00Z',
  room_message: { speaker: 'bot', bot_id: 'a', name: 'Researcher', text },
}))
const entries = botChat([], events, self, []).entries
const rootRoute = createRootRoute({
  component: () => createElement(ChatLog, {
    entries, bots, mentionBots: bots.filter((bot) => bot.id !== 'outside'), named: false, working: [], ...history,
  }),
})
const router = createRouter({
  routeTree: rootRoute,
  history: createMemoryHistory({ initialEntries: ['/'] }),
  isServer: true,
})
await router.load()
const mentions = renderToStaticMarkup(createElement(QueryClientProvider, { client },
  createElement(RouterProvider, { router }),
))
client.clear()
globalThis.postMessage({ attachments: html, mentions })
