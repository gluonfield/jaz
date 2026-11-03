import 'fake-indexeddb/auto'
import { test, expect } from 'bun:test'
import { saveBrowserCredential, loadBrowserCredential, forgetBrowserCredential } from './browserCredentials'

test('parallel connections keep one encryption key', async () => {
  await Promise.all([
    saveBrowserCredential('https://one.example', 'one', 'one', () => true),
    saveBrowserCredential('https://two.example', 'two', 'two', () => true),
  ])
  expect((await loadBrowserCredential('https://one.example'))?.token).toBe('one')
  expect((await loadBrowserCredential('https://two.example'))?.token).toBe('two')
})

test('login restores, records are encrypted, key stays non-exportable', async () => {
  const url = 'https://restore.example'
  await saveBrowserCredential(url, 'test-token', 'device-1', () => true)
  expect(await loadBrowserCredential(url)).toEqual({ token: 'test-token', deviceId: 'device-1' })
  const db = await new Promise((resolve) => {
    const r = indexedDB.open('jaz.browserCredentials', 1)
    r.onsuccess = () => resolve(r.result)
  })
  const entries = await new Promise((resolve) => {
    const r = db.transaction('credentials').objectStore('credentials').getAll()
    r.onsuccess = () => resolve(r.result)
  })
  db.close()
  expect(entries.find((x) => x instanceof CryptoKey).extractable).toBe(false)
  const record = entries.find((x) => x.ciphertext)
  expect(new TextDecoder().decode(record.ciphertext)).not.toContain('test-token')
})

test('another server never sees a saved token', async () => {
  expect(await loadBrowserCredential('https://other.example')).toBeUndefined()
})

test('a cancelled connection leaves no saved login', async () => {
  await expect(saveBrowserCredential('https://cancel.example', 'token', 'device', () => false)).rejects.toThrow()
  expect(await loadBrowserCredential('https://cancel.example')).toBeUndefined()
})

test('forgetting a server drops its saved login', async () => {
  await forgetBrowserCredential('https://restore.example')
  expect(await loadBrowserCredential('https://restore.example')).toBeUndefined()
})
