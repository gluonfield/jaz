import { setThemePref } from '@/lib/theme'
import type { SideBrowser } from '@/lib/sideBrowser'

export async function exerciseProfileImport(browser: SideBrowser): Promise<void> {
  const evaluate = async (expression: string) => {
    const result = await browser.call({ method: 'Runtime.evaluate', params: { expression, returnByValue: true, awaitPromise: true } }) as { result: { value: unknown } }
    return result.result.value
  }
  const until = async <T>(read: () => T): Promise<NonNullable<T>> => {
    const end = Date.now() + 5000
    while (Date.now() < end) {
      const value = read()
      if (value) {
        return value as NonNullable<T>
      }
      await new Promise(requestAnimationFrame)
    }
    throw new Error('Profile import UI did not reach the expected state')
  }
  const click = async (button: HTMLElement) => {
    const box = button.getBoundingClientRect()
    const x = Math.round(box.x + box.width / 2)
    const y = Math.round(box.y + box.height / 2)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const openImport = async () => {
    await click(document.querySelector<HTMLButtonElement>('[aria-label="Browser menu"]')!)
    const entry = await until(() => [...document.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Import cookies and passwords'))
    await new Promise((resolve) => setTimeout(resolve, 180))
    await window.smoke.capture('browser-menu')
    await click(entry)
    return until(() => document.querySelector<HTMLElement>('[role="dialog"]'))
  }
  if (document.querySelector('[data-browser-profile-offer]')) {
    throw new Error('The browser still shows a permanent import banner')
  }
  const dialog = await openImport()
  const select = await until(() => {
    const select = dialog.querySelector<HTMLSelectElement>('select')
    return select && select.options.length === 2 ? select : null
  })
  const toggle = (label: string) => dialog.querySelector<HTMLButtonElement>(`[aria-label="${label}"]`)!
  select.value = [...select.options].find((option) => option.text.includes('Firefox'))!.value
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await until(() => toggle('Import saved passwords').disabled)
  if (toggle('Import saved passwords').getAttribute('aria-checked') !== 'false') {
    throw new Error('Firefox falsely offers password import')
  }
  select.value = [...select.options].find((option) => option.text.includes('Chrome'))!.value
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await until(() => !toggle('Import saved passwords').disabled)
  const painted = async () => {
    await Promise.allSettled(document.getAnimations().map((animation) => animation.finished))
    await new Promise(requestAnimationFrame)
    await new Promise(requestAnimationFrame)
  }
  setThemePref('dark')
  await painted()
  await window.smoke.capture('profile-import-dark')
  setThemePref('light')
  await painted()
  await window.smoke.capture('profile-import-light')
  await window.smoke.resize(390, 780)
  await painted()
  if (dialog.getBoundingClientRect().width > window.innerWidth || document.documentElement.scrollWidth > window.innerWidth) {
    throw new Error('Profile import overflows a narrow window')
  }
  await window.smoke.capture('profile-import-narrow')
  await window.smoke.resize(1050, 850)
  await painted()
  await click(toggle('Import cookies'))
  const importButton = () => [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Import')!
  await click(importButton())
  await until(() => dialog.textContent!.includes('Imported 1 password.'))
  if (await evaluate("fetch('/profile-session').then(response => response.text()).then(text => text === 'Imported session is active')")) {
    throw new Error('Password-only import also copied cookies')
  }
  await click([...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Done')!)
  await until(() => !document.querySelector('[role="dialog"]'))
  const origin = await fetch('/password-origin').then((response) => response.text())
  await browser.call({ method: 'Jaz.open', params: { url: origin + '/login' } })
  const tab = await browser.call({ method: 'Jaz.tab' }) as { id: string }
  const api = window.jaz!.browserPasswords
  await api.act(Number(tab.id), { kind: 'fill', origin, username: 'imported@example.test' })
  const deadline = Date.now() + 5000
  while (!await evaluate("document.querySelector('[name=password]')?.value === 'fixture-import-latest'")) {
    if (Date.now() > deadline) {
      throw new Error('The imported password did not fill the actual browser login')
    }
    await new Promise(requestAnimationFrame)
  }
  await api.act(Number(tab.id), { kind: 'remove', origin, username: 'imported@example.test' })
  await browser.call({ method: 'Jaz.open', params: { url: location.origin + '/target' } })
  const retryDialog = await openImport()
  const passwords = await until(() => retryDialog.querySelector<HTMLButtonElement>('[aria-label="Import saved passwords"]:not([disabled])'))
  await click(passwords)
  const retryButton = await until(() => [...retryDialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Import' && !button.disabled))
  await click(retryButton)
  await until(() => retryDialog.textContent!.includes('Imported 4 cookies.'))
  if (!retryDialog.textContent!.includes('1 could not be imported.') || retryButton.disabled) {
    throw new Error('A partial import was treated as complete or could not be retried')
  }
  if (!await evaluate("fetch('/profile-session').then(response => response.text()).then(text => text === 'Imported session is active')")) {
    throw new Error('The imported HttpOnly cookie did not authenticate the side browser')
  }
  if ((await window.smoke.passwordStore()).count) {
    throw new Error('Cookie-only import also copied passwords')
  }
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await until(() => !document.querySelector('[role="dialog"]'))
}
