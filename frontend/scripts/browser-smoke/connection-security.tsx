import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { DevicesSettings } from '@/components/settings/DevicesSettings'
import { MCPSettings } from '@/components/settings/MCPSettings'
import { BrowserSettings } from '@/components/settings/BrowserSettings'
import { cachedApiAuthToken, apiBaseUrl, apiWebSocket, setApiAuthToken, setApiBaseUrl } from '@/lib/api/client'
import { clientRuntime } from '@/lib/clientRuntime'

export async function exerciseConnectionSecurity(serveBrowser: (socket: WebSocket) => void): Promise<void> {
  const backend = await window.smoke.backend()
  const originalToken = cachedApiAuthToken(backend)
  const originalFetch = window.fetch
  const originalStart = clientRuntime.startLocalBackend
  const originalURL = apiBaseUrl()
  const originalSize = { width: window.innerWidth, height: window.innerHeight }
  const local = 'http://127.0.0.1:5299'
  const saves: Record<string, unknown>[] = []
  let tokenSet = true
  let socket: WebSocket | undefined
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;padding:16px;background:var(--color-bg);overflow:auto'
  document.body.append(element)
  const root = createRoot(element)
  const clients: QueryClient[] = []
  clientRuntime.startLocalBackend = async () => ({ ok: true, url: local, key: 'fixture-host-credential' })
  window.fetch = async (input, init) => {
    const url = String(input)
    const authorization = new Headers(init?.headers).get('Authorization')
    if (authorization === 'Bearer fixture-host-credential' && !url.startsWith(local + '/')) {
      throw new Error('Host credential escaped the local backend')
    }
    if (url.endsWith('/v1/devices')) {
      return Response.json({ can_manage: authorization === 'Bearer fixture-host-credential', devices: [{ id: 'phone', name: 'Phone', kind: 'mobile', status: 'approved', created_at: new Date().toISOString() }], pairings: [] })
    }
    if (url.endsWith('/v1/devices/connection-link')) {
      return Response.json({ url: 'https://backend.example#key=invite_fixture', base_url: 'https://backend.example', public_url: 'https://backend.example' })
    }
    if (url.includes('/v1/mcp/servers')) {
      const server = { id: 'private-tools', name: 'Private tools', url: 'https://tools.example/mcp', enabled: false, status: 'disabled', tool_count: 0, transport: 'streamable_http', bearer_token_set: tokenSet, headers: [{ name: 'X-Key', value_set: true }], created_at: '', updated_at: '' }
      if (init?.method === 'PUT') {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>
        saves.push(body)
        if (body.bearer_token === '') {
          tokenSet = false
        }
        return Response.json({ ...server, bearer_token_set: tokenSet })
      }
      const failing = { ...server, id: 'vehicle-history', name: 'Vehicle history', url: 'https://mcp.example.com/v1/organizations/acme/mcp', enabled: true, status: 'error', error: 'calling "initialize": sending "initialize": rejected by transport: Post "https://mcp.example.com/v1/organizations/acme/mcp": tls: failed to verify certificate: x509: certificate is valid for ingress.local, not mcp.example.com' }
      return Response.json({ servers: [server, failing] })
    }
    return originalFetch(input, init)
  }
  const render = (view: 'devices' | 'mcp' | 'browser', key: string) => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clients.push(client)
    root.render(<QueryClientProvider client={client} key={key}>{view === 'devices' ? <DevicesSettings /> : view === 'browser' ? <BrowserSettings /> : <MCPSettings />}</QueryClientProvider>)
  }
  const button = (text: string) => [...document.querySelectorAll('button')].find((item) => item.textContent?.trim() === text)
  try {
    setApiBaseUrl(backend)
    for (const credential of ['wrong-credential', 'browser-smoke-credential']) {
      setApiAuthToken(backend, credential)
      socket = apiWebSocket('/v1/sessions/browser-fixture/browser')
      const connected = await new Promise<boolean>((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error('Authenticated browser socket did not settle')), 5000)
        socket.onopen = () => {
          clearTimeout(timer)
          resolve(true)
        }
        socket.onerror = () => {
          clearTimeout(timer)
          resolve(false)
        }
      })
      if (connected !== (credential === 'browser-smoke-credential') || (connected && socket.protocol !== 'jaz')) {
        socket.close()
        throw new Error('Browser transport failed authentication or non-secret protocol negotiation')
      }
      if (connected) {
        serveBrowser(socket)
        const response = await originalFetch(backend + '/v1/sessions/browser-fixture/browser', { method: 'POST', body: JSON.stringify({ action: 'status' }) })
        const result = await response.json()
        if (!response.ok || result.status !== 'ok') {
          throw new Error('Authenticated browser did not register with the production backend')
        }
        const exercise = await originalFetch(backend + '/exercise?url=' + encodeURIComponent(location.origin + '/target?authenticated=1'))
        if (!exercise.ok) {
          throw new Error('Authenticated MCP browser actions failed: ' + await exercise.text())
        }
        const closed = new Promise((resolve) => {
          socket.onclose = resolve
        })
        socket.close()
        await closed
      }
    }
    for (const [width, name] of [[1440, 'desktop'], [390, 'phone']] as const) {
      await window.smoke.resize(width, 900)
      for (const host of [true, false]) {
        setApiBaseUrl(host ? local : 'https://remote.example')
        render('devices', `${name}-${host}`)
        await until(() => Boolean(element.textContent?.includes('Phone')))
        await until(() => Boolean(button('Add device')) === host)
        if (Boolean(button('Revoke')) !== host) {
          throw new Error('Device-management controls do not follow the server capability')
        }
        if (!host && !element.textContent?.includes('Manage devices from the host computer.')) {
          throw new Error('Remote devices have no explanation for management ownership')
        }
        if (element.scrollWidth > element.clientWidth) {
          throw new Error('Device settings overflow horizontally')
        }
        await window.smoke.capture(`security-${host ? 'host' : 'remote'}-${name}`)
      }
      setApiBaseUrl('https://remote.example')
      tokenSet = true
      render('mcp', `mcp-${name}`)
      await until(() => Boolean(document.querySelector('[aria-label="Edit MCP server"]')))
      const failingName = element.querySelector('[title="Vehicle history"]')!
      if (failingName.scrollWidth > failingName.clientWidth || element.scrollWidth > element.clientWidth) {
        throw new Error('A long MCP connection error hides the server name')
      }
      await window.smoke.capture(`mcp-servers-${name}`)
      await click(document.querySelector('[aria-label="Edit MCP server"]')!)
      await until(() => Boolean(document.querySelector('input[placeholder="Saved token"]')))
      const savedInput = document.querySelector<HTMLInputElement>('input[placeholder="Saved token"]')!
      if (savedInput.value !== '') {
        throw new Error('Settings exposed the saved bearer')
      }
      await click(button('Advanced')!)
      await until(() => Boolean(document.querySelector('input[placeholder="Saved value"]')))
      await window.smoke.capture(`security-saved-secrets-${name}`)
      const before = saves.length
      await click(button('Save changes')!)
      await until(() => saves.length === before + 1 && !button('Save changes'))
      if ('bearer_token' in saves[before] || (saves[before].headers as { value?: string }[])[0].value !== undefined) {
        throw new Error('Editing settings overwrote an untouched saved secret')
      }
      await click(document.querySelector('[aria-label="Edit MCP server"]')!)
      await until(() => Boolean(button('Remove saved token')))
      await click(button('Remove saved token')!)
      await click(button('Save changes')!)
      await until(() => saves.length === before + 2)
      if (saves[before + 1].bearer_token !== '') {
        throw new Error('Explicit saved-token removal was lost')
      }
      await until(() => !button('Save changes'))
      setApiBaseUrl(backend)
      render('browser', `browser-${name}`)
      await until(() => Boolean(element.querySelector('[aria-label="Enable browser tools"]')))
      if (element.textContent?.includes('Extension') || element.textContent?.includes('Background Chromium') || element.textContent?.includes('Browser backend')) {
        throw new Error('Removed browser backend selection remains visible')
      }
      await click(element.querySelector('[aria-label="Enable browser tools"]')!)
      await until(() => element.querySelector('[aria-label="Enable browser tools"]')?.getAttribute('aria-checked') === 'false')
      await click(element.querySelector('[aria-label="Enable browser tools"]')!)
      await until(() => element.querySelector('[aria-label="Enable browser tools"]')?.getAttribute('aria-checked') === 'true')
      if (element.scrollWidth > element.clientWidth) {
        throw new Error('Browser settings overflow horizontally')
      }
      await window.smoke.capture(`browser-settings-${name}`)
    }
  } finally {
    socket?.close()
    root.unmount()
    clients.forEach((client) => client.clear())
    element.remove()
    window.fetch = originalFetch
    clientRuntime.startLocalBackend = originalStart
    setApiAuthToken(backend, originalToken)
    setApiBaseUrl(originalURL)
    await window.smoke.resize(originalSize.width, originalSize.height)
  }
}

async function click(element: Element): Promise<void> {
  element.scrollIntoView({ block: 'nearest' })
  await new Promise(requestAnimationFrame)
  await Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished))
  const rect = element.getBoundingClientRect()
  const x = Math.round(rect.x + rect.width / 2)
  const y = Math.round(rect.y + rect.height / 2)
  await window.smoke.pointer('mouseMove', x, y)
  await window.smoke.pointer('mouseDown', x, y)
  await window.smoke.pointer('mouseUp', x, y)
}

async function until(check: () => boolean): Promise<void> {
  const deadline = Date.now() + 5000
  while (!check()) {
    if (Date.now() > deadline) {
      throw new Error('Connection settings: ' + check.toString())
    }
    await new Promise((resolve) => setTimeout(resolve, 20))
  }
}
