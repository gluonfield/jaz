import { expect, mock, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

if (process.env.JAZ_NOTIFICATION_TEST_CHILD === '1') {
  let outcome = 'show'
  const notifications = []
  mock.module('electron', () => ({
    app: {},
    BrowserWindow: {},
    shell: {},
    Notification: class extends EventEmitter {
      constructor() {
        super()
        notifications.push(this)
      }
      show() {
        if (outcome !== 'silent') queueMicrotask(() => this.emit(outcome))
      }
    },
  }))
  mock.module('../assets/jaz-icon-1024.png?asset', () => ({ default: 'icon' }))
  const { sendTestNotification } = await import('./notifications')
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
