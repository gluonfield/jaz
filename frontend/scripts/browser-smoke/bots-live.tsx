import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode, useEffect, useState } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BotsPanel } from '@/components/bots/BotsPanel'
import { NavRail } from '@/components/sidebar/NavRail'
import { ToastProvider } from '@/components/ui/toast'
import { botsQuery } from '@/lib/api/bots'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot } from '@/lib/api/types'
import { botMood } from '@/lib/bots'
import { connectRemote, disconnectBackend } from '@/lib/connection'
import { useBotEvents } from '@/lib/hooks/useBotEvents'

export async function exerciseBotsLive() {
  const backend = await window.smoke.backend()
  const previousBase = apiBaseUrl()
  const error = await connectRemote(backend)
  if (error) {
    throw new Error(error)
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:100;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  let reconnect = () => {}
  let disconnect = () => {}
  const timings: Record<string, number[]> = {}
  let listReads = 0
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    if (String(input) === backend + '/v1/bots') {
      listReads++
    }
    return originalFetch(input, init)
  }
  function Events() {
    useBotEvents()
    return null
  }
  function Fixture() {
    const [subscribed, setSubscribed] = useState(true)
    const { data = [] } = useQuery(botsQuery)
    useEffect(() => {
      disconnect = () => flushSync(() => setSubscribed(false))
      reconnect = () => {
        disconnect()
        flushSync(() => setSubscribed(true))
      }
    }, [])
    return <ToastProvider>{subscribed && <Events />}<div style={{ display: 'flex', height: '100vh' }}>
      <NavRail tab="/bots" onOpenChat={() => {}} onOpenSettings={() => {}} />
      <div style={{ width: 280 }}><BotsPanel mobile={false} /></div>
      <output aria-label="Observed mood">{data[0] ? botMood(data[0]) : 'loading'}</output>
    </div></ToastProvider>
  }
  const route = createRootRoute({ component: Fixture })
  const router = createRouter({ routeTree: route, history: createMemoryHistory({ initialEntries: ['/'] }) })
  const mood = () => element.querySelector('output')?.textContent
  const cheerful = () => [...element.querySelectorAll('[data-face="eye"]')].some((eye) => eye.getAttribute('d')?.includes('q6 -10'))
  const until = async (label: string, check: () => boolean, timeout = 2000) => {
    const start = performance.now()
    while (!check()) {
      if (performance.now() - start > timeout) {
        throw new Error(`Live bots: ${label}; mood=${mood()}; data=${JSON.stringify(client.getQueryData(botsQuery.queryKey))}`)
      }
      await new Promise(requestAnimationFrame)
    }
  }
  const step = async (name: string) => {
    const response = await fetch(backend + '/exercise/' + name, { method: 'POST' })
    if (!response.ok) {
      throw new Error(await response.text())
    }
    return response.json()
  }
  const change = async (name: string, expected: string) => {
    const start = performance.now()
    await step(name)
    await until(name, () => mood() === expected)
    const samples = timings[name] ??= []
    samples.push(Math.round(performance.now() - start))
  }
  try {
    root.render(<StrictMode><QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider></StrictMode>)
    await until('initial list', () => mood() === 'idle')
    if (cheerful()) {
      throw new Error('Live bots: cheered on initial load')
    }
    for (let run = 0; run < 3; run++) {
      await change('start', 'thinking')
      await change('work', 'working')
      await new Promise(requestAnimationFrame)
      await new Promise(requestAnimationFrame)
      await window.smoke.capture('bots-working')
      await change('finish', 'idle')
      await until('completion cheer', cheerful)
      await until('completion cheer ended', () => !cheerful(), 4000)
    }
    await change('ask?worker=1', 'waiting')
    if (!element.querySelector('[aria-label="Waiting for your answer"]')) {
      throw new Error('Worker question never reached the bot tile')
    }
    await new Promise(requestAnimationFrame)
    await new Promise(requestAnimationFrame)
    await window.smoke.capture('bots-waiting')
    await change('cancel?worker=1', 'idle')
    if (cheerful()) {
      throw new Error('A cancelled worker cheered')
    }
    const before = client.getQueryData<Bot[]>(botsQuery.queryKey)![0].completed_at
    await step('short')
    await until('short run completion', () => client.getQueryData<Bot[]>(botsQuery.queryKey)?.[0].completed_at !== before)
    await until('short run cheer', cheerful)
    await until('short run cheer ended', () => !cheerful(), 4000)
    await change('start', 'thinking')
    reconnect()
    await change('work', 'working')
    await change('finish', 'idle')
    await until('last cheer', cheerful)
    await until('last cheer ended', () => !cheerful(), 4000)
    await change('start', 'thinking')
    disconnect()
    await step('work')
    await step('finish')
    const deadline = performance.now() + 2000
    while ((await step('status')).status !== 'idle') {
      if (performance.now() > deadline) {
        throw new Error('Offline run did not finish')
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
    reconnect()
    await until('reconnected snapshot', () => mood() === 'idle')
    const reads = listReads
    const end = performance.now() + 16_000
    while (performance.now() < end) {
      if (cheerful()) {
        throw new Error('Live bots: cheer replayed after reconnect')
      }
      await new Promise(requestAnimationFrame)
    }
    if (listReads !== reads) {
      throw new Error(`Idle bot list polled ${listReads - reads} times`)
    }
    console.warn(JSON.stringify({ botLatencyMs: timings, idleListReads: listReads - reads }))
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    disconnectBackend()
    setApiBaseUrl(previousBase)
  }
}
