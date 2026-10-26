const storage = {
  getItem: (key) => key === 'jaz.telemetry.enabled' ? 'false' : null,
  removeItem: () => {},
  setItem: () => {},
}
globalThis.window = { jaz: undefined, location: { origin: 'http://localhost' }, localStorage: storage }
globalThis.localStorage = storage

globalThis.onmessage = async (event) => {
  const requests = []
  const started = Promise.withResolvers()
  const durable = Promise.withResolvers()
  const parameters = new globalThis.URLSearchParams(event.data)
  const fastMode = parameters.get('fast')
  const configStarted = Promise.withResolvers()
  const configured = Promise.withResolvers()

  globalThis.fetch = async (input, init) => {
    const url = new globalThis.URL(String(input))
    const body = JSON.parse(String(init?.body ?? '{}'))
    requests.push({ path: url.pathname, body })
    if (url.pathname === '/v1/sessions') {
      return globalThis.Response.json({ id: 'session-1' })
    }
    if (url.pathname === '/v1/sessions/session-1/agent/config') {
      configStarted.resolve()
      await configured.promise
      return parameters.has('reject')
        ? globalThis.Response.json({ error: 'Fast Mode is unavailable' }, { status: 409 })
        : new globalThis.Response(null, { status: 204 })
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
  let error
  const pending = submitNewSession(
    { title: 'Inspect this', ...(fastMode ? { config_options: { 'fast-mode': fastMode } } : {}) },
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
  }).catch((failure) => {
    error = failure.message
  })

  let beforeConfigAcknowledgement
  if (fastMode) {
    await configStarted.promise
    await new Promise((resolve) => globalThis.setTimeout(resolve, 0))
    beforeConfigAcknowledgement = { route, settled, requests: [...requests] }
    configured.resolve()
  }
  if (parameters.has('reject')) {
    await pending
    globalThis.postMessage({ route, settled, error, requests, beforeConfigAcknowledgement })
  } else {
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
    const afterHistory = [{ ...initialPrompt.message, seq: 1, from_user: true, content: 'server copy' }]
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
      ...(fastMode ? { beforeConfigAcknowledgement } : {}),
    })
  }
}
