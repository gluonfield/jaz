import { setThemePref } from '@/lib/theme'

export async function exerciseProfileImport(evaluate: (expression: string) => Promise<unknown>): Promise<void> {
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
  const offer = await until(() => document.querySelector<HTMLElement>('[data-browser-profile-offer]'))
  await window.smoke.capture('profile-offer')
  offer.querySelector<HTMLButtonElement>('[aria-label="Dismiss import suggestion"]')!.click()
  await until(() => !document.querySelector('[data-browser-profile-offer]'))
  if (!await window.jaz!.browserProfiles.dismissed()) {
    throw new Error('Import dismissal was not persisted')
  }
  document.querySelector<HTMLButtonElement>('#profile-settings button')!.click()
  const dialog = await until(() => document.querySelector<HTMLElement>('[role="dialog"]'))
  const select = await until(() => {
    const select = dialog.querySelector<HTMLSelectElement>('select')
    return select && select.options.length > 2 ? select : null
  })
  const choose = (label: string) => {
    select.value = [...select.options].find((option) => option.text.includes(label))!.value
    select.dispatchEvent(new Event('change', { bubbles: true }))
  }
  choose('Chrome')
  const checkbox = await until(() => [...dialog.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].find((input) => input.closest('label')!.textContent!.includes('127.0.0.1')))
  if (dialog.querySelector('input:checked')) {
    throw new Error('Cookie sites were selected without user input')
  }
  checkbox.click()
  choose('Firefox')
  await until(() => dialog.textContent!.includes('firefox.test'))
  if (dialog.querySelector('input:checked')) {
    throw new Error('Site selection leaked between profiles')
  }
  choose('Chrome')
  const selected = await until(() => [...dialog.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].find((input) => input.closest('label')!.textContent!.includes('127.0.0.1')))
  selected.click()
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
  const button = [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Import 1 site')!
  if (button.disabled) {
    throw new Error('Selected sites could not be imported')
  }
  button.click()
  await until(() => dialog.textContent!.includes('Imported 1 cookie.'))
  if (!await evaluate("fetch('/profile-session').then(response => response.text()).then(text => text === 'Imported session is active')")) {
    throw new Error('The imported HttpOnly cookie did not authenticate the side browser')
  }
  const finished = [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Done')!
  finished.click()
  await until(() => !document.querySelector('[role="dialog"]'))
  document.querySelector<HTMLButtonElement>('#profile-settings button')!.click()
  const retryDialog = await until(() => document.querySelector<HTMLElement>('[role="dialog"]'))
  const retrySelect = await until(() => {
    const control = retryDialog.querySelector<HTMLSelectElement>('select')
    return control && control.options.length > 2 ? control : null
  })
  retrySelect.value = [...retrySelect.options].find((option) => option.text.includes('Chrome'))!.value
  retrySelect.dispatchEvent(new Event('change', { bubbles: true }))
  const badCookie = await until(() => [...retryDialog.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].find((input) => input.closest('label')!.textContent!.includes('bad-hash.test')))
  badCookie.click()
  const retryButton = await until(() => [...retryDialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Import 1 site' && !button.disabled))
  retryButton.click()
  await until(() => retryDialog.textContent!.includes('Imported 0 cookies. 1 could not be imported.'))
  if (retryButton.disabled || retryDialog.querySelector('.lucide-check')) {
    throw new Error('A failed cookie import was treated as completed')
  }
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await until(() => !document.querySelector('[role="dialog"]'))
}
