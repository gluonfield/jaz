import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { useState } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanelStateProvider } from '@/components/session/SidePanelState'
import { ThreadView } from '@/components/session/ThreadView'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import { sessionFileQuery } from '@/lib/api/sessions'
import type { PluginView, SessionMessages } from '@/lib/api/types'
import { SidePanelSlot } from '@/lib/sidePanelSlot'
import { keys } from '@/lib/query/keys'
import { TitlebarActionsOutlet, TitlebarProvider } from '@/lib/titlebar'
import { VoiceProvider } from '@/lib/voice/VoiceProvider'

export async function exercisePluginViews(): Promise<void> {
  const id = 'saved-plugin-view'
  const at = '2026-11-17T10:00:00Z'
  const view: PluginView = { reference: 'example/status', title: 'Progress', file: 'status.json', widget_code: '<h1>Stored plugin panel</h1><p id="value"></p><script>document.getElementById("value").textContent=window.data.title</script>' }
  const page: SessionMessages = { session: { id, slug: id, runtime: 'acp', status: 'idle', created_at: at, updated_at: at }, messages: [{ seq: 950, role: "user", content: "Recent work", blocks: [], created_at: at }], events: [], has_earlier: true, before_event_seq: 900, latest_event_seq: 999, history_revision: 1 }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  client.setQueryData(keys.computerSettings, { enabled: false })
  client.setQueryData(keys.sessionOverview(id), { threads: [], subagents: [], plugin_views: [view] })
  client.setQueryData(keys.sessionRepo(id), { git: false })
  client.setQueryData(sessionFileQuery(id, view.file).queryKey, { path: '/work/status.json', content: '{"title":"Saved state"}', size: 23 })
  const originalFetch = window.fetch
  let replayed = false
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname === `/v1/sessions/${id}/messages`) return Response.json(url.searchParams.has('before_event_seq') ? { ...page, before_event_seq: 600, messages: [{ ...page.messages[0], seq: 600 }] } : page)
    if (url.pathname === `/v1/sessions/${id}/events`) {
      replayed = url.searchParams.get('after_seq') !== '999'
      return new Response('', { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.pathname.endsWith('/seen')) return Response.json({ ok: true })
    return new Response(null, { status: 503 })
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  function Shell() {
    const [row, setRow] = useState<HTMLDivElement | null>(null)
    const [slot, setSlot] = useState<HTMLDivElement | null>(null)
    const [shown, setShown] = useState(true)
    return <TitlebarProvider><div className="flex h-10 justify-end"><button onClick={() => setShown((value) => !value)}>Switch thread</button><TitlebarActionsOutlet /></div><BrowserWorkspace><SidePanelStateProvider><SidePanelSlot value={{ row, slot }}><div ref={setRow} className="flex h-[calc(100%_-_40px)]"><div className="min-w-0 flex-1">{shown ? <ThreadView sessionId={id} /> : <p>Another thread</p>}</div><div ref={setSlot} className="contents" /></div></SidePanelSlot></SidePanelStateProvider></BrowserWorkspace></TitlebarProvider>
  }
  const route = createRootRoute({ component: Shell })
  const router = createRouter({ routeTree: route, history: createMemoryHistory({ initialEntries: ['/'] }) })
  const mount = () => flushSync(() => root.render(<QueryClientProvider client={client}><VoiceProvider><RouterProvider router={router} /></VoiceProvider></QueryClientProvider>))
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 4000
    while (!check()) {
      if (Date.now() > deadline) throw new Error('Plugin view: ' + check.toString() + ' · ' + element.textContent + ' · ' + JSON.stringify(client.getQueryData(keys.sessionOverview(id))))
      await new Promise(requestAnimationFrame)
    }
  }
  try {
    mount()
    await until(() => Boolean(element.querySelector('iframe[title="Progress"]')))
    if (replayed || element.textContent?.includes('View: Progress')) throw new Error('Restoration depended on transcript/event replay')
    await window.smoke.capture('plugin-views-restored')
    element.querySelector<HTMLButtonElement>('[aria-label="Close Progress"]')!.click()
    await until(() => !element.querySelector('[role="tab"]'))
    element.querySelector<HTMLButtonElement>('[aria-label="Plugin views"]')!.click()
    await until(() => Boolean(document.querySelector('[role="menuitem"]')))
    document.querySelector<HTMLButtonElement>('[role="menuitem"]')!.click()
    await until(() => element.querySelectorAll('iframe[title="Progress"]').length === 1)
    const switchThread = [...element.querySelectorAll('button')].find((button) => button.textContent === 'Switch thread')!
    switchThread.click()
    await until(() => element.textContent?.includes('Another thread') === true)
    switchThread.click()
    await until(() => element.querySelectorAll('iframe[title="Progress"]').length === 1)
    flushSync(() => root.render(null))
    mount()
    await until(() => element.querySelectorAll('iframe[title="Progress"]').length === 1)
    await window.smoke.capture('plugin-views-reopened')
  } finally {
    root.unmount()
    element.remove()
    client.clear()
    window.fetch = originalFetch
  }
}
