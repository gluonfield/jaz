import { describe, expect, test } from 'bun:test'
import {
  diffThreadCompletions,
  parseThreadCompletions,
  parseThreadNotificationConfig,
  threadNotificationPath,
} from './notifications'

describe('thread notifications', () => {
  test('normalizes monitor configuration at the IPC boundary', () => {
    expect(
      parseThreadNotificationConfig({
        enabled: true,
        baseUrl: ' https://jaz.example/path ',
        token: ' secret ',
      }),
    ).toEqual({
      enabled: true,
      baseUrl: 'https://jaz.example',
      token: 'secret',
    })
    expect(parseThreadNotificationConfig({ enabled: false })).toEqual({
      enabled: false,
    })
    expect(
      parseThreadNotificationConfig({
        enabled: true,
        baseUrl: 'file:///tmp',
        token: '',
      }),
    ).toBeNull()
  })

  test('parses the completion projection and rejects malformed payloads', () => {
    expect(
      parseThreadCompletions({
        items: [
          {
            id: ' thread-1 ',
            slug: ' first-thread ',
            title: ' Finished work ',
            completed_at: '2026-07-18T10:00:00Z',
          },
        ],
      }),
    ).toEqual([
      {
        id: 'thread-1',
        slug: 'first-thread',
        title: 'Finished work',
        completedAt: '2026-07-18T10:00:00Z',
      },
    ])
    expect(parseThreadCompletions({ items: [{ id: 'thread-1' }] })).toBeNull()
    expect(parseThreadCompletions(null)).toBeNull()
  })

  test('baselines existing items and detects a newer completion', () => {
    const item = {
      id: 'thread-1',
      slug: 'thread-1',
      title: '',
      completedAt: '2026-07-18T10:00:00Z',
    }
    const first = diffThreadCompletions(null, [item])
    expect(first.added).toEqual([])

    const next = { ...item, completedAt: '2026-07-18T10:01:00Z' }
    expect(diffThreadCompletions(first.history, [next]).added).toEqual([next])
  })

  test('retains completion identity while an item is temporarily absent', () => {
    const item = {
      id: 'thread-1',
      slug: 'thread-1',
      title: '',
      completedAt: '2026-07-18T10:00:00Z',
    }
    const baseline = diffThreadCompletions(null, [item])
    const absent = diffThreadCompletions(baseline.history, [])

    expect(diffThreadCompletions(absent.history, [item]).added).toEqual([])

    const next = { ...item, completedAt: '2026-07-18T10:01:00Z' }
    expect(diffThreadCompletions(absent.history, [next]).added).toEqual([next])
  })

  test('encodes the thread id before deep-linking', () => {
    expect(threadNotificationPath('thread/with spaces')).toBe('/sessions/thread%2Fwith%20spaces')
  })
})
