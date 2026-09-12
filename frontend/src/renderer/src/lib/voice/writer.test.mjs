import { expect, test } from 'bun:test'
import { setTimeout as delay } from 'node:timers/promises'
import { TextEncoder } from 'node:util'
import { VoiceTranscriptWriter } from './writer'

const message = (text, id = 'utterance', call = 'call') => ({ id, call_id: call, role: 'user', text, at: new Date().toISOString() })

test('a failed save retains speech and retries without needing another spoken turn', async () => {
  const batches = []
  const changes = []
  const writer = new VoiceTranscriptWriter(async (batch) => {
    batches.push(batch)
    if (batches.length === 1) {
      throw new Error('offline')
    }
  }, () => changes.push(writer.error))
  const spoken = message('Keep this on close')
  writer.append(spoken)
  await writer.flush()
  expect(writer.error).toContain('offline')
  await delay(1100)
  expect(batches).toEqual([[spoken], [spoken]])
  expect(changes.at(-1)).toBe('')
})

test('an acknowledged partial cannot erase a final transcript arriving during the save', async () => {
  const saving = Promise.withResolvers()
  const batches = []
  const writer = new VoiceTranscriptWriter(async (batch) => {
    batches.push(batch)
    if (batches.length === 1) {
      await saving.promise
    }
  }, () => {})
  const partial = message('Check')
  const final = { ...partial, text: 'Check the current directory.' }
  writer.append(partial)
  const flushed = writer.flush()
  writer.append(final, true)
  saving.resolve()
  await flushed
  expect(batches).toEqual([[partial], [final]])
})

test('reconnect and close preserve both calls and respect the transcript batch limit', async () => {
  const saving = Promise.withResolvers()
  const batches = []
  const writer = new VoiceTranscriptWriter(async (batch) => {
    batches.push(batch)
    if (batches.length === 1) {
      await saving.promise
    }
  }, () => {})
  const old = message('Old call')
  writer.append(old)
  const flushed = writer.flush()
  const next = Array.from({ length: 35 }, (_, index) => message('New call', `${index}`, 'next'))
  for (const entry of next) {
    writer.append(entry)
  }
  void writer.flush()
  saving.resolve()
  await flushed
  expect(batches.map((batch) => batch.length)).toEqual([1, 32, 3])
  expect(batches.flat()).toEqual([old, ...next])
})


test('a final utterance arriving immediately after an empty flush is still saved', async () => {
  const batches = []
  const writer = new VoiceTranscriptWriter(async (batch) => {
    batches.push(batch)
  }, () => {})
  const closing = writer.flush()
  const final = message('Late final turn')
  writer.append(final, true)
  await closing
  await delay(0)
  expect(batches).toEqual([[final]])
})

test('buffered Unicode speech fits the HTTP byte limit after a long disconnection', async () => {
  const batches = []
  const writer = new VoiceTranscriptWriter(async (batch) => {
    expect(new TextEncoder().encode(JSON.stringify(batch)).length).toBeLessThanOrEqual(256 * 1024)
    batches.push(batch)
  }, () => {})
  const entries = Array.from({ length: 5 }, (_, index) => message('🗣'.repeat(14000), `${index}`))
  for (const entry of entries) {
    writer.append(entry)
  }
  await writer.flush()
  expect(batches.map((batch) => batch.length)).toEqual([4, 1])
  expect(batches.flat()).toEqual(entries)
})
