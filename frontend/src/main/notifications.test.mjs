import { expect, mock, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

if (process.env.JAZ_NOTIFICATION_TEST_CHILD === '1') {
  let outcome = 'show'
  let focused = null
  let islandShows = true
  let onAlert = () => {}
  const notifications = []
  const presented = []
  mock.module('electron', () => ({
    app: { on() {} },
    BrowserWindow: { getFocusedWindow: () => focused },
    ipcMain: {},
    powerMonitor: {},
    screen: {},
    shell: {},
    Notification: class extends EventEmitter {
      static isSupported() {
        return true
      }
      constructor(options) {
        super()
        this.options = options
        notifications.push(this)
      }
      show() {
        if (outcome !== 'silent') queueMicrotask(() => this.emit(outcome))
      }
    },
  }))
  mock.module('../assets/jaz-icon-1024.png?asset', () => ({ default: 'icon' }))
  mock.module('./island', () => ({
    createIsland: () => ({
      present: async (alert) => {
        presented.push(alert.key)
        return islandShows
      },
      clear() {},
    }),
  }))
  mock.module('./notificationStream', () => ({
    createAlertStream: (handler) => {
      onAlert = handler
      return { follow() {} }
    },
  }))
  const { createNotifications, sendTestNotification } = await import('./notifications')
  test('each alert reaches the island or Notification Center once, and nothing while Jaz is focused', async () => {
    const router = createNotifications(() => {})
    const settle = () => new Promise((resolve) => setTimeout(resolve, 0))
    const alert = (key, state) => ({ sessionId: 'thread', key, state, kind: 'chat', name: '', avatars: [], title: 'Fix the build', body: 'Fixed.', requestId: '', at: 0 })
    const target = { baseUrl: 'http://jaz.test', token: 'token' }
    router.configure({ mode: 'notch', ...target })
    onAlert(alert('done', 'done'))
    onAlert(alert('done', 'done'))
    await settle()
    expect({ presented, shown: notifications.length }).toEqual({ presented: ['done'], shown: 0 })
    islandShows = false
    onAlert(alert('question', 'needs_you'))
    await settle()
    expect(notifications.map((item) => item.options)).toEqual([
      { title: 'Fix the build', subtitle: 'Jaz', body: 'Fixed.', groupId: 'thread', silent: false, sound: 'Blow' },
    ])
    router.configure({ mode: 'center', ...target })
    onAlert(alert('reply', 'reply'))
    await settle()
    focused = {}
    onAlert(alert('later', 'reply'))
    await settle()
    expect(presented).toEqual(['done', 'question'])
    expect(notifications.map((item) => item.options.silent)).toEqual([false, true])
    notifications.length = 0
  })
  test('notification delivery uses show events, handles failure, and bounds silent OS responses', async () => {
    expect(await sendTestNotification(1)).toBe(true)
    expect(notifications.at(-1).eventNames()).toEqual([])
    outcome = 'failed'
    expect(await sendTestNotification(1)).toBe(false)
    outcome = 'silent'
    expect(await sendTestNotification(1)).toBe(false)
    expect(notifications.at(-1).eventNames()).toEqual([])
    outcome = 'show'
    expect(await sendTestNotification(1)).toBe(true)
  })
} else {
  test('notification OS boundary in an isolated Electron fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_NOTIFICATION_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 8000,
    })
    expect({ status: result.status, output: result.status ? result.stderr : '' }).toEqual({ status: 0, output: '' })
  })
}
