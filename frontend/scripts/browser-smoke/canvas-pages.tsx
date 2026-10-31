import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider, useLocation } from '@tanstack/react-router'
import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Composer } from '@/components/session/Composer'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'
import { ThreadCanvas } from '@/components/canvas/ThreadCanvas'
import { useCanvasContexts } from '@/components/canvas/useCanvasContexts'
import { FileReaderLinkProvider, MessageMarkdown } from '@/components/session/MessageMarkdown'
import { ToastProvider } from '@/components/ui/toast'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { getCanvas, patchCanvas } from '@/lib/api/canvas'
import { keys } from '@/lib/query/keys'
import { Route as SessionRoute } from '@/routes/sessions.$sessionId'

export async function exerciseCanvasPages(): Promise<void> {
  const sessionId = 'canvas-pages'
  const backend = await window.smoke.backend()
  const before = apiBaseUrl()
  setApiBaseUrl(backend)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const submitted: SendMessageOptions[] = []
  let setLinkPage: (page: string) => void
  let setInset: (inset: number) => void
  const rootRoute = createRootRoute({ component: () => <ToastProvider><Outlet /></ToastProvider> })
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: '/sessions/$sessionId',
    validateSearch: SessionRoute.options.validateSearch,
    component: Fixture,
  })
  const router = createRouter({ routeTree: rootRoute.addChildren([route]), history: createMemoryHistory({ initialEntries: ['/sessions/canvas-pages'] }) })
  function Fixture() {
    const search = route.useSearch()
    const navigationKey = useLocation({ select: (location) => location.state.__TSR_key })
    const picks = useCanvasContexts({ contexts: [], removeContext: () => {}, replaceContexts: () => {} })
    const [linkPage, changeLinkPage] = useState('')
    const [inset, changeInset] = useState(0)
    setLinkPage = changeLinkPage
    setInset = changeInset
    return <>
      <ThreadCanvas sessionId={sessionId} inset={inset} working={false} bindSelection={picks.bindSelection} onSelect={picks.onSelect} onPage={picks.onPage} navigationKey={navigationKey} onPrompt={() => {}} page={search.page} item={search.item} />
      <div data-canvas-fixture-controls style={{ position: 'absolute', right: 16, top: 70, zIndex: 10, maxWidth: 220 }}>
        <FileReaderLinkProvider sessionId={sessionId} onOpen={() => {}}>
          <MessageMarkdown text={`[Go to item](/sessions/${sessionId}?page=${encodeURIComponent(linkPage)}&item=same)\n\n[Current thread item](?page=${encodeURIComponent(linkPage)}&item=same)\n\n[Missing page](/sessions/${sessionId}?item=same)\n\n[Unknown page](/sessions/${sessionId}?page=missing&item=same)\n\n[Unknown item](/sessions/${sessionId}?page=${encodeURIComponent(linkPage)}&item=missing)`} />
        </FileReaderLinkProvider>
        <output data-selection>{JSON.stringify(picks.contexts)}</output>
        <Composer streaming={false} contexts={picks.contexts} onRemoveContext={picks.removeContext} onReplaceContexts={picks.replaceContexts} onSend={(_text, options) => { submitted.push(options!) }} />
      </div>
    </>
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:2;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean | Promise<boolean>, message: string | (() => string)) => {
    const deadline = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > deadline) throw new Error('Canvas pages: ' + (typeof message === 'function' ? message() : message))
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const node = () => element.querySelector<HTMLElement>('.react-flow__node[data-id="same"]')
  const control = () => element.querySelector<HTMLButtonElement>('[aria-label="Canvas page"]')!
  const inView = () => {
    const box = node()?.getBoundingClientRect()
    const inset = Number.parseFloat(getComputedStyle(element.querySelector('.canvas')!).getPropertyValue('--canvas-inset'))
    return !!box && box.left >= inset && box.top >= 56 && box.right <= window.innerWidth && box.bottom <= window.innerHeight - 96
  }
  const transform = () => element.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform
  const raw = async () => (await (await fetch(backend + '/exercise-canvas')).json()).content as string
  const choose = async (name: string) => {
    control().click()
    await until(() => Boolean(document.querySelector('[role="option"]')), 'Page menu did not open')
    const option = [...document.querySelectorAll<HTMLButtonElement>('[role="option"]')].find((option) => option.textContent === name)!
    if (!option) throw new Error('Canvas pages: no option ' + name)
    option.click()
    await until(() => control().textContent === name, 'Page switch did not finish')
  }
  const link = (label: string) => [...element.querySelectorAll<HTMLAnchorElement>('a')].find((link) => link.textContent === label)!
  try {
    await router.load()
    root.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
    await until(() => Boolean(node()?.textContent?.includes('Legacy page')), 'Legacy content is not visible on Page 1')
    if (control().textContent !== 'Page 1' || JSON.parse(await raw()).pages) throw new Error('Legacy read changed disk schema or page name')
    await until(() => {
      const contexts = JSON.parse(element.querySelector('[data-selection]')!.textContent!)
      return contexts.length === 1 && contexts[0].page === 'page-1' && !contexts[0].node
    }, 'Empty selection did not identify Page 1')
    const send = element.querySelector<HTMLButtonElement>('[aria-label="Send message"]')!
    if (!send.disabled) throw new Error('An implicit page context enabled an empty message')
    const draft = element.querySelector<HTMLTextAreaElement>('textarea')!
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(draft, 'Add here')
    draft.dispatchEvent(new Event('input', { bubbles: true }))
    await until(() => !send.disabled, 'Text with active-page context could not be sent')
    send.click()
    await until(() => submitted.length === 1, 'Active-page submission did not finish')
    const sentLocation = preparedSendMessage(submitted[0]).contexts[0]
    if (sentLocation.type !== 'canvas' || sentLocation.page !== 'page-1' || sentLocation.id) throw new Error('Submission lost its page-only location')
    element.querySelector<HTMLButtonElement>('[aria-label="Add page"]')!.click()
    await until(() => control().textContent === 'Page 2', 'Adding Page 2 did not select it')
    if (node()) throw new Error('Page 1 item leaked onto Page 2')
    const document = await getCanvas(sessionId)
    const second = document.pages[1].id
    setLinkPage!(second)
    await until(() => {
      const contexts = JSON.parse(element.querySelector('[data-selection]')!.textContent!)
      return contexts.length === 1 && contexts[0].page === second && !contexts[0].node
    }, 'Switching pages retained stale empty-selection context')
    if (preparedSendMessage(submitted[0]).contexts[0].page !== 'page-1') throw new Error('Page switching changed a submitted context snapshot')
    const stored = JSON.parse(await raw())
    if (stored.pages[0].nodes[0].text !== 'Legacy page' || stored.pages.length !== 2) throw new Error('Creating a page lost legacy content')
    element.querySelector<HTMLButtonElement>('[aria-label="Rename page"]')!.click()
    await until(() => Boolean(element.querySelector('[aria-label="Page name"]')), 'Rename field did not open')
    const input = element.querySelector<HTMLInputElement>('[aria-label="Page name"]')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'Costs')
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await new Promise((resolve) => setTimeout(resolve, 20))
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await until(async () => JSON.parse(await raw()).pages[1].name === 'Costs', 'Renaming did not persist')
    await patchCanvas(sessionId, { page: second, nodes: [{ id: 'same', type: 'text', text: 'Second page', x: 900, y: 500, width: 240, height: 160 }] })
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    await until(() => Boolean(node()?.textContent?.includes('Second page')), 'Page-local item did not load')
    const agentDocument = JSON.parse(await raw())
    agentDocument.pages[0].name = 'Overview'
    agentDocument.pages[1].name = 'Agent plan'
    agentDocument.pages[0].nodes[0].text = 'Background update'
    const edited = await fetch(backend + '/exercise-canvas', { method: 'POST', body: JSON.stringify({ content: JSON.stringify(agentDocument) }) })
    if (!edited.ok) throw new Error(await edited.text())
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    await until(() => control().textContent === 'Agent plan', 'Agent rename did not reach the page controls')
    if (!node()?.textContent?.includes('Second page')) throw new Error('Agent edits switched the selected page')
    await choose('Overview')
    await until(() => Boolean(node()?.textContent?.includes('Background update')), 'Switching pages showed the wrong duplicate item id')
    element.querySelector<HTMLButtonElement>('[aria-label="Hand"]')!.click()
    const original = transform()
    await window.smoke.pointer('mouseDown', 600, 350)
    await window.smoke.pointer('mouseMove', 650, 390)
    await window.smoke.pointer('mouseUp', 650, 390)
    await until(() => transform() !== original, 'Page viewport did not pan')
    await until(() => !element.querySelector('.canvas-busy'), 'Viewport pan did not finish')
    const firstViewport = transform()
    await choose('Agent plan')
    await until(() => Boolean(node()?.textContent?.includes('Second page')), 'Second page was not restored')
    await choose('Overview')
    await until(() => transform() === firstViewport, () => `The first page viewport was not retained: expected ${firstViewport}, got ${transform()}`)
    setInset!(360)
    await until(() => Number.parseFloat(getComputedStyle(element.querySelector('.canvas')!).getPropertyValue('--canvas-inset')) === 360, 'Chat inset did not settle')
    link('Go to item').click()
    await until(() => control().textContent === 'Agent plan' && Boolean(node()?.classList.contains('selected')) && inView(), 'Page/item link did not switch to and select its item')
    const selected = JSON.parse(element.querySelector('[data-selection]')!.textContent!)[0]
    if (selected.page !== second || selected.node !== 'same') throw new Error('Selection context lost page identity')
    element.querySelector<HTMLButtonElement>('[aria-label^="Remove second page"]')!.click()
    await until(() => !node()?.classList.contains('selected'), 'Removing the context did not deselect its page item')
    await choose('Overview')
    await until(() => Boolean(node()?.textContent?.includes('Background update')), 'User page switch did not finish')
    await until(() => transform() === firstViewport, 'Manual switch did not restore its viewport')
    const userViewport = transform()
    const later = JSON.parse(await raw())
    later.pages[1].nodes[0].text = 'Later agent edit'
    const laterWrite = await fetch(backend + '/exercise-canvas', { method: 'POST', body: JSON.stringify({ content: JSON.stringify(later) }) })
    if (!laterWrite.ok) throw new Error(await laterWrite.text())
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    if (control().textContent !== 'Overview' || transform() !== userViewport) throw new Error('A consumed item link overrode user page/viewport on an agent edit')
    link('Go to item').click()
    await until(() => control().textContent === 'Agent plan' && inView(), 'A repeated deliberate item link was ignored')
    link('Missing page').click()
    await until(() => Boolean(element.querySelector('[role="alert"]')?.textContent?.includes('must specify its page')), 'An item-only link was not rejected')
    if (control().textContent !== 'Agent plan' || !node()?.classList.contains('selected')) throw new Error('Invalid navigation changed the page or selection')
    link('Unknown page').click()
    await until(() => Boolean(element.querySelector('[role="alert"]')?.textContent?.includes('was not found')), 'An unknown page was not rejected')
    link('Unknown item').click()
    await until(() => Boolean(element.querySelector('[role="alert"]')?.textContent?.includes('Item')), 'An unknown item was not rejected')
    if (control().textContent !== 'Agent plan' || !node()?.classList.contains('selected')) throw new Error('Unknown item navigation changed the page or selection')
    await choose('Overview')
    setInset!(0)
    link('Current thread item').click()
    await until(() => control().textContent === 'Agent plan' && Boolean(node()?.classList.contains('selected')) && inView(), 'Current-thread page/item link did not navigate')
    element.querySelector<HTMLElement>('[data-canvas-fixture-controls]')!.hidden = true
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    await window.smoke.capture('canvas-pages-desktop')
    await window.smoke.resize(430, 860)
    setInset!(340)
    await until(() => window.innerWidth === 430, 'Narrow canvas resize did not settle')
    const controls = element.querySelector<HTMLElement>('.canvas-pages')!.getBoundingClientRect()
    if (controls.left < 0 || controls.right > window.innerWidth) throw new Error('Page controls overflow the narrow canvas')
    setInset!(0)
    await until(() => element.querySelector<HTMLElement>('.canvas')!.style.getPropertyValue('--canvas-inset') === '0px', 'Narrow inset did not settle')
    element.querySelector<HTMLButtonElement>('button[title="Fit everything"]')!.click()
    await until(inView, 'Narrow page target is outside the viewport')
    await window.smoke.capture('canvas-pages-narrow')
  } finally {
    await window.smoke.resize(1050, 850)
    root.unmount()
    client.clear()
    element.remove()
    setApiBaseUrl(before)
  }
}
