import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import type { Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { Route as listRoute } from '@/routes/loops.index'
import { Route as detailRoute } from '@/routes/loops.$loopId'

export async function exerciseLoops(): Promise<void> {
  const reason = "Waiting until you write to Jaz again. You have been away for 3 days."
  const routine: Loop = {
    id: 'inbox', name: 'Inbox', prompt: 'Check incoming messages and flag replies that need attention.', bot_id: 'helper',
    schedule: { kind: 'cron', expr: '0 9 * * *', timezone: 'Europe/London' }, when_missed: 'skip', thread: 'main',
    runtime: 'acp', status: 'active', created_at: new Date().toISOString(), updated_at: new Date().toISOString(), blocked: reason,
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const path = new URL(String(input), location.href).pathname
    if (path === '/v1/loops') {
      return Response.json({ loops: [routine, { ...routine, id: 'monitor', name: 'Monitor', blocked: undefined }] })
    }
    if (path === '/v1/loops/inbox') {
      return Response.json({ loop: routine, runs: [], board_ids: [] })
    }
    if (path === '/v1/boards') {
      return Response.json({ boards: [] })
    }
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:50;overflow:auto'
  element.className = 'bg-bg'
  document.body.append(element)
  const root = createRoot(element)
  const rootRoute = createRootRoute()
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      listRoute.update({ getParentRoute: () => rootRoute, id: '/loops/', path: '/loops/' }),
      detailRoute.update({ getParentRoute: () => rootRoute, id: '/loops/$loopId', path: '/loops/$loopId' }),
    ]),
    history: createMemoryHistory({ initialEntries: ['/loops/'] }),
  })
  const until = async (label: string, check: () => boolean) => {
    const deadline = Date.now() + 5000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error(`Loops: ${label}: ${element.textContent}`)
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const visibleReason = () => [...element.querySelectorAll('p')].filter((node) => node.textContent === reason && node.getBoundingClientRect().height > 0)
  try {
    root.render(<StrictMode><QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider></StrictMode>)
    await until('list shows one waiting reason', () => visibleReason().length === 1 && element.textContent!.includes('Monitor'))
    await until('list waiting reason uses secondary text', () => getComputedStyle(visibleReason()[0]).color === getComputedStyle(element.querySelector('span.text-ink-2')!).color)
    await window.smoke.capture('loops-waiting-list')
    const link = element.querySelector<HTMLAnchorElement>('a[href="/loops/inbox"]')!
    const bounds = link.getBoundingClientRect()
    await window.smoke.pointer('mouseDown', bounds.x + 30, bounds.y + bounds.height / 2)
    await window.smoke.pointer('mouseUp', bounds.x + 30, bounds.y + bounds.height / 2)
    await until('details show the waiting reason', () => element.textContent!.includes('All loops') && visibleReason().length === 1)
    await until('details waiting reason uses secondary text', () => getComputedStyle(visibleReason()[0]).color === getComputedStyle(visibleReason()[0].previousElementSibling!).color)
    await window.smoke.capture('loops-waiting-detail')
    routine.blocked = undefined
    await client.invalidateQueries({ queryKey: keys.loopDetail(routine.id) })
    await until('cleared reason disappears from details', () => visibleReason().length === 0)
    await router.navigate({ to: '/loops' })
    await client.invalidateQueries({ queryKey: keys.loops })
    await until('cleared reason disappears from list', () => element.textContent!.includes('Monitor') && visibleReason().length === 0)
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
  }
}
