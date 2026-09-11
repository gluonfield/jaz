import { expect, test } from 'bun:test'
import { setTimeout as delay } from 'node:timers/promises'
import { BrowserRetention } from './browserRetention'

test('retains visible pages and releases an eligible idle page', async () => {
  let released = 0
  const retention = new BrowserRetention(async () => true, () => released += 1, 10)
  retention.configure(false)
  await delay(30)
  expect(released).toBe(0)
  retention.configure(true)
  await delay(30)
  expect(released).toBe(1)
  await delay(30)
  expect(released).toBe(1)
})

test('a pending command remains protected beyond the idle deadline', async () => {
  let released = 0
  let finish
  const retention = new BrowserRetention(async () => true, () => released += 1, 10)
  retention.configure(true)
  const action = retention.run(() => new Promise((resolve) => {
    finish = resolve
  }))
  await delay(30)
  expect(released).toBe(0)
  finish()
  await action
  await delay(30)
  expect(released).toBe(1)
})

test('new work invalidates an in-flight idle check', async () => {
  let released = 0
  let idleReply
  let idleSignal
  let checks = 0
  const retention = new BrowserRetention((signal) => {
    checks += 1
    if (checks > 1) return Promise.resolve(true)
    idleSignal = signal
    return new Promise((resolve) => {
      idleReply = resolve
    })
  }, () => released += 1, 10)
  retention.configure(true)
  await delay(30)
  let finish
  const action = retention.run(() => new Promise((resolve) => {
    finish = resolve
  }))
  expect(idleSignal.aborted).toBe(true)
  idleReply(true)
  await delay(30)
  expect(released).toBe(0)
  finish()
  await action
  await delay(30)
  expect(released).toBe(1)
})

test('visibility or cleanup invalidates a late idle reply', async () => {
  let released = 0
  let reply
  let idleSignal
  const retention = new BrowserRetention((signal) => new Promise((resolve) => {
    idleSignal = signal
    reply = resolve
  }), () => released += 1, 10)
  retention.configure(true)
  await delay(30)
  retention.configure(false)
  expect(idleSignal.aborted).toBe(true)
  reply(true)
  await delay(30)
  expect(released).toBe(0)
})

test('running or unavailable agent state retains the page and retries', async () => {
  let released = 0
  let checks = 0
  let complete
  const completed = new Promise((resolve) => {
    complete = resolve
  })
  const retention = new BrowserRetention(async () => {
    checks += 1
    if (checks === 1) throw new Error('Backend unavailable')
    return checks > 2
  }, () => {
    released += 1
    complete()
  }, 10)
  retention.configure(true)
  await completed
  expect(checks).toBe(3)
  expect(released).toBe(1)
})
