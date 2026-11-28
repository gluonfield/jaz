import { QueryClient } from '@tanstack/react-query'

const storage = {
  getItem: (key) => key === 'jaz.telemetry.enabled' ? 'false' : null,
  removeItem: () => {},
  setItem: () => {},
}
globalThis.window = { jaz: undefined, location: { origin: 'http://localhost' }, localStorage: storage }
globalThis.localStorage = storage

const { deliverMessage, newOutboxSend } = await import('./messageDelivery')
const { outbox, outboxTranscript } = await import('./outbox')
const { sessionMessagesQuery } = await import('./api/sessions')

// A thread's server: it saves a message when it accepts it, and serves the
// history it has saved by then.
const saved = [{ seq: 1, role: 'assistant', content: 'ready', blocks: [], created_at: '2026-11-28T06:00:00Z' }]
const turns = []
let historyDown = false
let refuse = ''
let queueTurns = false
let accepting = Promise.resolve()
const page = () => ({
  session: { id: 'thread', runtime: 'acp', status: 'running' },
  messages: [...saved],
  events: [],
  has_earlier: false,
  history_revision: 1,
  latest_event_seq: 0,
})
const accept = (content) => saved.push({ seq: saved.length + 1, role: 'user', from_user: true, content, blocks: [], created_at: new Date().toISOString() })
globalThis.fetch = async (input, init) => {
  const { pathname } = new globalThis.URL(String(input))
  if (pathname === '/v1/sessions/thread/messages') {
    return historyDown ? globalThis.Response.json({ error: 'unavailable' }, { status: 503 }) : globalThis.Response.json(page())
  }
  const body = JSON.parse(String(init.body))
  const hold = accepting
  await Promise.race([hold, new Promise((_, reject) => init.signal.addEventListener('abort', () => reject(init.signal.reason)))])
  if (refuse) return globalThis.Response.json({ error: refuse }, { status: 500 })
  if (pathname === '/v1/sessions/thread/messages:stream') {
    if (body.message !== '/compact') accept(body.message)
    return new globalThis.Response('event: accepted\ndata: {"type":"accepted"}\n\nevent: done\ndata: {"type":"done"}\n\n')
  }
  if (pathname === '/v1/threads/thread/turns') {
    const replayed = turns.includes(body.client_request_id)
    if (!replayed) {
      turns.push(body.client_request_id)
      // A turn sent while another runs waits its turn, with no row until then.
      if (!queueTurns) accept(body.text)
    }
    return globalThis.Response.json({ turn: { id: 'turn', thread_id: 'thread', status: 'queued' }, replayed, queued: queueTurns })
  }
  throw new Error(`unexpected request ${pathname}`)
}

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
const history = sessionMessagesQuery('thread')
await queryClient.fetchQuery(history)
const loaded = () => queryClient.getQueryData(history.queryKey).messages
const send = (text, cloudRunId) => {
  const controller = new globalThis.AbortController()
  const entry = newOutboxSend(text, {}, loaded().at(-1).seq)
  return { controller, id: entry.id, ...deliver(entry, controller, cloudRunId) }
}
const deliver = (entry, controller = new globalThis.AbortController(), cloudRunId) =>
  deliverMessage({ queryClient, sessionId: 'thread', entry, cloudRunId, signal: controller.signal, onEvent: () => {} })
const view = () => outboxTranscript(loaded(), outbox('thread')).messages.map((message) =>
  message.delivery ? `${message.content} (${message.delivery.state})` : message.content)
const tick = () => new Promise((resolve) => globalThis.setTimeout(resolve, 0))
const steps = {}

// Two sends while the agent is still starting: neither is accepted yet.
let release = Promise.withResolvers()
accepting = release.promise
const first = send('do the report')
const second = send('and email it')
await tick()
steps.bothWaiting = view()
release.resolve()
await Promise.all([first.settled, second.settled])
steps.bothSaved = view()
steps.outboxAfterSave = outbox('thread').map((entry) => entry.state)

// Accepted, but the history reload fails: the message must not go with it.
historyDown = true
const third = send('one more thing')
await third.settled
steps.acceptedWhileHistoryDown = view()
steps.outboxWhileHistoryDown = outbox('thread').map((entry) => entry.state)
historyDown = false
await queryClient.refetchQueries(history)
steps.afterHistoryReturns = view()
steps.settledAfterHistoryReturns = outboxTranscript(loaded(), outbox('thread')).settled.length

// Refused before it is accepted, then sent again.
refuse = 'agent did not start'
const fourth = send('try this')
await fourth.settled
steps.refused = view()
steps.refusedError = outbox('thread').find((entry) => entry.id === fourth.id).error
steps.savedAfterRefusal = saved.filter((message) => message.content === 'try this').length
refuse = ''
await deliver(outbox('thread').find((entry) => entry.id === fourth.id)).settled
steps.retried = view()
steps.savedAfterRetry = saved.filter((message) => message.content === 'try this').length

// Stopped before it is accepted: kept as not sent, never dropped.
release = Promise.withResolvers()
accepting = release.promise
const fifth = send('stop me')
await tick()
fifth.controller.abort()
await fifth.settled
steps.stopped = view().at(-1)
accepting = Promise.resolve()

// Cloud: the retry of a turn whose answer was lost is a replay, not a second turn.
const sixth = send('cloud turn', 'run')
await sixth.settled
await deliver({ ...newOutboxSend('cloud turn', {}, loaded().at(-1).seq), id: sixth.id }, undefined, 'run').settled
steps.cloudTurns = turns.length
steps.cloudSaved = saved.filter((message) => message.content === 'cloud turn').length

// Cloud, sent while the bot works: accepted and queued, so freshly loaded
// history has no row for it until the running turn ends.
queueTurns = true
const seventh = send('while you work', 'run')
await seventh.settled
steps.queuedBehindRunningTurn = view().at(-1)
accept('while you work')
await queryClient.refetchQueries(history)
steps.queuedTurnStarted = view().filter((content) => content === 'while you work').length
steps.queuedTurnSettled = outboxTranscript(loaded(), outbox('thread')).settled.includes(seventh.id)

// A command the server keeps no row for leaves once history has reloaded.
const eighth = send('/compact')
await eighth.settled
steps.commandLeft = !outbox('thread').some((entry) => entry.id === eighth.id)

queryClient.clear()
globalThis.postMessage(steps)
