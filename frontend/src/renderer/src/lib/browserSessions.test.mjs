import { expect, test } from 'bun:test'
import { BrowserSessions } from './browserSessions'

test('leaving a conversation retains its browser without opening another chat panel', () => {
  const sessions = new BrowserSessions()
  const shown = []
  const leave = sessions.bind('first', () => shown.push('first'))
  sessions.open('first', 'https://example.com/first')
  leave()
  sessions.bind('second', () => shown.push('second'))
  sessions.open('first', 'https://example.com/background')
  expect(shown).toEqual(['first'])
  expect(sessions.getSnapshot().find((entry) => entry.id === 'first').target.sourceUrl).toBe('https://example.com/background')
  expect(sessions.getSnapshot().find((entry) => entry.id === 'second').target.sourceUrl).toBe('')
})

test('late presentation cleanup cannot detach a newer viewer or change the page target', () => {
  const sessions = new BrowserSessions()
  const shown = []
  const leaveOld = sessions.bind('first', () => shown.push('old'))
  sessions.open('first', 'https://example.com')
  const target = sessions.getSnapshot()[0].target
  const hideOld = sessions.present('first', { onClose() {} })
  const next = { onClose() {} }
  const hideNew = sessions.present('first', next)
  sessions.bind('first', () => shown.push('new'))
  leaveOld()
  hideOld()
  expect(sessions.getSnapshot()[0].presentation).toBe(next)
  expect(sessions.getSnapshot()[0].target).toBe(target)
  hideNew()
  expect(sessions.getSnapshot()[0].presentation).toBeUndefined()
  expect(sessions.getSnapshot()[0].target).toBe(target)
  sessions.open('first', 'https://example.com/next')
  expect(shown).toEqual(['old', 'new'])
})

test('switching backend clears retained browsers and their previous viewers', () => {
  const sessions = new BrowserSessions()
  let shown = 0
  sessions.bind('first', () => shown += 1)
  sessions.open('first', 'https://example.com/first')
  sessions.clear()
  expect(sessions.getSnapshot()).toEqual([])
  sessions.open('first', 'https://example.com/new-backend')
  expect(shown).toBe(1)
})
