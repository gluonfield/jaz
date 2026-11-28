import { afterAll, beforeEach, expect, mock, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import { setTimeout as delay } from 'node:timers/promises'
import { CLAUDE_VOICE_CHANNEL } from '../shared/claudeVoice.ts'

const handlers = new Map()
const ipcMain = Object.assign(new EventEmitter(), {
  handle(channel, fn) {
    handlers.set(channel, fn)
  },
})

const windows = []
let failNextLoad = false

class FakeWindow extends EventEmitter {
  destroyed = false
  sent = []
  webContents = Object.assign(new EventEmitter(), {
    id: 1,
    mainFrame: { url: 'https://claude.ai/new' },
    isDestroyed: () => this.destroyed,
    setWindowOpenHandler() {},
    send: (_channel, requestId, command) => {
      this.sent.push({ requestId, command })
      const value = command.type === 'status'
        ? { available: true }
        : command.type === 'start'
          ? 'v=0\r\nanswer'
          : undefined
      queueMicrotask(() => {
        ipcMain.emit(
          CLAUDE_VOICE_CHANNEL + ':reply',
          { sender: this.webContents, senderFrame: this.webContents.mainFrame },
          { requestId, value },
        )
      })
    },
  })

  constructor() {
    super()
    windows.push(this)
  }

  static fromWebContents(contents) {
    return contents._window
  }

  isDestroyed() {
    return this.destroyed
  }

  loadURL() {
    if (failNextLoad) {
      failNextLoad = false
      return Promise.reject(new Error('network error'))
    }
    return Promise.resolve()
  }

  destroy() {
    if (this.destroyed) return
    this.destroyed = true
    this.emit('closed')
  }
}

mock.module('electron', () => ({
  BrowserWindow: FakeWindow,
  ipcMain,
}))

const prevRendererURL = process.env['ELECTRON_RENDERER_URL']
process.env['ELECTRON_RENDERER_URL'] = 'http://localhost:5173'

const { installClaudeVoice } = await import('./claudeVoice.ts')

function makeOwnerEvent(id) {
  const mainFrame = { url: 'http://localhost:5173/index.html' }
  const ownerWindow = new FakeWindow()
  windows.pop()
  ownerWindow.webContents.id = id
  ownerWindow.webContents.mainFrame = mainFrame
  ownerWindow.webContents.getType = () => 'window'
  ownerWindow.webContents._window = ownerWindow
  return { sender: ownerWindow.webContents, senderFrame: mainFrame }
}

beforeEach(() => {
  windows.length = 0
  handlers.clear()
  failNextLoad = false
})

afterAll(() => {
  if (prevRendererURL === undefined) {
    delete process.env['ELECTRON_RENDERER_URL']
  } else {
    process.env['ELECTRON_RENDERER_URL'] = prevRendererURL
  }
  mock.restore()
})

test('Claude voice bridge stays warm across status and start, then disposes after idle TTL', async () => {
  installClaudeVoice({ origin: 'https://claude.ai', preload: '/fake/preload.js', idleTimeoutMs: 40 })
  const invoke = handlers.get(CLAUDE_VOICE_CHANNEL)
  const event = makeOwnerEvent(101)

  const status = await invoke(event, { type: 'status' })
  expect(status).toEqual({ available: true })
  expect(windows).toHaveLength(1)
  expect(windows[0].isDestroyed()).toBe(false)

  const answer = await invoke(event, {
    type: 'start',
    id: 'call-1',
    options: { sdp: 'v=0\r\noffer', context: 'ctx', voice: 'buttery' },
  })
  expect(answer).toBe('v=0\r\nanswer')
  expect(windows).toHaveLength(1)
  expect(windows[0].sent.map((item) => item.command.type)).toEqual(['status', 'start'])

  await invoke(event, { type: 'close', id: 'call-1' })
  expect(windows[0].isDestroyed()).toBe(false)

  await delay(80)
  expect(windows[0].isDestroyed()).toBe(true)
})

test('Claude voice bridge disposes immediately when initial page load fails', async () => {
  installClaudeVoice({ origin: 'https://claude.ai', preload: '/fake/preload.js', idleTimeoutMs: 5000 })
  const invoke = handlers.get(CLAUDE_VOICE_CHANNEL)
  const event = makeOwnerEvent(102)

  failNextLoad = true
  await expect(invoke(event, { type: 'status' })).rejects.toThrow('network error')
  expect(windows).toHaveLength(1)
  expect(windows[0].isDestroyed()).toBe(true)

  const status = await invoke(event, { type: 'status' })
  expect(status).toEqual({ available: true })
  expect(windows).toHaveLength(2)
  windows[1].destroy()
})
