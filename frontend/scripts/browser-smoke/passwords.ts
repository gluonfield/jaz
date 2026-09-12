import type { SideBrowser } from '@/lib/sideBrowser'
import { setThemePref } from '@/lib/theme'

export async function exercisePasswords(browser: SideBrowser): Promise<void> {
  const origin = await fetch('/password-origin').then((response) => response.text())
  const evaluate = async (expression: string) => {
    const result = await browser.call({ method: 'Runtime.evaluate', params: { expression, returnByValue: true, awaitPromise: true } }) as { result: { value: unknown } }
    return result.result.value
  }
  const until = async (check: () => boolean | Promise<boolean>) => {
    const end = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > end) {
        await window.smoke.capture('password-failure')
        const page = await evaluate(`({url:location.href, fields:document.querySelectorAll('input').length, first:document.querySelector('[name=password]')?.value === 'fixture-password-one', second:document.querySelector('[name=password]')?.value === 'fixture-password-two'})`)
        throw new Error('Password browser check timed out: ' + JSON.stringify({ page, menu: document.querySelector('[aria-label="Browser passwords"]')?.textContent }))
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const navigate = async (url: string) => {
    await browser.call({ method: 'Jaz.open', params: { url } })
    await until(async () => await evaluate('document.readyState') === 'complete')
  }
  await navigate(origin + '/login')
  const tab = await browser.call({ method: 'Jaz.tab' }) as { id: string }
  const id = Number(tab.id)
  const api = window.jaz!.browserPasswords
  const state = () => api.state(id)
  const click = (label: string) => {
    const button = Array.from(document.querySelectorAll<HTMLButtonElement>('[aria-label="Browser passwords"] button'))
      .find((entry) => entry.textContent?.trim() === label)
    if (!button) {
      throw new Error(`Missing password button: ${label}`)
    }
    button.click()
  }
  const submit = async (password: string, username = 'browser-fixture@example.test') => {
    await evaluate(`document.querySelector('[name="username"]').value = ${JSON.stringify(username)}
document.querySelector('[name="password"]').value = ${JSON.stringify(password)}
document.querySelector('form').requestSubmit()`)
    await until(async () => Boolean((await state()).pending))
    const label = (await state()).pending?.update ? 'Update' : 'Save'
    await until(() => Array.from(document.querySelectorAll<HTMLButtonElement>('[aria-label="Browser passwords"] button'))
      .some((button) => button.textContent?.trim() === label && !button.disabled))
    await until(async () => await evaluate('location.pathname') === '/welcome')
  }

  if (await evaluate('typeof window.jaz') !== 'undefined') {
    throw new Error('The browser page received the application bridge')
  }
  await submit('fixture-password-one')
  const proposed = await state()
  if (JSON.stringify(proposed).includes('fixture-password-one') || proposed.pending?.update || proposed.origin !== origin || proposed.usernames.length) {
    throw new Error('Password capture leaked a secret, origin or saved without consent')
  }
  setThemePref('dark')
  await new Promise((resolve) => setTimeout(resolve, 200))
  await window.smoke.capture('password-save-dark')
  setThemePref('light')
  await new Promise((resolve) => setTimeout(resolve, 200))
  await window.smoke.capture('password-save-light')
  click('Save')
  await until(async () => (await state()).usernames.length === 1)
  const persisted = await window.smoke.passwordStore()
  if (persisted.count !== 1 || persisted.plaintext) {
    throw new Error('Passwords did not survive encrypted store reload')
  }

  await navigate(origin + '/login')
  await api.act(id, { kind: 'fill', origin, username: 'browser-fixture@example.test' })
  await until(async () => await evaluate("document.querySelector('[name=password]').value === 'fixture-password-one' && document.querySelector('[name=username]').value === 'browser-fixture@example.test'"))
  await evaluate("document.querySelector('form').requestSubmit()")
  await until(async () => await evaluate('location.pathname') === '/welcome')
  if ((await state()).pending) {
    throw new Error('An unchanged saved password prompted again')
  }

  await navigate(origin + '/login')
  await submit('fixture-password-two')
  if (!(await state()).pending?.update) {
    throw new Error('An existing login did not offer an update')
  }
  click('Update')
  await until(async () => !(await state()).pending)
  await navigate(origin + '/login')
  document.querySelector<HTMLButtonElement>('[aria-label="Passwords"]')!.click()
  await until(() => {
    const button = document.querySelector<HTMLButtonElement>('[title="Fill this login"]')
    return Boolean(button && !button.disabled && document.querySelector('[aria-label="Passwords"][aria-expanded="true"]'))
  })
  document.querySelector<HTMLButtonElement>('[title="Fill this login"]')!.click()
  await until(async () => await evaluate("document.querySelector('[name=password]').value === 'fixture-password-two'"))

  for (let cycle = 0; cycle < 3; cycle += 1) {
    const password = `fixture-password-cycle-${cycle}`
    await navigate(origin + '/login')
    await submit(password)
    click('Update')
    await until(async () => !(await state()).pending)
    await navigate(origin + '/login')
    await api.act(id, { kind: 'fill', origin, username: 'browser-fixture@example.test' })
    await until(async () => await evaluate(`location.pathname === '/login' && document.querySelector('[name=password]')?.value === ${JSON.stringify(password)}`))
  }

  const denied = await api.act(id, { kind: 'fill', origin: 'https://another-site.test', username: 'browser-fixture@example.test' })
  if (!denied.error) {
    throw new Error('Cross-origin password filling was accepted')
  }
  let unowned = false
  try {
    await api.state(id - 1)
  } catch {
    unowned = true
  }
  if (!unowned) {
    throw new Error('Password operations escaped the owning webview')
  }

  await submit('fixture-password-not-saved')
  const oldPrompt = (await state()).pending!
  await navigate(location.origin + '/target')
  const insecure = await api.act(id, { kind: 'fill', origin, username: 'browser-fixture@example.test' })
  if (!insecure.error || insecure.usernames.length || insecure.pending) {
    throw new Error('Saved logins leaked to an HTTP origin')
  }
  await navigate(origin + '/login')
  const stale = await api.act(id, { kind: 'save', id: oldPrompt.id })
  if (!stale.error) {
    throw new Error('A password prompt survived cross-origin navigation')
  }
  await api.act(id, { kind: 'remove', origin, username: 'browser-fixture@example.test' })
  if ((await state()).usernames.length || (await window.smoke.passwordStore()).count) {
    throw new Error('Password deletion was not persisted')
  }

  await submit('fixture-password-dismissed')
  click('Not now')
  await until(async () => !(await state()).pending)
  if ((await state()).usernames.length) {
    throw new Error('Dismissing the prompt saved the password')
  }

  const accounts = ['hidden', 'readonly', 'hidden-type']
  for (const mode of accounts) {
    await navigate(origin + '/login')
    await evaluate(`document.querySelector('[name=username]').autocomplete = 'section-login username'
document.querySelector('[name=username]').style.display = ${JSON.stringify(mode === 'hidden' ? 'none' : 'block')}
document.querySelector('[name=username]').readOnly = ${mode === 'readonly'}
document.querySelector('[name=username]').type = ${JSON.stringify(mode === 'hidden-type' ? 'hidden' : 'email')}`)
    const username = `${mode}@example.test`
    await submit(`fixture-password-${mode}`, username)
    const candidate = (await state()).pending!
    if (candidate.username !== username || candidate.update) {
      throw new Error(`A ${mode} username lost its account identity`)
    }
    click('Save')
    await until(async () => !(await state()).pending)
  }
  if ((await state()).usernames.length !== accounts.length || (await window.smoke.passwordStore()).count !== accounts.length) {
    throw new Error('Distinct accounts overwrote one another')
  }

  await navigate(origin + '/login')
  await evaluate(`document.body.innerHTML = '<form id="signup"><input autocomplete="username"><input type="password" autocomplete="new-password"></form><form id="login"><input name="username" autocomplete="section-login username"><input name="password" type="password" autocomplete="section-login current-password"></form>'`)
  await api.act(id, { kind: 'fill', origin, username: 'hidden@example.test' })
  await until(async () => await evaluate("document.querySelector('#login [name=password]').value === 'fixture-password-hidden'"))
  if (!await evaluate("document.querySelector('#signup input[type=password]').value === '' && document.querySelector('#login [name=username]').value === 'hidden@example.test'")) {
    throw new Error('Autofill chose the signup form or a username outside the login form')
  }
  for (const mode of ['readonly', 'hidden']) {
    await evaluate(`document.querySelector('#login [name=username]').readOnly = ${mode === 'readonly'}
document.querySelector('#login [name=username]').style.display = ${JSON.stringify(mode === 'hidden' ? 'none' : 'block')}
document.querySelector('#login [name=password]').value = ''`)
    await api.act(id, { kind: 'fill', origin, username: 'readonly@example.test' })
    await until(async () => Boolean((await state()).error))
    if (!await evaluate("document.querySelector('#login [name=password]').value === '' && document.querySelector('#login [name=username]').value === 'hidden@example.test'")) {
      throw new Error('Autofill changed a locked account identity')
    }
    await api.act(id, { kind: 'fill', origin, username: 'hidden@example.test' })
    await until(async () => await evaluate("document.querySelector('#login [name=password]').value === 'fixture-password-hidden'"))
  }
  await evaluate(`document.querySelector('#login').insertAdjacentHTML('beforeend', '<input type="password" autocomplete="new-password" value="fixture-new-password">')`)
  await api.act(id, { kind: 'capture' })
  await until(async () => Boolean((await state()).pending))
  const update = (await state()).pending!
  if (update.username !== 'hidden@example.test' || !update.update) {
    throw new Error('Password-change capture lost the account identity')
  }
  await api.act(id, { kind: 'save', id: update.id })
  await evaluate("document.querySelector('#login [name=password]').value = ''")
  await api.act(id, { kind: 'fill', origin, username: 'hidden@example.test' })
  await until(async () => await evaluate("document.querySelector('#login [name=password]').value === 'fixture-new-password'"))
  for (const mode of accounts) {
    await api.act(id, { kind: 'remove', origin, username: `${mode}@example.test` })
  }
  await navigate(location.origin + '/target')
}
