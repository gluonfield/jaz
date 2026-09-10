import { expect, test } from 'bun:test'
import { startDictationProcess } from './dictationProcess'

function run(source) {
  const events = []
  const closed = Promise.withResolvers()
  const process = startDictationProcess(globalThis.process.execPath, ['-e', source], (event) => events.push(event), closed.resolve)
  return { process, events, closed: closed.promise }
}

test('native stream handles split UTF-8 and emits one complete transcript', async () => {
  const runState = run([
    'const data = Buffer.from(JSON.stringify({ type: "complete", text: "Labas, pasauli 🌍" }) + "\\n")',
    'process.stdout.write(data.subarray(0, data.length - 5))',
    'setTimeout(() => process.stdout.write(data.subarray(data.length - 5)), 10)',
    'setInterval(() => {}, 1000)',
  ].join('\n'))
  await runState.closed
  expect(runState.events).toEqual([{ type: 'complete', text: 'Labas, pasauli 🌍' }])
})

test('stop finishes the same native session and retains the final result', async () => {
  const events = []
  const closed = Promise.withResolvers()
  const source = [
    'process.stdin.on("data", data => {',
    '  process.stdout.write(JSON.stringify({ type: "complete", text: data.toString().trim() }) + "\\n")',
    '})',
    'process.stdout.write(JSON.stringify({ type: "status", phase: "recording" }) + "\\n")',
  ].join('\n')
  const native = startDictationProcess(process.execPath, ['-e', source], (event) => {
    events.push(event)
    if (event.type === 'status') {
      native.stop()
      native.stop()
    }
  }, closed.resolve)
  await closed.promise
  expect(events).toEqual([
    { type: 'status', phase: 'recording' },
    { type: 'complete', text: 'stop' },
  ])
})

test('cancel closes once and never commits a result', async () => {
  const events = []
  let closed = 0
  const native = startDictationProcess(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], (event) => events.push(event), () => {
    closed += 1
  })
  native.cancel()
  native.cancel()
  native.stop()
  expect(closed).toBe(1)
  expect(events).toEqual([])
})

test('invalid native output and premature exit produce actionable errors', async () => {
  for (const source of [
    'process.stdout.write("invalid\\n")',
    'process.stdout.write(JSON.stringify({ type: "result", text: 42 }) + "\\n")',
    'process.exit(1)',
  ]) {
    const runState = run(source)
    await runState.closed
    expect(runState.events).toHaveLength(1)
    expect(runState.events[0].type).toBe('error')
    expect(runState.events[0].message.length).toBeGreaterThan(10)
  }
})

test('a native error is terminal even if a late completion follows it', async () => {
  const runState = run([
    'process.stdout.write(JSON.stringify({ type: "error", message: "Microphone denied" }) + "\\n")',
    'process.stdout.write(JSON.stringify({ type: "complete", text: "discard me" }) + "\\n")',
  ].join('\n'))
  await runState.closed
  expect(runState.events).toEqual([{ type: 'error', message: 'Microphone denied' }])
})
