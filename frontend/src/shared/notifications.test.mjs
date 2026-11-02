import { describe, expect, test } from 'bun:test'
import { alertLabel, alertPath, parseAlert, parseNotificationConfig } from './notifications'

describe('notifications', () => {
  test('normalizes the stream configuration at the IPC boundary', () => {
    expect(parseNotificationConfig({ mode: 'notch', baseUrl: ' https://jaz.example/path ', token: ' secret ' })).toEqual({
      mode: 'notch',
      baseUrl: 'https://jaz.example',
      token: 'secret',
    })
    expect(parseNotificationConfig({ mode: 'off' })).toEqual({ mode: 'off' })
    expect(parseNotificationConfig({ mode: 'center', baseUrl: 'file:///tmp', token: '' })).toBeNull()
    expect(parseNotificationConfig({ enabled: true, baseUrl: 'https://jaz.example', token: '' })).toBeNull()
  })

  test('parses the backend alert and drops malformed avatars', () => {
    const alert = parseAlert({
      session_id: ' bot-1 ',
      key: 'task:4',
      state: 'done',
      kind: 'bot',
      name: 'Jaz AGI',
      avatars: [{ shape: 'blob', color: 'teal' }, { shape: 'star', color: 'teal' }],
      title: 'Side browser address bar',
      body: 'Search now goes to Google.',
      at: '2026-11-01T22:00:00.5Z',
    })
    expect(alert).toEqual({
      sessionId: 'bot-1',
      key: 'task:4',
      state: 'done',
      kind: 'bot',
      name: 'Jaz AGI',
      avatars: [{ shape: 'blob', color: 'teal' }],
      title: 'Side browser address bar',
      body: 'Search now goes to Google.',
      requestId: '',
      at: Date.parse('2026-11-01T22:00:00.5Z'),
    })
    expect(alertPath(alert)).toBe('/bots/bot-1')
    expect(alertLabel(alert)).toBe('Done')
    expect(alertLabel({ state: 'reply' })).toBe('')
    expect(alertPath({ sessionId: 'a b', kind: 'chat' })).toBe('/sessions/a%20b')
    expect(parseAlert({ session_id: 'x', key: 'k', state: 'finished', kind: 'chat', at: '2026-11-01T22:00:00Z' })).toBeNull()
    expect(parseAlert({ session_id: 'x', key: 'k', state: 'reply', kind: 'chat', at: 'later' })).toBeNull()
  })
})
