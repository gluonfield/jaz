/// <reference lib="dom" />
import { ipcRenderer } from 'electron'
import { PASSWORD_CAPTURE_CHANNEL, PASSWORD_FILL_CHANNEL, PASSWORD_RESULT_CHANNEL } from '@shared/browserPasswords'

function loginFields(root: Document | HTMLFormElement, operation: 'capture' | 'fill') {
  const inputs = Array.from(root instanceof HTMLFormElement ? root.elements : root.querySelectorAll('input'))
    .filter((element): element is HTMLInputElement => element instanceof HTMLInputElement)
  const passwords = inputs.filter((input) => input.type === 'password' && !input.disabled && input.getClientRects().length > 0 &&
    (operation === 'capture' ? Boolean(input.value) : !input.readOnly && !input.matches('[autocomplete~="new-password" i]')))
  const password = passwords.find((input) => input.matches(operation === 'capture'
    ? '[autocomplete~="new-password" i]' : '[autocomplete~="current-password" i]')) ?? passwords[0]
  if (!password) {
    return {}
  }
  const formInputs = inputs.filter((input) => input.form === password.form)
  const username = formInputs.find((input) => input.matches('[autocomplete~="username" i]')) ??
    formInputs.find((input) => input.type === 'email') ??
    formInputs.filter((input) => (input.type === 'text' || input.type === 'tel') && inputs.indexOf(input) < inputs.indexOf(password)).at(-1)
  return { username, password }
}

function capture(root: Document | HTMLFormElement, manual = false) {
  const { username, password } = loginFields(root, 'capture')
  if (!password?.value) {
    if (manual) {
      ipcRenderer.send(PASSWORD_RESULT_CHANNEL, false)
    }
    return
  }
  ipcRenderer.send(PASSWORD_CAPTURE_CHANNEL, { origin: location.origin, username: username?.value ?? '', password: password.value })
}

function setValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  setter.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

export function installBrowserPasswordCapture(): void {
  if (!process.isMainFrame || location.protocol !== 'https:') {
    return
  }
  document.addEventListener('submit', (event) => {
    if (event.isTrusted && event.target instanceof HTMLFormElement) {
      capture(event.target)
    }
  }, true)

  ipcRenderer.on(PASSWORD_CAPTURE_CHANNEL, () => capture(document, true))
  ipcRenderer.on(PASSWORD_FILL_CHANNEL, (_event, credential: { origin: string; username: string; password: string }) => {
    if (location.origin !== credential.origin) {
      return
    }
    const fields = loginFields(document, 'fill')
    const usernameEditable = fields.username && !fields.username.disabled && !fields.username.readOnly && fields.username.getClientRects().length > 0
    if (!fields.password || (fields.username && !usernameEditable && fields.username.value !== credential.username)) {
      ipcRenderer.send(PASSWORD_RESULT_CHANNEL, false)
      return
    }
    if (fields.username && usernameEditable) {
      setValue(fields.username, credential.username)
    }
    setValue(fields.password, credential.password)
    fields.password.focus()
    ipcRenderer.send(PASSWORD_RESULT_CHANNEL, true)
  })
}
