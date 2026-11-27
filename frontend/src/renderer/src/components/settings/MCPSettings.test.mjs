import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { keys } from '@/lib/query/keys'

mock.module('@/lib/clientRuntime', () => ({
  DEFAULT_API_BASE_URL: 'http://127.0.0.1:5299',
  openExternal: () => {},
  clientRuntime: { platform: 'browser', defaultApiBaseUrl: () => 'https://jaz.example' },
  backendFetch: (url, init) => globalThis.fetch(url, init),
  backendTransportUrl: (url) => url,
}))

mock.module('@/components/ui/Modal', () => ({ Modal: () => null }))

test.each([
  ['needs_auth', 'Sign in required'],
  ['auth_error', 'authorization failed: access_denied'],
])('MCP Settings shows sign-in for a custom server with status %s', async (status, label) => {
  const { MCPSettings } = await import('@/components/settings/MCPSettings')
  const client = new QueryClient()
  const servers = [
    { id: 'custom', name: 'Custom fixture', status, error: status === 'auth_error' ? label : undefined },
    { id: 'connection', name: 'Connected fixture', source: 'connection' },
    { id: 'plugin', name: 'Plugin fixture', source: 'plugin' },
  ].map((server) => ({
    transport: 'streamable_http',
    url: 'https://example.invalid/mcp',
    enabled: true,
    status: 'connected',
    tool_count: 1,
    headers: [],
    tools: [],
    ...server,
  }))
  client.setQueryData(keys.mcpServers, { servers, oauth_redirect_url: 'https://jaz.chat/oauth/callback' })
  const html = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(MCPSettings)))
  expect(html).toContain('Custom fixture')
  expect(html).toContain(label)
  expect(html).toContain('Sign in to MCP server')
  expect(html).not.toContain('Show Custom fixture tools')
  expect(html).not.toContain('Connected fixture')
  expect(html).not.toContain('Plugin fixture')
  expect(html).toContain('Manage connected apps in Connections')
  expect(client.getQueryData(keys.mcpServers).servers).toEqual(servers)
  client.clear()
})
