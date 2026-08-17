const storage = {
  getItem: (key) => key === 'jaz.telemetry.enabled' ? 'false' : null,
  removeItem: () => {},
  setItem: () => {},
}
globalThis.window = { jaz: undefined, location: { origin: 'http://localhost' }, localStorage: storage }
globalThis.localStorage = storage

const requests = []
const started = Promise.withResolvers()
const durable = Promise.withResolvers()

globalThis.fetch = async (input, init) => {
  const url = new globalThis.URL(String(input))
  const body = JSON.parse(String(init?.body ?? '{}'))
  requests.push({ path: url.pathname, body })
  if (url.pathname === '/v1/sessions') {
    return globalThis.Response.json({ id: 'session-1' })
  }
  if (url.pathname === '/v1/sessions/session-1/queue') {
    started.resolve()
    await durable.promise
    return globalThis.Response.json({ id: 'session-1' })
  }
  throw new Error(`unexpected request ${url.pathname}`)
}

const { submitNewSession } = await import('./-newSessionSubmission')
let route = '/new'
let settled = false
let initialPrompt
const pending = submitNewSession(
  { title: 'Inspect this' },
  ' inspect this ',
  {
    planRequested: true,
    goalRequested: true,
    attachments: [{ id: 'attachment-1', name: 'evidence.txt' }],
    contexts: [{ id: 'selection-1', type: 'selection', text: ' evidence ', comment: ' note ' }],
  },
  (prompt) => {
    route = `/sessions/${prompt.sessionId}`
    initialPrompt = prompt
  },
).then(() => {
  settled = true
})

await started.promise
const beforeAcknowledgement = { route, settled }
durable.resolve()
await pending
const { created_at, ...displayMessage } = initialPrompt.message
const { optimisticTranscriptMessages, pendingOptimisticUserMessage } = await import('../lib/optimisticUserMessage')
const beforeHistory = []
const projectedBeforeHistory = optimisticTranscriptMessages(
  beforeHistory,
  pendingOptimisticUserMessage(beforeHistory, initialPrompt),
).map((message) => message.content)
const afterHistory = [{ ...initialPrompt.message, seq: 1, content: 'server copy' }]
const projectedAfterHistory = optimisticTranscriptMessages(
  afterHistory,
  pendingOptimisticUserMessage(afterHistory, initialPrompt),
).map((message) => message.content)
globalThis.postMessage({
  beforeAcknowledgement,
  route,
  initialPrompt: {
    sessionId: initialPrompt.sessionId,
    baselineMessageSeq: initialPrompt.baselineMessageSeq,
    message: {
      ...displayMessage,
      validTimestamp: !Number.isNaN(Date.parse(created_at)),
    },
  },
  projectedBeforeHistory,
  projectedAfterHistory,
  requests,
})
