/// <reference lib="dom" />
import { ipcRenderer } from 'electron'
import {
  PASSWORD_CAPTURE_CHANNEL,
  PASSWORD_FILL_CHANNEL,
  PASSWORD_RESULT_CHANNEL,
  PASSWORD_USERNAME_CHANNEL,
  passwordOrigin,
} from '@shared/browserPasswords'

const USERNAME_HINT = /user|email|mail|login|account|identifier|handle|phone/i
const NON_SUBMIT_BUTTON = /\b(show|hide|reveal|toggle|visibility|clear|forgot|reset|cancel|back|close|dismiss|cookie|privacy|terms)\b/i
const SKIP_USERNAME_HINT = /search|otp|code|token|captcha|coupon|promo|zip|postal/i

export type PasswordBridge = {
  send: (channel: string, payload: unknown) => void
  on: (channel: string, listener: (_event: unknown, payload: unknown) => void) => void
}

type CapturedDraft = {
  origin: string
  username: string
  password: string
}

type CaptureRequest = { requestId?: string } | undefined
type FillRequest = { origin: string; username: string; password: string; requestId?: string }

function collectDeepInputs(root: Document | ShadowRoot | Element, seen = new Set<HTMLInputElement>(), out: HTMLInputElement[] = []): HTMLInputElement[] {
  if (root instanceof HTMLFormElement) {
    for (const element of Array.from(root.elements)) {
      if (element instanceof HTMLInputElement && !seen.has(element)) {
        seen.add(element)
        out.push(element)
      }
    }
  }
  for (const element of root.querySelectorAll('input')) {
    if (element instanceof HTMLInputElement && !seen.has(element)) {
      seen.add(element)
      out.push(element)
    }
  }
  for (const host of root.querySelectorAll('*')) {
    if (host.shadowRoot) {
      collectDeepInputs(host.shadowRoot, seen, out)
    }
  }
  return out
}

export function collectShadowInputs(root: Document | ShadowRoot | Element): HTMLInputElement[] {
  return collectDeepInputs(root)
}

function isVisibleInput(input: HTMLInputElement): boolean {
  if (input.type === 'hidden') {
    return false
  }
  if (input.getClientRects().length === 0) {
    return false
  }
  const view = input.ownerDocument.defaultView
  if (view?.getComputedStyle) {
    const style = view.getComputedStyle(input)
    if (style.display === 'none' || style.visibility === 'hidden') {
      return false
    }
  }
  return true
}

function matchesSelector(input: HTMLInputElement, selector: string): boolean {
  try {
    return input.matches(selector)
  } catch {
    return false
  }
}

function inputHintText(input: HTMLInputElement): string {
  return [input.name, input.id, input.placeholder, input.getAttribute('aria-label') ?? ''].join(' ')
}

function isLikelyUsernameInput(input: HTMLInputElement): boolean {
  if (input.type === 'password') {
    return false
  }
  if (matchesSelector(input, '[autocomplete~="username" i], [autocomplete~="email" i]')) {
    return true
  }
  if (input.type === 'email') {
    return true
  }
  if (input.type !== 'text' && input.type !== 'tel') {
    return false
  }
  const hint = inputHintText(input)
  if (SKIP_USERNAME_HINT.test(hint)) {
    return false
  }
  return USERNAME_HINT.test(hint)
}

function pickUsername(candidates: HTMLInputElement[], allInputs: HTMLInputElement[], password: HTMLInputElement): HTMLInputElement | undefined {
  const passwordIndex = allInputs.indexOf(password)
  const beforePassword = (input: HTMLInputElement) => {
    const index = allInputs.indexOf(input)
    return index !== -1 && (passwordIndex === -1 || index < passwordIndex)
  }
  return candidates.find((input) => matchesSelector(input, '[autocomplete~="username" i]'))
    ?? candidates.find((input) => matchesSelector(input, '[autocomplete~="email" i]') || input.type === 'email')
    ?? candidates.filter((input) => (input.type === 'text' || input.type === 'tel') && beforePassword(input) && USERNAME_HINT.test(inputHintText(input)) && !SKIP_USERNAME_HINT.test(inputHintText(input))).at(-1)
    ?? candidates.filter((input) => (input.type === 'text' || input.type === 'tel') && beforePassword(input) && !SKIP_USERNAME_HINT.test(inputHintText(input))).at(-1)
}

export function findLoginFields(root: Document | HTMLFormElement, operation: 'capture' | 'fill'): {
  username?: HTMLInputElement
  password?: HTMLInputElement
} {
  const inputs = collectShadowInputs(root)
  const passwords = inputs.filter((input) => input.type === 'password' && !input.disabled && isVisibleInput(input) &&
    (operation === 'capture' ? Boolean(input.value) : !input.readOnly && !matchesSelector(input, '[autocomplete~="new-password" i]')))
  const password = passwords.find((input) => matchesSelector(input, operation === 'capture'
    ? '[autocomplete~="new-password" i]' : '[autocomplete~="current-password" i]')) ?? passwords[0]
  if (!password) {
    return {}
  }
  const sameForm = password.form ? inputs.filter((input) => input.form === password.form) : []
  const username = (sameForm.length ? pickUsername(sameForm, inputs, password) : undefined)
    ?? pickUsername(inputs, inputs, password)
  return { username, password }
}

function findStandaloneUsername(doc: Document): HTMLInputElement | undefined {
  const inputs = collectShadowInputs(doc).filter((input) => !input.disabled && isVisibleInput(input) && Boolean(input.value.trim()))
  return inputs.find((input) => isLikelyUsernameInput(input))
}

function setValue(input: HTMLInputElement, value: string): void {
  const proto = input.ownerDocument.defaultView?.HTMLInputElement?.prototype ?? HTMLInputElement.prototype
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set
  if (setter) {
    setter.call(input, value)
  } else {
    input.value = value
  }
  input.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
  input.dispatchEvent(new Event('change', { bubbles: true, composed: true }))
}

function findActionControl(path: ReadonlyArray<EventTarget | null>): HTMLElement | null {
  for (const node of path) {
    if (!(node instanceof Element)) {
      continue
    }
    const tag = node.tagName.toLowerCase()
    if (tag === 'button') {
      return node as HTMLElement
    }
    if (tag === 'input') {
      const type = (node.getAttribute('type') || '').toLowerCase()
      if (type === 'submit' || type === 'button' || type === 'image') {
        return node as HTMLElement
      }
    }
    if (node.getAttribute('role') === 'button') {
      return node as HTMLElement
    }
  }
  return null
}

function isNonSubmitControl(control: HTMLElement): boolean {
  if (control instanceof HTMLButtonElement && control.type === 'reset') {
    return true
  }
  if (control instanceof HTMLButtonElement && control.type === 'submit') {
    return false
  }
  if (control instanceof HTMLInputElement && control.type === 'submit') {
    return false
  }
  const label = [
    control.getAttribute('aria-label') ?? '',
    control.getAttribute('title') ?? '',
    control.textContent ?? '',
  ].join(' ').trim()
  return NON_SUBMIT_BUTTON.test(label)
}

export function attachBrowserPasswordCapture(win: Window, doc: Document, bridge: PasswordBridge): () => void {
  const origin = passwordOrigin(win.location.href)
  if (!origin) {
    return () => {}
  }

  let lastUsername = ''
  let draft: CapturedDraft | null = null

  const rememberUsername = (raw: string) => {
    const trimmed = raw.trim()
    if (!trimmed || trimmed.length > 1024 || trimmed === lastUsername) {
      return
    }
    lastUsername = trimmed
    bridge.send(PASSWORD_USERNAME_CHANNEL, { origin, username: trimmed })
  }

  const syncDraftFromEvent = (event: Event) => {
    const path = typeof event.composedPath === 'function' ? event.composedPath() : [event.target]
    const target = path[0]
    if (!(target instanceof HTMLInputElement) || target.disabled) {
      return
    }
    if (target.type === 'password') {
      if (target.value) {
        const fields = findLoginFields(target.form ?? doc, 'capture')
        const fieldUsername = fields.username?.value.trim() ?? ''
        if (fieldUsername) {
          rememberUsername(fieldUsername)
        }
        draft = { origin, username: fieldUsername || lastUsername, password: target.value }
      } else if (draft?.origin === origin) {
        draft = null
      }
      return
    }
    if (isLikelyUsernameInput(target) || (target.form && collectShadowInputs(target.form).some((input) => input.type === 'password'))) {
      rememberUsername(target.value)
      if (draft) {
        draft = { ...draft, username: target.value.trim() || lastUsername }
      }
    }
  }

  const emitCapture = (root: Document | HTMLFormElement, options: { manual?: boolean; requestId?: string; allowDraft?: boolean } = {}) => {
    const { username, password } = findLoginFields(root, 'capture')
    if (password?.value) {
      const resolvedUsername = username?.value.trim() || lastUsername
      if (resolvedUsername) {
        rememberUsername(resolvedUsername)
      }
      draft = { origin, username: resolvedUsername, password: password.value }
      bridge.send(PASSWORD_CAPTURE_CHANNEL, {
        origin,
        username: resolvedUsername,
        password: password.value,
        ...(options.requestId ? { requestId: options.requestId } : {}),
      })
      if (options.manual) {
        bridge.send(PASSWORD_RESULT_CHANNEL, { requestId: options.requestId, found: true })
      }
      return
    }
    if (options.allowDraft && draft?.password) {
      bridge.send(PASSWORD_CAPTURE_CHANNEL, {
        origin: draft.origin,
        username: draft.username || lastUsername,
        password: draft.password,
      })
      return
    }
    if (options.manual) {
      bridge.send(PASSWORD_RESULT_CHANNEL, { requestId: options.requestId, found: false })
    }
  }

  const onSubmit = (event: Event) => {
    const target = event.target
    if (target instanceof HTMLFormElement) {
      const fields = findLoginFields(target, 'capture')
      if (fields.password?.value) {
        emitCapture(target)
        return
      }
      const standalone = findStandaloneUsername(doc)
      if (standalone) {
        rememberUsername(standalone.value)
      }
    }
    if (draft?.password) {
      emitCapture(doc, { allowDraft: true })
    }
  }

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Enter') {
      return
    }
    const path = typeof event.composedPath === 'function' ? event.composedPath() : [event.target]
    const target = path[0]
    if (!(target instanceof HTMLInputElement)) {
      return
    }
    if (target.type === 'password' && target.value) {
      emitCapture(target.form ?? doc, { allowDraft: true })
      return
    }
    if (isLikelyUsernameInput(target)) {
      rememberUsername(target.value)
    }
  }

  const onClick = (event: Event) => {
    const path = typeof event.composedPath === 'function' ? event.composedPath() : [event.target]
    const control = findActionControl(path)
    if (!control || isNonSubmitControl(control)) {
      return
    }
    const form = control instanceof HTMLButtonElement || control instanceof HTMLInputElement ? control.form : null
    const fields = findLoginFields(form ?? doc, 'capture')
    if (fields.password?.value || draft?.password) {
      emitCapture(form ?? doc, { allowDraft: true })
      return
    }
    const standalone = findStandaloneUsername(doc)
    if (standalone) {
      rememberUsername(standalone.value)
    }
  }

  const onPageHide = () => {
    if (draft?.password || findLoginFields(doc, 'capture').password?.value) {
      emitCapture(doc, { allowDraft: true })
    }
  }

  doc.addEventListener('input', syncDraftFromEvent, true)
  doc.addEventListener('change', syncDraftFromEvent, true)
  doc.addEventListener('submit', onSubmit, true)
  doc.addEventListener('keydown', onKeyDown, true)
  doc.addEventListener('click', onClick, true)
  win.addEventListener('pagehide', onPageHide)
  win.addEventListener('beforeunload', onPageHide)

  bridge.on(PASSWORD_CAPTURE_CHANNEL, (_event, payload) => {
    const request = payload as CaptureRequest
    emitCapture(doc, { manual: true, requestId: request?.requestId })
  })

  bridge.on(PASSWORD_FILL_CHANNEL, (_event, payload) => {
    const credential = payload as FillRequest
    if (!credential || origin !== credential.origin) {
      bridge.send(PASSWORD_RESULT_CHANNEL, { requestId: credential?.requestId, found: false })
      return
    }
    const fields = findLoginFields(doc, 'fill')
    const usernameEditable = fields.username && !fields.username.disabled && !fields.username.readOnly && isVisibleInput(fields.username)
    if (!fields.password || (fields.username && !usernameEditable && fields.username.value !== credential.username)) {
      bridge.send(PASSWORD_RESULT_CHANNEL, { requestId: credential.requestId, found: false })
      return
    }
    if (fields.username && usernameEditable) {
      setValue(fields.username, credential.username)
      rememberUsername(credential.username)
    }
    setValue(fields.password, credential.password)
    fields.password.focus()
    bridge.send(PASSWORD_RESULT_CHANNEL, { requestId: credential.requestId, found: true })
  })

  return () => {
    doc.removeEventListener('input', syncDraftFromEvent, true)
    doc.removeEventListener('change', syncDraftFromEvent, true)
    doc.removeEventListener('submit', onSubmit, true)
    doc.removeEventListener('keydown', onKeyDown, true)
    doc.removeEventListener('click', onClick, true)
    win.removeEventListener('pagehide', onPageHide)
    win.removeEventListener('beforeunload', onPageHide)
  }
}

let installed = false

export function installBrowserPasswordCapture(): void {
  if (installed) {
    return
  }
  installed = true
  attachBrowserPasswordCapture(window, document, {
    send: (channel, payload) => ipcRenderer.send(channel, payload),
    on: (channel, listener) => ipcRenderer.on(channel, listener),
  })
}
