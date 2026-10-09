import { afterAll, expect, mock, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import process from 'node:process'

import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

if (process.env.JAZ_COMPUTER_IPC_TEST_CHILD === '1') {
  const handlers = new Map()
  const app = new EventEmitter()
  app.quit = () => {}
  app.getAppPath = () => process.cwd()
  const sdk = {
    currentMacOsPermissionStatus: () => ({ accessibility: false, screenRecording: false }),
  }
  mock.module('electron', () => ({
    app,
    ipcMain: { handle: (name, handler) => handlers.set(name, handler) },
  }))
  mock.module('@trycua/cua-driver', () => sdk)
  const { installComputerControl } = await import('./computerControl')
  installComputerControl()
  afterAll(() => mock.restore())

  function event(type = 'window', url = 'file:///Applications/Jaz.app/Contents/Resources/app.asar/out/renderer/index.html') {
    const sender = Object.assign(new EventEmitter(), { id: 12, getType: () => type, mainFrame: { url } })
    return { sender, senderFrame: sender.mainFrame }
  }
  const invoke = (name, ...args) => handlers.get('jaz:computer:' + name)(...args)

  test('computer IPC denies webviews, foreign documents and subframes', async () => {
    for (const request of [event('webview'), event('window', 'https://untrusted.example/renderer/index.html'), { ...event(), senderFrame: { url: 'file:///renderer/index.html' } }]) {
      expect(() => invoke('status', request)).toThrow('trusted Jaz window')
      expect(() => invoke('begin', request, 'request', 'thread')).toThrow('trusted Jaz window')
    }
  })

  test('status reports the computer use permissions', async () => {
    const status = await invoke('status', event())
    expect(status.platform).toBe(process.platform)
    if (process.platform === 'darwin') {
      expect(status.permissions).toEqual({ accessibility: false, screenRecording: false })
    }
  })

  test('computer IPC rejects malformed IDs and non-object arguments before native execution', () => {
    expect(() => invoke('begin', event(), '../bad', 'thread')).toThrow('command ID')
    expect(() => invoke('begin', event(), undefined, 'thread')).toThrow('command ID')
    expect(() => invoke('call', event(), 'request', { name: 'click', args: [] })).toThrow('arguments')
    expect(() => invoke('call', event(), 'request', { name: 'click', args: { text: 'a'.repeat(150000) } })).toThrow('arguments')
  })
} else {
  test('computer IPC trust and permission boundary in an isolated Electron module fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_COMPUTER_IPC_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 10000,
    })
    expect({ status: result.status, error: result.error?.message, output: result.status ? result.stderr : '' }).toEqual({ status: 0, error: undefined, output: '' })
  })
}
