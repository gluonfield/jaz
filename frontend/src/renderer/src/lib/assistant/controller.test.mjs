import { expect, test } from 'bun:test'
import { createAssistantController } from './controller'

const PRIMARY_BOT = {
  id: 'bot-prime',
  kind: 'bot',
  name: 'Prime',
  main: true,
  avatar: { shape: 'circle', color: 'blue' },
  unread: false,
  status: 'idle',
  created_at: '2026-11-27T00:00:00Z',
  updated_at: '2026-11-27T00:00:00Z',
}

function makeHarness(overrides = {}) {
  let clock = 1000
  let wakeMode = 'dictate'
  let cloud = false
  let voiceAvailable = true
  let voice = { phase: 'off', sessionId: null, connected: [], started: 0 }
  const published = []
  const sent = []
  const routes = []
  const toasts = []
  const dictationCalls = []
  let focusCalls = 0
  let nextId = 1

  const dictation = {
    availability: async () => ({ available: true }),
    start: async (id) => { dictationCalls.push(['start', id]) },
    stop: async (id) => { dictationCalls.push(['stop', id]) },
    cancel: async (id) => { dictationCalls.push(['cancel', id]) },
  }

  const controller = createAssistantController({
    getWakeMode: () => wakeMode,
    isCloud: () => cloud,
    getVoice: () => ({
      phase: voice.phase,
      sessionId: voice.sessionId,
      connect: (id) => voice.connected.push(id),
      start: () => { voice.started += 1 },
    }),
    getDictation: () => dictation,
    resolvePrimaryBot: async () => PRIMARY_BOT,
    isVoiceAvailable: async () => voiceAvailable,
    sendBotMessage: async (botId, text) => { sent.push([botId, text]) },
    focusMain: () => { focusCalls += 1 },
    openRoute: (path) => routes.push(path),
    toast: (message) => toasts.push(message),
    publish: (state) => published.push(state),
    now: () => clock,
    newId: () => `dict-${nextId++}`,
    tickMs: 50,
    sentHoldMs: 50,
    errorHoldMs: 50,
    ...overrides,
  })

  return {
    controller,
    dictation,
    published,
    sent,
    routes,
    toasts,
    dictationCalls,
    get focusCalls() { return focusCalls },
    setClock: (v) => { clock = v },
    setWakeMode: (m) => { wakeMode = m },
    setCloud: (v) => { cloud = v },
    setVoiceAvailable: (v) => { voiceAvailable = v },
    voice,
  }
}

test('Mode A opens primary bot, focuses Jaz, and starts live voice when available', async () => {
  const h = makeHarness()
  h.setWakeMode('voice')
  await h.controller.handleAction({ mode: 'default' })
  expect(h.focusCalls).toBe(1)
  expect(h.routes).toEqual(['/bots/bot-prime'])
  expect(h.voice.connected).toEqual(['bot-prime'])
  expect(h.toasts).toEqual([])
})

test('Mode A opens primary bot with a helpful toast in Cloud workspaces without connecting local voice', async () => {
  const h = makeHarness()
  h.setCloud(true)
  await h.controller.handleAction({ mode: 'voice' })
  expect(h.focusCalls).toBe(1)
  expect(h.routes).toEqual(['/bots/bot-prime'])
  expect(h.voice.connected).toEqual([])
  expect(h.toasts[0]).toContain('Cloud')
})

test('Mode B quick dictation streams partial text, preserves audio level, auto-stops on silence, and sends without focusing Jaz', async () => {
  const h = makeHarness()
  await h.controller.handleAction({ mode: 'dictate' })
  expect(h.focusCalls).toBe(0)
  expect(h.dictationCalls).toEqual([['start', 'dict-1']])

  h.controller.handleDictationEvent('dict-1', { type: 'status', phase: 'recording' })
  h.controller.handleDictationEvent('dict-1', { type: 'level', level: 0.4 })
  h.controller.handleDictationEvent('dict-1', { type: 'result', text: 'check lead times' })
  expect(h.published.at(-1)).toMatchObject({ phase: 'listening', text: 'check lead times', level: 0.4 })

  h.setClock(2700)
  h.controller.handleDictationEvent('dict-1', { type: 'level', level: 0.002 })
  expect(h.dictationCalls).toEqual([['start', 'dict-1'], ['stop', 'dict-1']])

  h.controller.handleDictationEvent('dict-1', { type: 'complete', text: 'check lead times' })
  await new Promise((resolve) => setTimeout(resolve, 10))
  expect(h.sent).toEqual([['bot-prime', 'check lead times']])
  expect(h.published.at(-1)).toMatchObject({ phase: 'sent', botName: 'Prime', text: 'check lead times' })
  expect(h.focusCalls).toBe(0)
})

test('concurrent Mode B triggers do not double-start dictation, and re-trigger while listening stops and sends', async () => {
  const gate = Promise.withResolvers()
  const h = makeHarness({
    resolvePrimaryBot: () => gate.promise,
  })
  const first = h.controller.handleAction({ mode: 'dictate' })
  const second = h.controller.handleAction({ mode: 'dictate' })
  gate.resolve(PRIMARY_BOT)
  await Promise.all([first, second])
  expect(h.dictationCalls).toEqual([['start', 'dict-1']])

  h.controller.handleDictationEvent('dict-1', { type: 'status', phase: 'recording' })
  await h.controller.handleAction({ mode: 'dictate' })
  expect(h.dictationCalls).toEqual([['start', 'dict-1'], ['stop', 'dict-1']])
})

test('cancel during async startup prevents stale dictation start from leaking', async () => {
  const gate = Promise.withResolvers()
  const h = makeHarness({ resolvePrimaryBot: () => gate.promise })
  const pending = h.controller.handleAction({ mode: 'dictate' })
  h.controller.cancel(true)
  gate.resolve(PRIMARY_BOT)
  await pending
  expect(h.dictationCalls).toEqual([])
})

test('Mode B direct text send posts straight to primary bot without focusing Jaz', async () => {
  const h = makeHarness()
  await h.controller.handleAction({ mode: 'send', text: 'Follow up with supplier' })
  expect(h.focusCalls).toBe(0)
  expect(h.sent).toEqual([['bot-prime', 'Follow up with supplier']])
  expect(h.published.at(-1)).toMatchObject({ phase: 'sent', text: 'Follow up with supplier' })

  h.controller.handlePillCommand('open')
  expect(h.focusCalls).toBe(1)
  expect(h.routes).toEqual(['/bots/bot-prime'])
})
