import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { ChatLog } from '@/components/bots/ChatLog'
import { FileReaderLinkProvider, MessageMarkdown, PreviewLinkProvider, RenderedMarkdown, UserMessageMarkdown } from '@/components/session/MessageMarkdown'
import { BrowserSettings } from '@/components/settings/BrowserSettings'
import { browserSettingsQuery } from '@/lib/api/settings'
import { skillsQuery } from '@/lib/api/skills'
import { clientRuntime } from '@/lib/clientRuntime'

export async function mountResponseLinks() {
  const previews: string[] = []
  const files: string[] = []
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData(browserSettingsQuery.queryKey, { enabled: false })
  client.setQueryData(skillsQuery().queryKey, [])
  const reply = '[Article](https://example.test/article) [Notes](/project/notes.ts:7) [Section](#section) [Canvas](/sessions/chat?page=overview)'
  function Fixture() {
    return <QueryClientProvider client={client}>
      <BrowserSettings />
      <PreviewLinkProvider onOpen={(url) => previews.push(url)}>
        <FileReaderLinkProvider sessionId="chat" onOpen={(file) => files.push(file.path)}>
          <div id="assistant"><MessageMarkdown text={reply} /></div>
          <div id="bot"><ChatLog entries={[{ kind: 'bot', key: 'reply', at: '2026-01-01T00:00:00Z', name: 'Bot', text: '[Report](http://example.test/report)' }]}
            bots={[]} named={false} working={[]} scrollRef={{ current: null }} paging={{ hasEarlier: false, loading: false, loadEarlier: async () => false }} /></div>
          <div id="user"><UserMessageMarkdown text="[My link](https://example.test/user)" /></div>
          <div id="document"><RenderedMarkdown text="[Source](https://example.test/document)" /></div>
        </FileReaderLinkProvider>
      </PreviewLinkProvider>
      <div id="without-preview"><MessageMarkdown text="[Site](https://example.test/fallback)" /></div>
    </QueryClientProvider>
  }
  const root = createRootRoute({ component: Fixture })
  const chat = createRoute({ getParentRoute: () => root, path: '/sessions/$sessionId' })
  const router = createRouter({ routeTree: root.addChildren([chat]), history: createMemoryHistory() })
  await router.load()
  flushSync(() => createRoot(document.getElementById('root')!).render(<RouterProvider router={router} />))
  const link = (id: string) => document.querySelector<HTMLAnchorElement>(`#${id} a[href^="http"]`)!
  // Observe React's decision, then suppress any native navigation in the fixture.
  function click(anchor: HTMLAnchorElement, init: MouseEventInit = {}, cancelled = false) {
    let handled = false
    window.addEventListener('click', (event) => { handled = event.defaultPrevented; event.preventDefault() }, { once: true })
    const event = new MouseEvent('click', { bubbles: true, cancelable: true, ...init })
    if (cancelled) event.preventDefault()
    anchor.dispatchEvent(event)
    return handled
  }
  return async (reloaded: boolean) => {
    const trigger = document.querySelector<HTMLButtonElement>('[aria-label="Open links in replies"]')!
    const external = clientRuntime.kind === 'electron' ? 'Default browser' : 'New browser tab'
    if (trigger.textContent !== (reloaded ? external : 'Built-in browser')) throw new Error('Unexpected saved link preference: ' + trigger.textContent)
    if (!reloaded && click(link('without-preview'))) throw new Error('A link without a side browser lost native navigation')
    const replies = [link('assistant'), link('bot')]
    if (!reloaded) {
      replies.forEach((anchor) => click(anchor))
      if (previews.join() !== replies.map((a) => a.href).join()) throw new Error('Default mode missed a reply link')
    }
    flushSync(() => trigger.click())
    const label = reloaded ? 'Built-in browser' : external
    const option = [...document.querySelectorAll<HTMLElement>('[role="option"]')].find((row) => row.textContent === label)
    if (!option) throw new Error('Missing browser choice: ' + label)
    flushSync(() => option.click())
    if (trigger.textContent !== label) throw new Error('Browser choice was not applied')
    previews.length = 0
    for (const anchor of replies) {
      if (!click(anchor)) throw new Error('Reply link was not handled')
      for (const modifiers of [{ ctrlKey: true }, { metaKey: true }, { altKey: true }, { shiftKey: true }, { button: 1 }]) {
        if (click(anchor, modifiers)) throw new Error('Modified link click was intercepted')
      }
      click(anchor, {}, true)
    }
    for (const id of ['user', 'document']) click(link(id))
    const expected = [...(reloaded ? replies : []), link('user'), link('document')].map((a) => a.href)
    if (previews.join() !== expected.join()) throw new Error('Unexpected side-browser destinations: ' + previews.join())
    document.querySelector<HTMLElement>('#assistant [role="link"]')!.click()
    if (files.join() !== '/project/notes.ts') throw new Error('File link missed its reader')
    if (!click(document.querySelector('#assistant a[href="#section"]')!)) throw new Error('Heading link missed its document')
    document.querySelector<HTMLAnchorElement>('#assistant a[href*="/sessions/chat"]')!.click()
    await router.load()
    if (router.state.location.href !== '/sessions/chat?page=overview') throw new Error('Canvas link missed its route')
    return reloaded ? [] : replies.map((a) => a.href)
  }
}
