import { describe, expect, test } from 'bun:test'
import { createThreadCompletionMonitor } from './notificationMonitor'

const completion = (completedAt) => ({
  items: [
    {
      id: 'thread-1',
      slug: 'first-thread',
      title: 'First thread',
      completed_at: completedAt,
    },
  ],
})

describe('thread completion monitor', () => {
  test('preserves its baseline when a renderer repeats the same authenticated configuration', async () => {
    let payload = completion('2026-07-18T10:00:00Z')
    const requests = []
    const seen = []
    const monitor = createThreadCompletionMonitor(
      (item) => seen.push(item),
      (url, init) => {
        requests.push({ url, authorization: init?.headers?.Authorization })
        return Promise.resolve({ ok: true, json: () => Promise.resolve(payload) })
      },
    )

    expect(
      await monitor.configure({
        enabled: true,
        baseUrl: 'https://jaz.example',
        token: 'secret',
      }),
    ).toBeTrue()
    expect(seen).toEqual([])

    payload = completion('2026-07-18T10:01:00Z')
    expect(
      await monitor.configure({
        enabled: true,
        baseUrl: 'https://jaz.example',
        token: 'secret',
      }),
    ).toBeTrue()
    await monitor.poll()
    expect(seen).toHaveLength(1)
    await monitor.poll()
    expect(seen).toHaveLength(1)
    expect(requests).toEqual([
      {
        url: 'https://jaz.example/v1/feed/completions',
        authorization: 'Bearer secret',
      },
      {
        url: 'https://jaz.example/v1/feed/completions',
        authorization: 'Bearer secret',
      },
      {
        url: 'https://jaz.example/v1/feed/completions',
        authorization: 'Bearer secret',
      },
    ])
    monitor.stop()
  })

  test('stops polling when disabled and rejects malformed configuration', async () => {
    let requests = 0
    const monitor = createThreadCompletionMonitor(
      () => {},
      () => {
        requests += 1
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ items: [] }) })
      },
    )

    expect(
      await monitor.configure({ enabled: true, baseUrl: 'http://localhost:5299', token: '' }),
    ).toBeTrue()
    expect(await monitor.configure({ enabled: false })).toBeTrue()
    await monitor.poll()
    expect(requests).toBe(1)
    expect(
      await monitor.configure({ enabled: true, baseUrl: 'http://localhost:5299', token: '' }),
    ).toBeTrue()
    expect(
      await monitor.configure({ enabled: true, baseUrl: 'file:///tmp', token: '' }),
    ).toBeFalse()
    await monitor.poll()
    expect(requests).toBe(2)
  })

  test('keeps known completions when credentials rotate for the same backend', async () => {
    let payload = completion('2026-07-18T10:00:00Z')
    const authorizations = []
    const seen = []
    const monitor = createThreadCompletionMonitor(
      (item) => seen.push(item),
      (_url, init) => {
        authorizations.push(init?.headers?.Authorization)
        return Promise.resolve({ ok: true, json: () => Promise.resolve(payload) })
      },
    )

    await monitor.configure({ enabled: true, baseUrl: 'https://jaz.example', token: 'old' })
    payload = completion('2026-07-18T10:01:00Z')
    await monitor.configure({ enabled: true, baseUrl: 'https://jaz.example', token: 'new' })

    expect(seen).toHaveLength(1)
    expect(authorizations).toEqual(['Bearer old', 'Bearer new'])
    monitor.stop()
  })
})
