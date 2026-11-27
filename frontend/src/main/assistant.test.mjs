import { expect, mock, test } from 'bun:test'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { ASSISTANT_CHANNEL } from '../shared/assistant'
import { ISLAND_CAPSULE_WIDTH, ISLAND_HEIGHT } from '../shared/notifications'

if (process.env.JAZ_ASSISTANT_TEST_CHILD === '1') {
  Object.defineProperty(process, 'platform', { value: 'darwin' })
  let notchReply = {
    model: 'Mac16,5',
    screens: [{ id: 1, notch: { x: 918, width: 220, height: 38 } }],
    focus: 'on',
    focus_access: 'denied',
    previews: 'never',
    notifications: 'denied',
  }
  let internalDisplay = true
  const listeners = new Map()
  const windows = []

  const emitIpc = (channel, event, ...args) => {
    for (const handler of listeners.get(channel) ?? []) handler(event, ...args)
  }

  mock.module('node:child_process', () => ({
    spawn: () => ({ on() {} }),
    execFile: (_file, _args, _options, callback) => {
      if (notchReply) callback(null, { stdout: JSON.stringify(notchReply), stderr: '' })
      else callback(new Error('helper missing'))
    },
  }))

  mock.module('electron', () => ({
    app: { isPackaged: true, getAppPath: () => '/Jaz' },
    ipcMain: {
      on(channel, handler) {
        const list = listeners.get(channel) ?? []
        list.push(handler)
        listeners.set(channel, list)
      },
      removeListener(channel, handler) {
        const list = listeners.get(channel) ?? []
        listeners.set(channel, list.filter((item) => item !== handler))
      },
    },
    screen: {
      getCursorScreenPoint: () => ({ x: 10, y: 10 }),
      getDisplayNearestPoint: () => ({
        id: 1,
        internal: internalDisplay,
        bounds: { x: 0, y: 0, width: 1512, height: 982 },
        workArea: { x: 0, y: 38, width: 1512, height: 944 },
      }),
    },
    webContents: { getFocusedWebContents: () => null },
    BrowserWindow: class {
      constructor(options) {
        this.options = options
        this.shown = 0
        this.shownInactive = 0
        this.focused = 0
        this.visible = false
        this.ignoredMouse = null
        this.sent = []
        this.webContents = {
          mainFrame: { id: 1 },
          throttled: true,
          getBackgroundThrottling() {
            return this.throttled
          },
          setBackgroundThrottling(value) {
            this.throttled = value
          },
          send: (channel, payload) => this.sent.push([channel, payload]),
          on() {},
        }
        windows.push(this)
      }
      isDestroyed() {
        return false
      }
      isMinimized() {
        return false
      }
      isVisible() {
        return this.visible
      }
      restore() {}
      once() {}
      setAlwaysOnTop() {}
      setVisibleOnAllWorkspaces() {}
      setIgnoreMouseEvents(ignore) {
        this.ignoredMouse = ignore
      }
      loadFile() {}
      setBounds(bounds) {
        this.bounds = bounds
      }
      show() {
        this.visible = true
        this.shown++
      }
      showInactive() {
        this.visible = true
        this.shownInactive++
      }
      focus() {
        this.focused++
      }
      hide() {
        this.visible = false
      }
      destroy() {}
    },
  }))

  const { BrowserWindow } = await import('electron')
  const { createAssistant } = await import('./assistant')

  test('queues assistant deep links until renderer is ready and keeps Mode B non-activating even when Focus/Notifications are blocked', async () => {
    const main = new BrowserWindow({})
    const assistant = createAssistant(() => main, () => main)
    assistant.attach(main)

    assistant.dispatch({ mode: 'dictate' })
    expect(main.sent).toEqual([])
    expect(main.focused).toBe(0)

    const ownerEvent = { sender: main.webContents, senderFrame: main.webContents.mainFrame }
    emitIpc(`${ASSISTANT_CHANNEL}ready`, ownerEvent)
    expect(main.sent).toEqual([[`${ASSISTANT_CHANNEL}trigger`, { mode: 'dictate' }]])
    expect(main.focused).toBe(0)

    emitIpc(`${ASSISTANT_CHANNEL}publish`, ownerEvent, {
      phase: 'listening',
      botName: 'Prime',
      botId: 'bot-1',
      text: '',
      level: 0.2,
    })
    await new Promise((resolve) => setTimeout(resolve, 20))

    const pill = windows[1]
    expect(pill).toBeDefined()
    expect(pill.options).toMatchObject({ focusable: false, type: 'panel', roundedCorners: false })
    expect(pill.shownInactive).toBe(1)
    expect(pill.focused).toBe(0)
    expect(main.focused).toBe(0)
    expect(pill.bounds).toEqual({ x: 778, y: 0, width: 500, height: ISLAND_HEIGHT })

    const pillEvent = { sender: pill.webContents, senderFrame: pill.webContents.mainFrame }
    emitIpc(`${ASSISTANT_CHANNEL}pill-ready`, pillEvent)
    expect(pill.sent.at(-1)?.[0]).toBe(`${ASSISTANT_CHANNEL}pill-state`)
    expect(pill.sent.at(-1)?.[1]).toMatchObject({ phase: 'listening', botName: 'Prime', notch: { width: 220, height: 38 } })

    emitIpc(`${ASSISTANT_CHANNEL}pill-command`, pillEvent, { type: 'hover', inside: true })
    expect(pill.ignoredMouse).toBe(false)
    emitIpc(`${ASSISTANT_CHANNEL}pill-command`, pillEvent, { type: 'send' })
    expect(main.sent.at(-1)).toEqual([`${ASSISTANT_CHANNEL}pill-command`, { type: 'send' }])

    emitIpc(`${ASSISTANT_CHANNEL}focus-main`, ownerEvent)
    expect(main.shown).toBe(1)
    expect(main.focused).toBe(1)

    emitIpc(`${ASSISTANT_CHANNEL}publish`, ownerEvent, null)
    internalDisplay = false
    emitIpc(`${ASSISTANT_CHANNEL}publish`, ownerEvent, {
      phase: 'sent',
      botName: 'Prime',
      botId: 'bot-1',
      text: 'Hello',
      level: 0,
    })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(pill.bounds).toEqual({
      x: Math.round((1512 - ISLAND_CAPSULE_WIDTH) / 2),
      y: 46,
      width: ISLAND_CAPSULE_WIDTH,
      height: ISLAND_HEIGHT,
    })

    assistant.teardown()
  })
} else {
  test('assistant main controller handles deep links and top-of-screen pill placement', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_ASSISTANT_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 8000,
    })
    expect({ status: result.status, output: result.status ? result.stderr : '' }).toEqual({ status: 0, output: '' })
  })
}
