import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider } from '@tanstack/react-router'
import { useLayoutEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanel } from '@/components/session/SidePanel'
import { SidePanelDrawer } from '@/components/session/SidePanelDrawer'
import { SidePanelStateProvider, useSidePanelState } from '@/components/session/SidePanelState'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import { cachedApiAuthToken, apiBaseUrl, setApiAuthToken, setApiBaseUrl } from '@/lib/api/client'
import type { Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export async function exerciseBrowserVisibility(): Promise<void> {
  const backend = await window.smoke.backend()
  const primary = (await (await fetch(`${backend}/v1/sessions/tabs`)).json()).id as string
  const background = (await (await fetch(`${backend}/v1/sessions/browser-background`)).json()).id as string
  const originalURL = apiBaseUrl()
  const originalToken = cachedApiAuthToken(backend)
  setApiBaseUrl(backend)
  setApiAuthToken(backend, 'browser-smoke-credential')
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(keys.browserSettings, { enabled: true })
  client.setQueryData(keys.mcpApps, [])
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  let root = createRoot(element)
  const noop = async () => {}
  let panel: ReturnType<typeof useSidePanelState>
  function Chat({ id }: { id: string }) {
    const state = useSidePanelState(id)
    useLayoutEffect(() => {
      panel = state
    })
    const session: Session = { id, slug: id, status: 'idle', runtime: 'acp', created_at: '', updated_at: '', last_attention_at: '' }
    return <div ref={state.containerRef} data-visibility-chat={id} className="flex h-full flex-col">
      <header className="h-[52px] shrink-0">{id}</header>
      <div className="relative flex min-h-0 flex-1">
        <div className="min-w-0 flex-1">Conversation</div>
        <SidePanelDrawer panel={state} isMobile={false}>
          <SidePanel session={session} panel={state} subagents={[]} spawnedThreads={[]} working={false} sideChatAvailable={false} sideChatEvents={[]} onSend={noop} onQueuePrompt={noop} onQueueAction={noop} onSendSideChat={noop} overview={<div>Overview</div>} />
        </SidePanelDrawer>
      </div>
    </div>
  }
  const rootRoute = createRootRoute({ component: () => <BrowserWorkspace><SidePanelStateProvider><Outlet /></SidePanelStateProvider></BrowserWorkspace> })
  const chatRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/$id',
    component: () => {
      const { id } = chatRoute.useParams()
      return <Chat key={id} id={id} />
    },
  })
  const router = createRouter({ routeTree: rootRoute.addChildren([chatRoute]), history: createMemoryHistory({ initialEntries: ['/' + primary] }) })
  const action = async (id: string, input: { action: string; url?: string; code?: string }) => {
    const endpoint = input.action === 'script' ? `/exercise-script/${id}` : `/v1/sessions/${id}/browser`
    const response = await fetch(backend + endpoint, { method: 'POST', body: JSON.stringify(input) })
    if (!response.ok) {
      throw new Error(await response.text())
    }
    return response.json()
  }
  const shown = (id: string) => {
    const host = document.querySelector<HTMLElement>(`[data-browser-session="${id}"]`)
    return element.querySelector<HTMLElement>('[data-visibility-chat]')?.dataset.visibilityChat === id && panel?.open && panel.mode === 'tabs' && panel.activeTab?.id === id && host && !host.inert && host.getBoundingClientRect().width > 240
  }
  const view = (id: string) => document.querySelector<PreviewWebviewElement>(`[data-browser-session="${id}"] webview`)
  try {
    root.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
    await until(async () => (await fetch(`${backend}/v1/sessions/${primary}/browser`, { method: 'POST', body: '{"action":"status"}' })).ok)
    if (panel!.open || view(primary)) {
      throw new Error('Browser fixture did not start closed')
    }
    await action(background, { action: 'navigate', url: `${location.origin}/target?unvisited` })
    if (panel!.open || !view(background)) {
      throw new Error('An unvisited conversation could not open its own browser in the background')
    }
    root.unmount()
    root = createRoot(element)
    root.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
    await until(() => Boolean(element.querySelector('[data-visibility-chat]')))
    await until(async () => {
      try {
        await action(background, { action: 'tabs' })
        return true
      } catch {
        return false
      }
    })
    await action(background, { action: 'navigate', url: `${location.origin}/target?after-restart` })
    if (panel!.open || !view(background)) {
      throw new Error('Restarting the desktop left the background browser disconnected')
    }
    const backgroundPage = view(background)!.getWebContentsId()
    await action(primary, { action: 'navigate', url: `${location.origin}/target?visibility` })
    await until(() => Boolean(shown(primary) && view(primary)))
    const first = view(primary)!.getWebContentsId()
    for (const input of [{ action: 'state' }, { action: 'script', code: 'await tab.getAXState()' }]) {
      panel!.close()
      await until(() => !panel!.open)
      await action(primary, input)
      await until(() => Boolean(shown(primary)))
      if (view(primary)!.getWebContentsId() !== first) {
        throw new Error('Revealing a browser replaced the page')
      }
    }
    await Promise.allSettled(element.getAnimations({ subtree: true }).filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished))
    await window.smoke.capture('agent-browser-reveals-panel')
    panel!.close()
    await until(() => !panel!.open)
    await router.navigate({ to: '/$id', params: { id: background } })
    await until(() => Boolean(shown(background)))
    if (view(background)!.getWebContentsId() !== backgroundPage) {
      throw new Error('Visiting the background conversation replaced its browser')
    }
    panel!.close()
    await until(() => !panel!.open)
    await action(primary, { action: 'state' })
    if (panel!.open) {
      throw new Error('Background browsing opened a different conversation’s panel')
    }
    await router.navigate({ to: '/$id', params: { id: primary } })
    await until(() => Boolean(shown(primary)))
    if (view(primary)!.getWebContentsId() !== first) {
      throw new Error('Returning to the owning conversation replaced the page')
    }
    await Promise.allSettled(element.getAnimations({ subtree: true }).filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished))
    await window.smoke.capture('agent-browser-reveals-on-return')
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    setApiAuthToken(backend, originalToken)
    setApiBaseUrl(originalURL)
  }
}

async function until(check: () => boolean | Promise<boolean>): Promise<void> {
  const deadline = Date.now() + 7000
  while (!await check()) {
    if (Date.now() > deadline) {
      throw new Error('Browser visibility did not settle: ' + check.toString())
    }
    await new Promise((resolve) => setTimeout(resolve, 20))
  }
}
