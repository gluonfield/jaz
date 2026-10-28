import { createElement } from 'react'
import { mock } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { QueryClient, QueryClientProvider, QueryObserver } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, Outlet, RouterProvider } from '@tanstack/react-router'

const storage = {
  getItem: () => null,
  setItem: () => {},
  removeItem: () => {},
}
globalThis.window = {
  location: new globalThis.URL('http://localhost'),
  localStorage: storage,
  matchMedia: () => ({ matches: false, addEventListener: () => {} }),
  addEventListener: () => {},
}
globalThis.localStorage = storage
globalThis.sessionStorage = storage
globalThis.document = {
  addEventListener: () => {},
  compatMode: 'CSS1Compat',
  documentElement: {
    classList: { toggle: () => {}, contains: () => false },
    style: { setProperty: () => {}, removeProperty: () => {} },
  },
}
globalThis.fetch = async () => new globalThis.Response(null, { status: 503 })
const reactDOM = await import('react-dom')
mock.module('react-dom', () => ({ ...reactDOM, createPortal: (children) => children }))
mock.module('@/lib/connection', () => ({
  useBackendChange: () => {},
  useConnection: () => ({ status: 'disconnected', url: 'http://localhost', pairing: null, error: null }),
}))
const browserSessionsModule = await import('@/lib/browserSessions')
mock.module('@/lib/browserSessions', () => ({
  ...browserSessionsModule,
  useSessionPreview: () => ({ target: { displayUrl: '', sourceUrl: '' }, setTarget: () => {} }),
}))
mock.module('@/lib/appearance', () => ({
  useEffectsEnabled: () => false,
  useShowModelIcons: () => false,
  useInlineDiffs: () => true,
  useInlineShellCommands: () => true,
}))

const { Route } = await import('@/routes/sessions.$sessionId')
const { SidePanelStateProvider } = await import('@/components/session/SidePanelState')
const { ToastProvider } = await import('@/components/ui/toast')
const { TitlebarProvider } = await import('@/lib/titlebar')
const { VoiceProvider } = await import('@/lib/voice/VoiceProvider')
const { BrowserSessions, BrowserSessionsContext } = await import('@/lib/browserSessions')
const { keys } = await import('@/lib/query/keys')
const { ApiError } = await import('@/lib/api/response')

const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false, retryDelay: 10, staleTime: Infinity } } })
const sessions = new BrowserSessions()
const rootRoute = createRootRoute({
  component: () => createElement(ToastProvider, null,
    createElement(TitlebarProvider, null,
      createElement(VoiceProvider, null,
        createElement(BrowserSessionsContext.Provider, { value: sessions },
          createElement(SidePanelStateProvider, null, createElement(Outlet)))))),
})
const router = createRouter({
  routeTree: rootRoute.addChildren([Route.update({ path: '/sessions/$sessionId', getParentRoute: () => rootRoute })]),
  history: createMemoryHistory({ initialEntries: ['/sessions/thread'] }),
  isServer: true,
})
await router.load()
const render = () => renderToString(createElement(QueryClientProvider, { client }, createElement(RouterProvider, { router })))
const page = {
  session: { id: 'thread', slug: 'thread', runtime: 'acp', status: 'running', created_at: '2026-09-14T18:00:00Z', updated_at: '2026-09-14T18:00:00Z' },
  messages: [{ seq: 1, role: 'user', content: 'Keep this conversation visible', blocks: [], created_at: '2026-09-14T18:00:00Z' }],
  events: [],
  history_revision: 1,
  latest_event_seq: 0,
  has_earlier: false,
}
const queryKey = keys.sessionMessages('thread')
client.setQueryData(queryKey, page)
const before = render()
const conflict = new ApiError(409, 'session history changed; reload from the latest page')
await client.fetchQuery({ queryKey, staleTime: 0, queryFn: () => Promise.reject(new ApiError(503, 'Service unavailable')) }).catch(() => {})
const after = render()
const query = client.getQueryCache().find({ queryKey })
const requests = []
let conflicts = 3
const freshPage = {
  ...page,
  history_revision: 2,
  has_earlier: true,
  before_message_seq: 3,
  messages: [{ ...page.messages[0], seq: 3, content: 'Latest conversation' }],
}
globalThis.fetch = async (input) => {
  const url = new globalThis.URL(String(input))
  requests.push(url.search)
  if (!url.search) {
    return globalThis.Response.json(freshPage)
  }
  if (conflicts-- > 0) {
    return globalThis.Response.json({ error: conflict.message }, { status: 409 })
  }
  return globalThis.Response.json({
    ...freshPage,
    has_earlier: false,
    before_message_seq: undefined,
    messages: [{ ...page.messages[0], seq: 2, content: 'Earlier current history' }],
  })
}
function nextFailure() {
  return new Promise((resolve) => {
    const unsubscribe = client.getQueryCache().subscribe((event) => {
      if (event.type === 'updated' && event.query === query && ['failed', 'error'].includes(event.action.type)) {
        unsubscribe()
        resolve()
      }
    })
  })
}

const observer = new QueryObserver(client, { ...query.options, refetchOnMount: false })
const unsubscribe = observer.subscribe(() => {})
const firstFailure = nextFailure()
const refresh = client.fetchQuery({ ...query.options, staleTime: 0 }).catch(() => {})
await firstFailure
client.setQueryData(queryKey, (previous) => ({ ...previous, session: { ...previous.session, title: 'Live metadata update' } }))
const during = render()
await refresh
const recovered = render()
const recoveryRequests = [...requests]

conflicts = Infinity
const cancellingFailure = nextFailure()
const cancelled = client.fetchQuery({ ...query.options, staleTime: 0 }).catch(() => {})
await cancellingFailure
unsubscribe()
await cancelled
const requestsAtUnmount = requests.length
await new Promise((resolve) => globalThis.setTimeout(resolve, 30))
const requestsAfterUnmount = requests.length - requestsAtUnmount

client.removeQueries({ queryKey })
let failureRequests = 0
globalThis.fetch = async () => {
  failureRequests++
  return globalThis.Response.json({ error: 'Internal database details' }, { status: 503 })
}
await client.fetchQuery({ ...query.options, staleTime: 0 }).catch(() => {})
const failed = render()

globalThis.postMessage({
  before,
  after,
  failed,
  during,
  recovered,
  requests: recoveryRequests,
  requestsAfterUnmount,
  failureRequests,
})
client.clear()
