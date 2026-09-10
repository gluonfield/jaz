import { expect, test } from 'bun:test'
import { spawnSync } from 'node:child_process'
import process from 'node:process'
import { clearTimeout, setTimeout } from 'node:timers'
import { fileURLToPath, URL } from 'node:url'
import { TextEncoder } from 'node:util'
import { BrowserRepl } from './browserRepl'

test('browser scripts retain variables, await actions, and emit documentation once', async () => {
  const actions = []
  const repl = new BrowserRepl(async (input) => {
    actions.push(input)
    return { status: 'ok', text: 'Button Save ref=p1:e1', data: { elements: [{ ref: 'p1:e1' }] } }
  })
  try {
    const first = await repl.run('await tab.goto("https://example.com")\nconst observation = await tab.getState()\nconst ref = observation.elements[0].ref')
    expect(first.text).toContain('# Jaz browser JavaScript')
    const next = await repl.run('await tab.click(ref)\nnodeRepl.write(observation.elements.length)\nawait tab.getState()')
    expect(next.text).not.toContain('# Jaz browser JavaScript')
    expect(next.text).toContain('1')
    expect(actions.map((action) => action.action)).toEqual(['navigate', 'state', 'click', 'state'])
    expect(actions[2].ref).toBe('p1:e1')
  } finally {
    repl.cancel()
  }
})

test('browser scripts isolate host capabilities and retain destructured values', async () => {
  const repl = new BrowserRepl(async () => ({ status: 'ok' }))
  try {
    await repl.run('const {a, nested: [b]} = {a: 3, nested: [7]}')
    const result = await repl.run('nodeRepl.write([a, b, typeof process, typeof require, typeof fetch, typeof window])')
    expect(result.text).toContain('[3,7,"undefined","undefined","undefined","undefined"]')
  } finally {
    repl.cancel()
  }
})

test('persistent scripts preserve JavaScript scope, hoisting, and constant bindings', async () => {
  const repl = new BrowserRepl(async () => ({ status: 'ok' }))
  try {
    await repl.run(`const answer = await doubled(21)
async function doubled(value) {
  return value * 2
}
let count = 1
if (true) {
  var shared = answer
  let local = 'block only'
}`)
    expect((await repl.run('count += 1\nnodeRepl.write([answer, count, shared, typeof local])')).text).toContain('[42,2,42,"undefined"]')
    await expect(repl.run('answer = 0')).rejects.toThrow()
    await expect(repl.run('const answer = 0')).rejects.toThrow()
    expect((await repl.run('nodeRepl.write(answer)')).text).toContain('42')
  } finally {
    repl.cancel()
  }
})

test('one persistent browser script surface exposes animated methods and direct CDP', async () => {
  const actions = []
  const repl = new BrowserRepl(async (input) => {
    actions.push(input)
    return { status: 'ok', data: { result: { value: 'page response' } } }
  })
  try {
    await repl.run('const cdp = tab.cdp\nawait tab.scroll("down", 0, "p1:e1")')
    const result = await repl.run(`const response = await cdp.send('Runtime.evaluate', {expression:'document.title',returnByValue:true})
nodeRepl.write(response.result.value)
await tab.scroll('down')`)
    expect(result.text).toContain('page response')
    expect(actions).toEqual([
      { action: 'scroll', text: 'down', amount: 0, ref: 'p1:e1' },
      { action: 'cdp', method: 'Runtime.evaluate', params: { expression: 'document.title', returnByValue: true } },
      { action: 'scroll', text: 'down', amount: 800 },
    ])
  } finally {
    repl.cancel()
  }
})

test('browser scripts propagate an action failure instead of continuing', async () => {
  const repl = new BrowserRepl(async () => {
    throw new Error('stale ref')
  })
  try {
    await expect(repl.run('await tab.click("p1:e1")\nnodeRepl.write("done")')).rejects.toThrow('stale ref')
  } finally {
    repl.cancel()
  }
})

test('the interpreter rejects overlapping scripts without interrupting its active call', async () => {
  const started = Promise.withResolvers()
  const action = Promise.withResolvers()
  const repl = new BrowserRepl(() => {
    started.resolve()
    return action.promise
  })
  try {
    const first = repl.run('await tab.getState()\nconst finished = true')
    await started.promise
    await expect(repl.run('nodeRepl.write("second")')).rejects.toThrow('Another browser script is running')
    action.resolve({ status: 'ok', text: 'page is ready' })
    expect((await first).text).toContain('page is ready')
    expect((await repl.run('nodeRepl.write(finished)')).text).toContain('true')
  } finally {
    repl.cancel()
  }
})

test('cancellation aborts a pending browser action and resets the interpreter', async () => {
  const repl = new BrowserRepl((_input, signal) => new Promise((_resolve, reject) => {
    signal.addEventListener('abort', () => reject(signal.reason), { once: true })
  }), 50)
  await expect(repl.run('await tab.getState()')).rejects.toThrow('cancelled')
  const result = await repl.run('nodeRepl.write(typeof missing)')
  expect(result.text).toContain('# Jaz browser JavaScript')
  expect(result.text).toContain('undefined')
  repl.cancel()
})

test('an unresolved JavaScript promise times out and can be followed by another script', async () => {
  const repl = new BrowserRepl(async () => ({ status: 'ok' }), 50)
  await expect(repl.run('await new Promise(() => {})')).rejects.toThrow('time limit')
  expect((await repl.run('nodeRepl.write(42)')).text).toContain('42')
  repl.cancel()
})

test('a failed script cancels unfinished actions before returning its original error', async () => {
  let signal
  let watchdogFired = false
  const repl = new BrowserRepl((_input, actionSignal) => {
    signal = actionSignal
    return new Promise((_resolve, reject) => {
      actionSignal.addEventListener('abort', () => reject(actionSignal.reason), { once: true })
    })
  })
  const watchdog = setTimeout(() => {
    watchdogFired = true
    repl.cancel()
  }, 200)
  try {
    await expect(repl.run('void tab.getState()\nthrow new Error("stop this script")')).rejects.toThrow('stop this script')
    expect(watchdogFired).toBe(false)
    expect(signal.aborted).toBe(true)
    expect((await repl.run('nodeRepl.write("recovered")')).text).toContain('recovered')
  } finally {
    clearTimeout(watchdog)
    repl.cancel()
  }
})

test('cancellation does not wait for an unresponsive host operation or leak its late output', async () => {
  const started = Promise.withResolvers()
  const action = Promise.withResolvers()
  let watchdogFired = false
  const repl = new BrowserRepl(() => {
    started.resolve()
    return action.promise
  })
  const running = repl.run('await tab.getState()')
  await started.promise
  repl.cancel()
  const watchdog = setTimeout(() => {
    watchdogFired = true
    action.resolve({ status: 'ok', text: 'late page result' })
  }, 200)
  try {
    await expect(running).rejects.toThrow('cancelled')
    expect(watchdogFired).toBe(false)
    const next = await repl.run('nodeRepl.write("next script")')
    action.resolve({ status: 'ok', text: 'late page result' })
    await action.promise
    expect(next.text).toContain('next script')
    expect(next.text).not.toContain('late page result')
  } finally {
    clearTimeout(watchdog)
    action.resolve({ status: 'ok' })
    repl.cancel()
  }
})

test('bounded script output keeps first-use documentation and the latest UTF-8 observation', async () => {
  const repl = new BrowserRepl(async () => ({ status: 'ok', text: '🌏'.repeat(15000) }))
  try {
    const result = await repl.run('await tab.getState()\nnodeRepl.write("FINAL STATE VERIFIED")')
    expect(result.text).toStartWith('# Jaz browser JavaScript')
    expect(result.text).toEndWith('FINAL STATE VERIFIED')
    expect(result.text).toContain('truncated')
    expect(result.text).not.toContain('�')
    expect(new TextEncoder().encode(result.text).length).toBeLessThanOrEqual(12000)
  } finally {
    repl.cancel()
  }
})

test('promise loops allow host cancellation, respect CPU deadlines, and recover cleanly', () => {
  const script = `
import { BrowserRepl } from ${JSON.stringify(fileURLToPath(new URL('./browserRepl.ts', import.meta.url)))}
const repl = new BrowserRepl(async () => {
  throw new Error('unsupported action')
}, 100)
await repl.run('')
let responsive = false
const cancellation = setTimeout(() => {
  responsive = true
  repl.cancel()
}, 10)
try {
  await repl.run('while (true) { try { await tab.getState() } catch {} }')
  throw new Error('An endless script completed successfully')
} catch (error) {
  if (!/interrupted|time limit|cancelled/.test(error.message)) {
    throw error
  }
}
clearTimeout(cancellation)
if (!responsive) {
  throw new Error('The script starved host cancellation')
}
try {
  await repl.run('while (true) { await Promise.resolve() }')
  throw new Error('An endless guest promise loop completed successfully')
} catch (error) {
  if (!/interrupted|time limit|cancelled/.test(error.message)) {
    throw error
  }
}
const result = await repl.run('nodeRepl.write("recovered")')
if (!result.text.includes('recovered')) {
  throw new Error('The interpreter did not recover')
}
repl.cancel()
`
  const result = spawnSync(process.execPath, ['run', '-'], { input: script, encoding: 'utf8', timeout: 2000 })
  expect({ status: result.status, error: result.error?.message, stderr: result.stderr }).toEqual({ status: 0, error: undefined, stderr: '' })
})
