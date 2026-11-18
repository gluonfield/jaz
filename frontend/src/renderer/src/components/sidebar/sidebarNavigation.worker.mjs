import { mock } from 'bun:test'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterContextProvider } from '@tanstack/react-router'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { keys } from '@/lib/query/keys'

globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) }
globalThis.localStorage ??= { getItem: () => null }
globalThis.document ??= { documentElement: { style: { setProperty() {}, removeProperty() {} }, classList: { add() {}, remove() {}, toggle() {} } } }
const reactDOM = await import('react-dom')
mock.module('react-dom', () => ({ ...reactDOM, createPortal: (children) => children }))

const { BotsPanel } = await import('@/components/bots/BotsPanel')
const { ChatPanel } = await import('@/components/sidebar/ChatPanel')

async function renderPanel(Panel, pathname, mobile) {
  const root = createRootRoute()
  const router = createRouter({
    routeTree: root.addChildren(['/new', '/bots', '/boards', '/loops'].map((path) => createRoute({ getParentRoute: () => root, path }))),
    history: createMemoryHistory({ initialEntries: [pathname] }),
  })
  await router.load()
  const client = new QueryClient()
  client.setQueryData(keys.bots, [])
  client.setQueryData(keys.mcpApps, [])
  client.setQueryData(keys.sidebarSessions, [])
  client.setQueryData(keys.projects, [])
  const html = renderToStaticMarkup(createElement(QueryClientProvider, { client },
    createElement(RouterContextProvider, { router },
      createElement(Panel, { mobile, open: true, onOpenCommandPalette() {} }))))
  client.clear()
  const links = [...html.matchAll(/<a\b[^>]*href="([^"]+)"[^>]*>(.*?)<\/a>/g)]
    .map(([, href, content]) => ({ href, text: content.replace(/<[^>]+>/g, '') }))
  return { html, links }
}

globalThis.postMessage({
  mobileBots: await renderPanel(BotsPanel, '/bots', true),
  desktopBots: await renderPanel(BotsPanel, '/bots', false),
  mobileChats: await renderPanel(ChatPanel, '/boards', true),
})
