import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import type { IntegrationPlugin } from '@/lib/api/types'

export async function exerciseConnectCard(): Promise<void> {
  const authURL = 'https://slack.example/oauth/authorize?state=smoke'
  const slack = {
    id: 'slack',
    name: 'Slack',
    description: 'Search channels and send messages.',
    provider: { id: 'slack', name: 'Slack' },
    icon: { kind: 'asset', value: 'slack' },
    auth: [{ kind: 'oauth' }],
    capabilities: [],
    multi_account: true,
    implementation: { status: 'available', owner: 'jaz' },
    connection: { status: 'not_connected' },
  } as IntegrationPlugin
  let starts = 0
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = String(input)
    if (url.endsWith('/v1/connections/plugins/slack/connect') && init?.method === 'POST') {
      starts += 1
      return Response.json({ type: 'oauth', auth_url: authURL })
    }
    if (url.endsWith('/v1/connections/plugins')) {
      const connection = starts
        ? { status: 'connected', accounts: [{ id: 'slack:team', provider: 'slack', account_name: 'Cambridge team' }] }
        : slack.connection
      return Response.json({ plugins: [{ ...slack, connection }] })
    }
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;padding:24px;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const button = () => [...element.querySelectorAll('button')].find((item) => item.textContent === 'Connect')
  try {
    root.render(
      <QueryClientProvider client={new QueryClient()}>
        <MessageMarkdown text={'Slack is not connected yet.\n\njaz://connect/slack'} />
      </QueryClientProvider>,
    )
    await until(() => Boolean(button()))
    const rect = button()!.getBoundingClientRect()
    const x = Math.round(rect.x + rect.width / 2)
    const y = Math.round(rect.y + rect.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
    await until(async () => (await window.smoke.openedURLs()).includes(authURL))
    await until(() => Boolean(element.textContent?.includes('Cambridge team') && element.textContent.includes('Add account')))
    if (starts !== 1) {
      throw new Error(`A connect card click started sign-in ${starts} times`)
    }
  } finally {
    root.unmount()
    element.remove()
    window.fetch = originalFetch
  }
}

async function until(check: () => boolean | Promise<boolean>): Promise<void> {
  const deadline = Date.now() + 5000
  while (!(await check())) {
    if (Date.now() > deadline) {
      throw new Error('Connect card: ' + check.toString())
    }
    await new Promise((resolve) => setTimeout(resolve, 20))
  }
}
