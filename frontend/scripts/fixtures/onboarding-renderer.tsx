import React from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SystemPermissionRows } from '@/components/settings/SystemPermissionRows'
import { ComputerSettings } from '@/components/settings/ComputerSettings'
import { keys } from '@/lib/query/keys'
import type { SystemPermission, SystemPermissionStatus } from '@shared/systemPermissions'
import '@/styles/globals.css'

const root = createRoot(document.getElementById('root')!)
const host = window as Window & {
  fixtureState: { calls: SystemPermission[]; current: SystemPermissionStatus; mode: string; resume(): void }
  renderPermissionState(mode: string): Promise<void>
  runPermissionStates(): Promise<{ passed: boolean }>
}
let client: QueryClient
const pause = () => new Promise(resolve => setTimeout(resolve, 80))
const check = (value: unknown, message: string) => {
  if (!value) throw new Error(`${message}: ${document.body.innerText}`)
}
const text = () => document.body.innerText
const buttons = () => [...document.querySelectorAll('button')]
const click = (label: string) => {
  const button = buttons().find(button => button.textContent === label)
  check(button, `Missing ${label}: ${text()}`)
  button!.click()
}
host.renderPermissionState = async (mode: string) => {
  client?.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryDefaults(keys.computerSettings, { staleTime: Infinity })
  client.setQueryData(keys.computerSettings, { enabled: true })
  host.fixtureState.mode = mode === 'error' || mode === 'loading' || mode === 'action-error' ? mode : 'ready'
  host.fixtureState.current = { accessibility: 'needed', screenRecording: 'needed', microphone: 'needed', notifications: 'needed' }
  if (mode === 'granted') {
    host.fixtureState.current.microphone = 'granted'
    host.fixtureState.current.notifications = 'granted'
  }
  const computer = mode === 'computer'
  document.documentElement.className = 'dark'
  root.render(<QueryClientProvider key={mode} client={client}>
    <main className="mx-auto max-w-[520px] p-8 text-ink">
      {mode === 'driver-error' ? <ComputerSettings /> : <>
        <h1 className="mb-6 text-center text-[22px] font-semibold">{computer ? 'Let jaz use your computer' : 'Talk to jaz'}</h1>
        <div className="divide-y divide-border rounded-card bg-surface">
          <SystemPermissionRows permissions={computer ? ['accessibility', 'screenRecording'] : ['microphone', 'notifications']} />
        </div>
      </>}
    </main>
  </QueryClientProvider>)
  await pause()
}
host.runPermissionStates = async () => {
  await host.renderPermissionState('loading')
  check(document.querySelector('[aria-label="Checking"]'), 'Loading must be visible')
  check(!buttons().length, 'No Allow before checking')
  await host.renderPermissionState('computer')
  check(buttons().every(button => button.textContent === 'Open Settings'), 'Computer actions must name Settings')
  await host.renderPermissionState('voice')
  click('Send test')
  await pause()
  check(host.fixtureState.calls.at(-1) === 'notifications', 'Notification button must request a notification test')
  check(text().includes('Waiting for macOS'), 'Request must show waiting')
  check(buttons().every(button => button.disabled), 'Prevent concurrent permission actions')
  host.fixtureState.resume()
  await pause()
  check(text().includes('Test notification sent'), 'Delivered notification must update its row')
  check(document.querySelector('[aria-label="Test sent"]'), 'Delivery is not labelled as an OS grant')
  click('Allow')
  await pause()
  check(host.fixtureState.calls.at(-1) === 'microphone', 'Microphone button must request microphone access')
  host.fixtureState.resume()
  await pause()
  check(document.querySelector('[aria-label="Allowed"]'), 'Microphone grant must refresh')
  check(!buttons().some(button => button.textContent === 'Allow'), 'Granted permission cannot still offer Allow')
  host.fixtureState.current.microphone = 'off'
  await client.refetchQueries({ queryKey: keys.systemPermissions })
  await pause()
  check(buttons().some(button => button.textContent === 'Open Settings'), 'Refused microphone must open Settings')
  host.fixtureState.current.microphone = 'granted'
  window.dispatchEvent(new Event('focus'))
  await pause()
  check(document.querySelector('[aria-label="Allowed"]'), 'Returning from Settings must refresh')
  await host.renderPermissionState('error')
  check(text().includes('Couldn’t check permissions'), 'Query failure must be visible')
  check(!text().includes('dlopen'), 'Raw status diagnostics must stay out of the UI')
  host.fixtureState.mode = 'ready'
  click('Try again')
  await pause()
  check(buttons().some(button => button.textContent === 'Allow'), 'Retry must recover')
  await host.renderPermissionState('action-error')
  click('Allow')
  await pause()
  host.fixtureState.resume()
  await pause()
  check(text().includes('Try again in System Settings'), 'Action errors must remain actionable')
  check(!text().includes('dlopen'), 'Raw action diagnostics must stay out of the UI')
  await host.renderPermissionState('voice')
  host.fixtureState.current.notifications = 'unavailable'
  await client.refetchQueries({ queryKey: keys.systemPermissions })
  await pause()
  check(text().includes('Needs a signed Jaz build'), 'Unsupported notification host must be explicit')
  check(!buttons().some(button => button.textContent === 'Send test'), 'Unavailable notifications cannot offer a test')
  await host.renderPermissionState('driver-error')
  check(text().includes('Update or reinstall Jaz'), 'Driver failure must offer recovery')
  check(buttons().some(button => button.textContent === 'Try again'), 'Driver failure must be retryable')
  check(document.documentElement.scrollWidth <= window.innerWidth, 'Permission states must fit the window')
  return { passed: true }
}
