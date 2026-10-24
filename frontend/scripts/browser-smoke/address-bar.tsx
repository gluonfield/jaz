import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import { PreviewPanel } from '@/components/session/PreviewPanel'
import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'

function Fixture({ name }: { name: string }) {
  const [target, setTarget] = useState(() => ({
    displayUrl: `${location.origin}/target?address=${name}`,
    sourceUrl: `${location.origin}/target?address=${name}`,
  }))
  return <div data-address-browser={name}>
    <PreviewPanel embedded target={target} onTargetChange={setTarget} />
  </div>
}

export async function exerciseAddressBar(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  try {
    root.render(<StrictMode><Fixture name="owner" /><Fixture name="neighbour" /></StrictMode>)
    const deadline = Date.now() + 5000
    while (element.querySelectorAll('[aria-label="Reload preview"]:not(:disabled)').length !== 2) {
      if (Date.now() > deadline) {
        throw new Error('Address-bar webviews did not become ready')
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
    await checkAddresses(element.querySelector('[data-address-browser="owner"]')!, element.querySelector<PreviewWebviewElement>('[data-address-browser="neighbour"] webview')!)
  } finally {
    root.unmount()
    element.remove()
  }
}

async function checkAddresses(panel: Element, neighbour: PreviewWebviewElement): Promise<void> {
  const address = panel.querySelector<HTMLInputElement>('input[spellcheck="false"]')!
  const neighbourURL = neighbour.getURL()
  const neighbourID = neighbour.getWebContentsId()
  let submissions = 0
  address.form!.addEventListener('submit', () => submissions++)
  const submit = async (value: string) => {
    const before = submissions
    address.focus()
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(address, value)
    address.dispatchEvent(new Event('input', { bubbles: true }))
    await window.smoke.key('Enter')
    if (submissions !== before + 1) {
      throw new Error('Enter did not submit the address form')
    }
  }
  await window.smoke.navigationProbe(true)
  try {
    for (const blank of ['', '   ']) {
      await submit(blank)
      if ((await window.smoke.navigationProbe()).length || panel.querySelector('.text-danger')) {
        throw new Error('Blank address submission navigated or displayed an error')
      }
    }
    const cases = [
      ['weather', 'https://www.google.com/search?q=weather'],
      ['  café 東京 😀 + &?q=one#two/%  ', 'https://www.google.com/search?q=caf%C3%A9%20%E6%9D%B1%E4%BA%AC%20%F0%9F%98%80%20%2B%20%26%3Fq%3Done%23two%2F%25'],
      ['a  b', 'https://www.google.com/search?q=a%20%20b'],
      [' https://example.com/path?q=a%20b&x=1#part ', 'https://example.com/path?q=a%20b&x=1#part'],
      ['example.com:8080/path', 'https://example.com:8080/path'],
      ['localhost:3000/app', 'http://localhost:3000/app'],
      ['127.0.0.1:3000/app', 'http://127.0.0.1:3000/app'],
      ['[::1]:3000/app', 'http://[::1]:3000/app'],
      ['192.168.1.1:8080/app', 'https://192.168.1.1:8080/app'],
      ['printer:443/path', 'https://printer/path'],
    ]
    for (const [input, expected] of cases) {
      const count = (await window.smoke.navigationProbe()).length
      await submit(input)
      const deadline = Date.now() + 3000
      const ready = () => !panel.querySelector<HTMLButtonElement>('[aria-label="Reload preview"]')!.disabled
        && panel.querySelector<PreviewWebviewElement>('webview')?.getURL() === expected
        && !panel.querySelector('.animate-spin')
      while (!ready() && Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 20))
      }
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
      if (!ready()) {
        throw new Error(`Address ${JSON.stringify(input)} did not finish navigating to ${expected}; loaded ${panel.querySelector<PreviewWebviewElement>('webview')?.getURL()}`)
      }
      const view = panel.querySelector<PreviewWebviewElement>('webview')!
      const requests = (await window.smoke.navigationProbe()).slice(count)
      if (!requests.length || requests.some((request) => request.url !== expected || request.webContentsId !== view.getWebContentsId()) || view.getURL() !== expected) {
        throw new Error(`Address ${JSON.stringify(input)} requested ${JSON.stringify(requests)}, expected ${expected} in its own webview; loaded ${view.getURL()}`)
      }
      if (neighbour.getURL() !== neighbourURL || neighbour.getWebContentsId() !== neighbourID) {
        throw new Error('Address submission changed the neighbouring browser')
      }
    }
    console.warn(`Address bar: ${cases.length} Enter navigations, 2 blank submissions and neighbouring browser preservation passed`)
  } finally {
    await window.smoke.navigationProbe(false)
  }
}
