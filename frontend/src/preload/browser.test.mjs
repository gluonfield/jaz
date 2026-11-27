import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'

const result = await build({
  entrypoints: [new URL('./browser.ts', import.meta.url).pathname],
  target: 'node',
  format: 'cjs',
  external: ['electron'],
})
if (!result.success) {
  throw new AggregateError(result.logs)
}
const evaluate = new Function('require', 'module', 'exports', await result.outputs[0].text())
const require = createRequire(import.meta.url)

// Minimal DOM + ShadowRoot harness exercising real DOM/Shadow traversal & events in bun:test.
class FakeEvent {
  constructor(type, init = {}) {
    this.type = type
    this.bubbles = Boolean(init.bubbles)
    this.composed = Boolean(init.composed)
    this.key = init.key ?? ''
    this._path = init.path ?? []
    this.target = init.target ?? this._path[0] ?? null
  }
  composedPath() {
    return this._path.length ? this._path : this.target ? [this.target] : []
  }
}

class FakeElement {
  constructor(tagName, attrs = {}) {
    this.tagName = tagName.toUpperCase()
    this.attributes = new Map(Object.entries(attrs).map(([k, v]) => [k.toLowerCase(), String(v)]))
    this.children = []
    this.parentNode = null
    this.shadowRoot = null
    this.ownerDocument = null
    this.textContent = attrs.textContent ?? ''
    this.visible = attrs.visible ?? true
  }
  getAttribute(name) {
    return this.attributes.get(name.toLowerCase()) ?? null
  }
  setAttribute(name, value) {
    this.attributes.set(name.toLowerCase(), String(value))
  }
  appendChild(child) {
    child.parentNode = this
    child.ownerDocument = this.ownerDocument
    this.children.push(child)
    return child
  }
  removeChild(child) {
    const index = this.children.indexOf(child)
    if (index >= 0) {
      this.children.splice(index, 1)
      child.parentNode = null
    }
  }
  attachShadow() {
    const root = new FakeRoot(this.ownerDocument)
    this.shadowRoot = root
    return root
  }
  getClientRects() {
    return this.visible ? [{ width: 100, height: 24 }] : []
  }
  matches(selector) {
    return selector.split(',').some((part) => {
      const m = part.trim().match(/^\[autocomplete~="([^"]+)"\s*i?\]$/i)
      if (!m) return false
      const tokens = (this.getAttribute('autocomplete') ?? '').toLowerCase().split(/\s+/)
      return tokens.includes(m[1].toLowerCase())
    })
  }
  querySelectorAll(selector) {
    const out = []
    const walk = (node) => {
      for (const child of node.children) {
        if (selector === '*' || child.tagName.toLowerCase() === selector.toLowerCase()) {
          out.push(child)
        }
        walk(child)
      }
    }
    walk(this)
    return out
  }
  dispatchEvent(event) {
    event.target = event.target ?? this
    const doc = this.ownerDocument
    if (doc) {
      doc.dispatchEvent(event)
    }
  }
}

class FakeInputElement extends FakeElement {
  constructor(attrs = {}) {
    super('input', attrs)
    this.type = attrs.type ?? 'text'
    this.name = attrs.name ?? ''
    this.id = attrs.id ?? ''
    this.placeholder = attrs.placeholder ?? ''
    this.value = attrs.value ?? ''
    this.disabled = Boolean(attrs.disabled)
    this.readOnly = Boolean(attrs.readOnly)
    this.form = attrs.form ?? null
    this.focused = false
  }
  focus() {
    this.focused = true
  }
}

class FakeButtonElement extends FakeElement {
  constructor(attrs = {}) {
    super('button', attrs)
    this.type = attrs.type ?? 'submit'
    this.form = attrs.form ?? null
  }
}

class FakeFormElement extends FakeElement {
  constructor(attrs = {}) {
    super('form', attrs)
  }
  get elements() {
    return this.querySelectorAll('input')
  }
}

class FakeRoot {
  constructor(ownerDocument) {
    this.children = []
    this.ownerDocument = ownerDocument
    this.listeners = new Map()
  }
  appendChild(child) {
    child.ownerDocument = this.ownerDocument ?? this
    child.parentNode = this
    this.children.push(child)
    return child
  }
  removeChild(child) {
    const index = this.children.indexOf(child)
    if (index >= 0) {
      this.children.splice(index, 1)
      child.parentNode = null
    }
  }
  querySelectorAll(selector) {
    const out = []
    const walk = (node) => {
      for (const child of node.children) {
        if (selector === '*' || child.tagName.toLowerCase() === selector.toLowerCase()) {
          out.push(child)
        }
        walk(child)
      }
    }
    walk(this)
    return out
  }
  addEventListener(type, fn) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set())
    this.listeners.get(type).add(fn)
  }
  removeEventListener(type, fn) {
    this.listeners.get(type)?.delete(fn)
  }
  dispatchEvent(event) {
    for (const fn of this.listeners.get(event.type) ?? []) {
      fn(event)
    }
  }
}

const savedGlobals = {
  Element: globalThis.Element,
  HTMLInputElement: globalThis.HTMLInputElement,
  HTMLButtonElement: globalThis.HTMLButtonElement,
  HTMLFormElement: globalThis.HTMLFormElement,
  Event: globalThis.Event,
}

afterEach(() => {
  for (const [key, value] of Object.entries(savedGlobals)) {
    if (value === undefined) {
      delete globalThis[key]
    } else {
      globalThis[key] = value
    }
  }
})

function loadPreload() {
  globalThis.Element = FakeElement
  globalThis.HTMLInputElement = FakeInputElement
  globalThis.HTMLButtonElement = FakeButtonElement
  globalThis.HTMLFormElement = FakeFormElement
  globalThis.Event = FakeEvent
  const module = { exports: {} }
  evaluate((name) => name === 'electron' ? { ipcRenderer: { send: () => {}, on: () => {} } } : require(name), module, module.exports)
  return module.exports
}

function createHarness(href) {
  const { attachBrowserPasswordCapture } = loadPreload()
  const winListeners = new Map()
  const win = {
    location: new URL(href),
    HTMLInputElement: FakeInputElement,
    getComputedStyle: () => ({ display: 'block', visibility: 'visible' }),
    addEventListener(type, fn) {
      if (!winListeners.has(type)) winListeners.set(type, new Set())
      winListeners.get(type).add(fn)
    },
    removeEventListener(type, fn) {
      winListeners.get(type)?.delete(fn)
    },
    dispatchEvent(event) {
      for (const fn of winListeners.get(event.type) ?? []) fn(event)
    },
  }
  const doc = new FakeRoot(null)
  doc.ownerDocument = doc
  doc.defaultView = win
  const sent = []
  const ipcListeners = new Map()
  const bridge = {
    send: (channel, payload) => sent.push({ channel, payload }),
    on: (channel, listener) => ipcListeners.set(channel, listener),
  }
  const detach = attachBrowserPasswordCapture(win, doc, bridge)
  return {
    win,
    doc,
    sent,
    detach,
    triggerIPC: (channel, payload) => ipcListeners.get(channel)?.({}, payload),
  }
}

test('SPA button click and Enter key capture credentials without native form submit, ignoring show-password toggles', () => {
  const h = createHarness('https://spa.example.test/login')
  const email = h.doc.appendChild(new FakeInputElement({ type: 'email', name: 'email' }))
  const password = h.doc.appendChild(new FakeInputElement({ type: 'password', name: 'password' }))
  const reveal = h.doc.appendChild(new FakeButtonElement({ type: 'button', 'aria-label': 'Show password' }))
  const loginBtn = h.doc.appendChild(new FakeButtonElement({ type: 'button', textContent: 'Sign in' }))

  email.value = 'spa@example.test'
  h.doc.dispatchEvent(new FakeEvent('input', { path: [email] }))
  password.value = 'spa-secret'
  h.doc.dispatchEvent(new FakeEvent('input', { path: [password] }))

  // Clicking "Show password" must not trigger a premature capture.
  h.doc.dispatchEvent(new FakeEvent('click', { path: [reveal] }))
  expect(h.sent.filter((m) => m.channel === 'jaz:browser-passwords:capture')).toEqual([])

  // Clicking SPA Sign in button captures credentials.
  h.doc.dispatchEvent(new FakeEvent('click', { path: [loginBtn] }))
  expect(h.sent.filter((m) => m.channel === 'jaz:browser-passwords:capture').at(-1)?.payload).toEqual({
    origin: 'https://spa.example.test',
    username: 'spa@example.test',
    password: 'spa-secret',
  })

  // Pressing Enter in the password input also captures credentials.
  h.sent.length = 0
  password.value = 'spa-secret-2'
  h.doc.dispatchEvent(new FakeEvent('keydown', { key: 'Enter', path: [password] }))
  expect(h.sent.filter((m) => m.channel === 'jaz:browser-passwords:capture').at(-1)?.payload).toEqual({
    origin: 'https://spa.example.test',
    username: 'spa@example.test',
    password: 'spa-secret-2',
  })
})

test('Shadow DOM login inputs are discovered for capture and autofill', () => {
  const h = createHarness('https://shadow.example.test/signin')
  const host = h.doc.appendChild(new FakeElement('auth-card'))
  const shadow = host.attachShadow()
  const username = shadow.appendChild(new FakeInputElement({ type: 'text', autocomplete: 'username' }))
  const password = shadow.appendChild(new FakeInputElement({ type: 'password', autocomplete: 'current-password' }))
  const submit = shadow.appendChild(new FakeButtonElement({ type: 'button', textContent: 'Continue' }))

  username.value = 'shadow@example.test'
  h.doc.dispatchEvent(new FakeEvent('input', { path: [username, host], target: host }))
  password.value = 'shadow-pass'
  h.doc.dispatchEvent(new FakeEvent('input', { path: [password, host], target: host }))
  h.doc.dispatchEvent(new FakeEvent('click', { path: [submit, host], target: host }))

  expect(h.sent.filter((m) => m.channel === 'jaz:browser-passwords:capture').at(-1)?.payload).toEqual({
    origin: 'https://shadow.example.test',
    username: 'shadow@example.test',
    password: 'shadow-pass',
  })

  username.value = ''
  password.value = ''
  h.sent.length = 0
  h.triggerIPC('jaz:browser-passwords:fill', {
    origin: 'https://shadow.example.test',
    username: 'filled@example.test',
    password: 'filled-password',
    requestId: 'req-1',
  })
  expect(username.value).toBe('filled@example.test')
  expect(password.value).toBe('filled-password')
  expect(password.focused).toBe(true)
  expect(h.sent.at(-1)).toEqual({
    channel: 'jaz:browser-passwords:result',
    payload: { requestId: 'req-1', found: true },
  })
})

test('two-step login retains step 1 username after email field unmounts and flushes draft on pagehide', () => {
  const h = createHarness('http://localhost:5173/auth')
  const email = h.doc.appendChild(new FakeInputElement({ type: 'email', name: 'identifier' }))
  email.value = 'twostep@localhost.test'
  h.doc.dispatchEvent(new FakeEvent('input', { path: [email] }))
  // Step 1 finishes and removes the email input before mounting step 2 password input.
  h.doc.removeChild(email)

  const password = h.doc.appendChild(new FakeInputElement({ type: 'password', name: 'passwd' }))
  password.value = 'step-two-secret'
  h.doc.dispatchEvent(new FakeEvent('input', { path: [password] }))
  // SPA clears/unmounts form and navigates away -> pagehide flushes draft with step-1 username!
  h.doc.removeChild(password)
  h.win.dispatchEvent(new FakeEvent('pagehide'))

  expect(h.sent.filter((m) => m.channel === 'jaz:browser-passwords:capture').at(-1)?.payload).toEqual({
    origin: 'http://localhost:5173',
    username: 'twostep@localhost.test',
    password: 'step-two-secret',
  })
})

test('non-loopback HTTP pages do not install capture listeners', () => {
  const h = createHarness('http://example.test/login')
  const password = h.doc.appendChild(new FakeInputElement({ type: 'password' }))
  password.value = 'should-not-capture'
  h.doc.dispatchEvent(new FakeEvent('keydown', { key: 'Enter', path: [password] }))
  expect(h.sent).toEqual([])
})
