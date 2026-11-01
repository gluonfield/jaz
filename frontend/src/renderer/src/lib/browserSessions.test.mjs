import { expect, test } from 'bun:test'
import { BrowserSessions } from './browserSessions'

test('revealing browser activity preserves the page and only opens its hidden current viewer', () => {
  const sessions = new BrowserSessions()
  const shown = []
  const leave = sessions.bind('first', () => shown.push('first'))
  sessions.open('first', 'https://example.com')
  const target = sessions.getSnapshot()[0].target
  const hide = sessions.present('first', {})
  sessions.show('first')
  expect(shown).toEqual(['first'])
  hide()
  sessions.show('first')
  expect(shown).toEqual(['first', 'first'])
  expect(sessions.getSnapshot()[0].target).toBe(target)
  leave()
  sessions.bind('second', () => shown.push('second'))
  sessions.show('first')
  expect(shown).toEqual(['first', 'first'])
})

test('background browsing reveals its retained page on return without opening another chat panel', async () => {
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
  const leaveAgain = sessions.bind('first', () => shown.push('returned'))
  expect(shown).toEqual(['first'])
  await Promise.resolve()
  expect(shown).toEqual(['first', 'returned'])
  leaveAgain()
  sessions.bind('first', () => shown.push('no new activity'))
  await Promise.resolve()
  expect(shown).toEqual(['first', 'returned'])
})

test('deferred browser reveal follows the current viewer and is discarded when its tab closes or backend changes', async () => {
  const sessions = new BrowserSessions()
  const shown = []
  sessions.open('first', 'https://example.com')
  const leave = sessions.bind('first', () => shown.push('left'))
  leave()
  await Promise.resolve()
  expect(shown).toEqual([])
  sessions.bind('first', () => shown.push('current'))
  await Promise.resolve()
  expect(shown).toEqual(['current'])
  sessions.open('closed', 'https://example.com')
  sessions.bind('closed', () => shown.push('closed'))
  sessions.close('closed')
  await Promise.resolve()
  expect(shown).toEqual(['current'])
  sessions.open('cleared', 'https://example.com')
  sessions.bind('cleared', () => shown.push('cleared'))
  sessions.clear()
  sessions.bind('cleared', () => shown.push('new backend'))
  await Promise.resolve()
  expect(shown).toEqual(['current'])
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

test('closing an extra browser releases only that tab and closing the agent browser leaves it available to reopen', () => {
  const sessions = new BrowserSessions()
  let shown = 0
  sessions.bind('chat', () => shown += 1)
  sessions.open('chat', 'https://example.com/agent')
  sessions.update('extra', { ownerId: 'chat', target: { displayUrl: 'https://example.com/extra', sourceUrl: 'https://example.com/extra' } })
  sessions.close('extra')
  expect(sessions.getSnapshot().map((entry) => entry.id)).toEqual(['chat'])
  expect(sessions.getSnapshot()[0].target.sourceUrl).toBe('https://example.com/agent')
  sessions.close('chat')
  expect(sessions.getSnapshot()[0].target.sourceUrl).toBe('')
  expect(sessions.getSnapshot()[0].generation).toBe(1)
  sessions.open('chat', 'https://example.com/reopened')
  expect(shown).toBe(2)
})

test('browser allocation is synchronous, preserves blank tabs, and reuses only matching URLs in the same conversation', () => {
  const sessions = new BrowserSessions()
  sessions.bind('chat', () => {})
  const first = sessions.openTab('chat')
  const second = sessions.openTab('chat')
  const third = sessions.openTab('chat', 'https://example.com')
  const otherChat = sessions.openTab('other', 'https://example.com')
  expect(first).toBe('chat')
  expect(new Set([first, second, third, otherChat]).size).toBe(4)
  expect(sessions.openTab('chat', 'https://example.com')).toBe(third)
  expect(sessions.getSnapshot().filter((entry) => entry.ownerId === 'chat')).toHaveLength(3)
  sessions.close(first)
  expect(sessions.openTab('chat', 'https://example.com/reopened')).toBe(first)
  expect(sessions.getSnapshot().find((entry) => entry.id === first).generation).toBe(1)
  expect(sessions.getSnapshot().find((entry) => entry.id === third).target.sourceUrl).toBe('https://example.com')
})

test('opening a tab preserves the existing agent browser and creates distinct targets without waiting for a render', () => {
  const sessions = new BrowserSessions()
  sessions.open('chat', 'https://example.com/agent')
  sessions.openTab('chat', 'https://example.com/one')
  sessions.openTab('chat', 'https://example.com/two')
  expect(sessions.getSnapshot().map((entry) => entry.target.sourceUrl)).toEqual([
    'https://example.com/agent', 'https://example.com/one', 'https://example.com/two',
  ])
  expect(sessions.getSnapshot().every((entry) => entry.ownerId === 'chat')).toBe(true)
})

test('duplicating the agent browser opens a separate page in the same conversation and reload targets only existing tabs', () => {
  const sessions = new BrowserSessions()
  sessions.open('chat', 'https://example.com/agent')
  const copy = sessions.duplicate('chat', 'chat')
  const [agent, duplicate] = sessions.getSnapshot()
  expect(duplicate.id).toBe(copy)
  expect(copy).not.toBe('chat')
  expect(duplicate.ownerId).toBe('chat')
  expect(duplicate.target).toBe(agent.target)
  sessions.reload(copy)
  sessions.reload('missing')
  expect(sessions.getSnapshot().map((entry) => [entry.id, entry.reloads])).toEqual([['chat', undefined], [copy, 1]])
})
