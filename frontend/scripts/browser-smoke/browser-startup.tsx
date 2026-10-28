import { focusManager, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from '@/App'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { connectRemote, disconnectBackend } from '@/lib/connection'
import { queryClient } from '@/lib/query/queryClient'

export async function exerciseBrowserStartup(): Promise<void> {
  const backend = await window.smoke.backend()
  const fixture = await (await fetch(backend + '/exercise-startup-backend')).json() as { url: string; token: string; session: string }
  const originalURL = apiBaseUrl()
  const originalFetch = window.fetch
  let unauthorizedSettings = 0
  window.fetch = async (input, init) => {
    const response = await originalFetch(input, init)
    if (String(input) === fixture.url + '/v1/browser' && response.status === 401) unauthorizedSettings += 1
    return response
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const route = createRootRoute({ component: () => <div>Startup fixture</div> })
  const router = createRouter({ routeTree: route, history: createMemoryHistory() })
  disconnectBackend()
  setApiBaseUrl(fixture.url)
  focusManager.setFocused(false)
  try {
    root.render(<StrictMode><QueryClientProvider client={queryClient}><App router={router} /></QueryClientProvider></StrictMode>)
    await new Promise(requestAnimationFrame)
    await new Promise(requestAnimationFrame)
    const error = await connectRemote(fixture.url + '#key=' + fixture.token)
    if (error) throw new Error('Authenticated startup failed: ' + error)
    const deadline = Date.now() + 7000
    let connected = false
    while (Date.now() < deadline) {
      const response = await originalFetch(`${fixture.url}/v1/sessions/${fixture.session}/browser`, {
        method: 'POST', headers: { Authorization: 'Bearer ' + fixture.token }, body: '{"action":"status"}',
      })
      if (response.ok) {
        connected = true
        break
      }
      await new Promise((resolve) => setTimeout(resolve, 50))
    }
    if (!connected) throw new Error('Desktop host did not register after authenticated startup; unauthorized settings requests: ' + unauthorizedSettings)
    if (unauthorizedSettings) throw new Error('Browser settings were fetched before authentication')
    const target = new URL('/target?authenticated-startup', location.origin)
    target.hostname = 'jaz-startup-fixture.localhost'
    const opened = await originalFetch(`${fixture.url}/v1/sessions/${fixture.session}/browser`, {
      method: 'POST', headers: { Authorization: 'Bearer ' + fixture.token }, body: JSON.stringify({ action: 'navigate', url: target.href }),
    })
    const output = await opened.text()
    if (!opened.ok || !output.includes('Browser control')) {
      throw new Error('An unvisited conversation could not navigate after authenticated startup: ' + opened.status + ' ' + output.slice(0, 700))
    }
  } finally {
    root.unmount()
    element.remove()
    focusManager.setFocused(undefined)
    window.fetch = originalFetch
    disconnectBackend()
  }
  const error = await connectRemote(originalURL)
  if (error) throw new Error('Could not restore the smoke connection: ' + error)
}
