import { expect, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import { attachBrowserCloseShortcut } from './browserCloseShortcut'
import { BROWSER_CLOSE_SHORTCUT_CHANNEL } from '../shared/browserCloseShortcut'

test('forwards only an unmodified Command-W from a live webview to its host', () => {
  const messages = []
  const host = { isDestroyed: () => false, send: (...message) => messages.push(message) }
  const guest = Object.assign(new EventEmitter(), { getType: () => 'webview', hostWebContents: host })
  attachBrowserCloseShortcut(guest)
  const input = { type: 'keyDown', key: 'w', meta: true, control: false, alt: false, shift: false, isAutoRepeat: false }
  const send = (changes = {}) => {
    let prevented = false
    guest.emit('before-input-event', { preventDefault: () => { prevented = true } }, { ...input, ...changes })
    return prevented
  }
  for (const changes of [{ type: 'keyUp' }, { key: 'x' }, { meta: false }, { control: true }, { alt: true }, { shift: true }, { isAutoRepeat: true }]) {
    expect(send(changes)).toBe(false)
  }
  expect(messages).toEqual([])
  expect(send()).toBe(true)
  expect(messages).toEqual([[BROWSER_CLOSE_SHORTCUT_CHANNEL]])
  host.isDestroyed = () => true
  expect(send()).toBe(false)
})
