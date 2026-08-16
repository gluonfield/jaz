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
const pending = submitNewSession(
  { title: 'Inspect this' },
  ' inspect this ',
  {
    planRequested: true,
    goalRequested: true,
    attachments: [{ id: 'attachment-1' }],
    contexts: [{ id: 'selection-1', type: 'selection', text: ' evidence ', comment: ' note ' }],
  },
  (sessionId) => {
    route = `/sessions/${sessionId}`
  },
).then(() => {
  settled = true
})

await started.promise
const beforeAcknowledgement = { route, settled }
durable.resolve()
await pending
globalThis.postMessage({ beforeAcknowledgement, route, requests })
