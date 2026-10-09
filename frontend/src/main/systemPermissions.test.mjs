import { afterAll, expect, mock, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import process from 'node:process'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

if (process.env.JAZ_SYSTEM_PERMISSIONS_TEST_CHILD === '1') {
  const handlers = new Map()
  const opened = []
  const app = { isPackaged: true }
  let tests = 0
  mock.module('electron', () => ({
    app,
    ipcMain: { handle: (name, handler) => handlers.set(name, handler) },
    shell: { openExternal: async (url) => opened.push(url) },
    systemPreferences: {
      getMediaAccessStatus: () => 'denied',
      askForMediaAccess: async () => {
        throw new Error('A refused microphone must not prompt again')
      },
    },
  }))
  mock.module('@trycua/cua-driver', () => ({
    currentMacOsPermissionStatus: () => ({ accessibility: true, screenRecording: false }),
  }))
  mock.module('@main/notifications', () => ({ sendTestNotification: async () => (tests++, false), openNotificationSettings: async () => opened.push('notifications') }))
  mock.module('@main/permissionGuide', () => ({ guidePermission: async () => {} }))
  const { installSystemPermissions } = await import('./systemPermissions')
  installSystemPermissions()
  afterAll(() => mock.restore())

  function event(type = 'window', url = 'file:///Applications/Jaz.app/Contents/Resources/app.asar/out/renderer/index.html') {
    const sender = Object.assign(new EventEmitter(), { id: 7, getType: () => type, mainFrame: { url } })
    return { sender, senderFrame: sender.mainFrame }
  }
  const invoke = (name, ...args) => handlers.get('jaz:system-permissions:' + name)(...args)

  test('system permission IPC answers only trusted Jaz windows and known permissions', () => {
    for (const request of [event('webview'), event('window', 'https://untrusted.example/renderer/index.html'), { ...event(), senderFrame: { url: 'file:///renderer/index.html' } }]) {
      expect(() => invoke('status', request)).toThrow('trusted Jaz window')
      expect(() => invoke('allow', request, 'microphone')).toThrow('trusted Jaz window')
    }
    expect(() => invoke('allow', event(), 'camera')).toThrow('Invalid system permission')
    expect(opened).toEqual([])
  })

  test('status maps macOS answers, and Allow on a refused microphone opens its privacy list', async () => {
    expect(await invoke('status', event())).toEqual({ accessibility: 'granted', screenRecording: 'needed', microphone: 'off', notifications: 'needed' })
    await invoke('allow', event(), 'microphone')
    expect(opened).toEqual(['x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone'])
  })

  test('a refused notification turns the permission off, and Allow then opens its settings', async () => {
    await invoke('allow', event(), 'notifications')
    expect((await invoke('status', event())).notifications).toBe('off')
    expect(opened).not.toContain('notifications')
    await invoke('allow', event(), 'notifications')
    expect(opened).toContain('notifications')
  })

  test('a development build reports notifications unavailable and never tries one', async () => {
    app.isPackaged = false
    const before = tests
    expect((await invoke('status', event())).notifications).toBe('unavailable')
    await invoke('allow', event(), 'notifications')
    expect(tests).toBe(before)
  })
} else {
  test('system permission IPC in an isolated Electron module fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_SYSTEM_PERMISSIONS_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 10000,
    })
    expect({ status: result.status, error: result.error?.message, output: result.status ? result.stderr : '' }).toEqual({ status: 0, error: undefined, output: '' })
  })
}
