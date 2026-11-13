import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider, useRouterState } from '@tanstack/react-router'
import { StrictMode, useState } from 'react'
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
    const activeSessionId = route.useParams().sessionId
    const search = route.useSearch()
    const navigationKey = useRouterState({ select: (state) => state.resolvedLocation?.state.__TSR_key })
    const picks = useCanvasContexts({ contexts: [], removeContext: () => {}, replaceContexts: () => {} })
    const [linkPage, changeLinkPage] = useState('')
    const [inset, changeInset] = useState(0)
    setLinkPage = changeLinkPage
    setInset = changeInset
    return <>
      <header data-canvas-titlebar style={{ position: 'absolute', inset: '0 0 auto', height: 'var(--titlebar-height)', zIndex: 20, background: 'var(--color-bg)' }}><button type="button" aria-label="Canvas fixture navigation" style={{ width: 200, height: '100%' }}>Back · Forward</button></header>
      <ThreadCanvas sessionId={activeSessionId} inset={inset} working={false} bindSelection={picks.bindSelection} onSelect={picks.onSelect} onPage={picks.onPage} navigationKey={navigationKey} onPrompt={() => {}} page={search.page} item={search.item} />
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
  element.style.cssText = 'position:fixed;inset:0;z-index:2;background:var(--color-bg);--titlebar-height:40px'
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
  const control = () => element.querySelector<HTMLButtonElement>('.canvas-pages [aria-current="page"]')!
  const tab = (name: string) => [...element.querySelectorAll<HTMLButtonElement>('.canvas-pages [data-page]')].find((tab) => tab.textContent === name)
  const action = (name: string) => [...document.querySelectorAll<HTMLButtonElement>('[role="menu"] button')].find((button) => button.textContent === name)!
  const openActions = async () => {
    const rect = control().getBoundingClientRect()
    await window.smoke.pointer('mouseDown', rect.x + rect.width / 2, rect.y + rect.height / 2, 'right')
    await window.smoke.pointer('mouseUp', rect.x + rect.width / 2, rect.y + rect.height / 2, 'right')
    await until(() => Boolean(action('Rename page') && action('Delete page')), 'Right-click did not open page actions')
  }
  const checkControls = () => {
    const box = control().getBoundingClientRect()
    const titlebar = element.querySelector('[data-canvas-titlebar]')!.getBoundingClientRect()
    if (box.top < titlebar.bottom || !control().contains(document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2))) throw new Error(`Canvas pages: titlebar covers the page selector (titlebar bottom ${titlebar.bottom}, selector top ${box.top})`)
  }
  const inView = () => {
    const box = node()?.getBoundingClientRect()
    const inset = Number.parseFloat(getComputedStyle(element.querySelector('.canvas')!).getPropertyValue('--canvas-inset'))
    return !!box && box.left >= inset && box.top >= 56 && box.right <= window.innerWidth && box.bottom <= window.innerHeight - 96
  }
  const transform = () => element.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform
  const raw = async () => (await (await fetch(backend + '/exercise-canvas')).json()).content as string
  const write = async (document: unknown) => {
    const response = await fetch(backend + '/exercise-canvas', { method: 'POST', body: JSON.stringify({ content: JSON.stringify(document) }) })
    if (!response.ok) throw new Error(await response.text())
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
  }
  const choose = async (name: string) => {
    await until(() => Boolean(tab(name)), 'No tab ' + name)
    tab(name)!.click()
    await until(() => control().textContent === name, 'Page switch did not finish')
  }
  const remount = async (key: string, name: string) => {
    const canvas = element.querySelector('.canvas')
    root.render(<StrictMode><QueryClientProvider key={key} client={client}><RouterProvider router={router} /></QueryClientProvider></StrictMode>)
    await until(() => element.querySelector('.canvas') !== canvas && control()?.textContent === name, `Remount (${key}) did not restore ${name}`)
  }
  const link = (label: string) => [...element.querySelectorAll<HTMLAnchorElement>('a')].find((link) => link.textContent === label)!
  try {
    await router.load()
    root.render(<StrictMode><QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider></StrictMode>)
    await until(() => Boolean(node()?.textContent?.includes('Legacy page')), 'Legacy content is not visible on Page 1')
    checkControls()
    if (element.querySelector('[aria-label="Rename page"]')) throw new Error('Canvas pages: persistent edit button is still present')
    await openActions()
    if (!action('Delete page').disabled || !document.querySelector('[role="menu"]')?.textContent?.includes('Keep at least one page')) throw new Error('Canvas pages: final-page deletion is not visibly disabled')
    await window.smoke.key('Escape')
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
    const created = await getCanvas(sessionId)
    const second = created.pages[1].id
    setLinkPage!(second)
    await until(() => {
      const contexts = JSON.parse(element.querySelector('[data-selection]')!.textContent!)
      return contexts.length === 1 && contexts[0].page === second && !contexts[0].node
    }, 'Switching pages retained stale empty-selection context')
    if (preparedSendMessage(submitted[0]).contexts[0].page !== 'page-1') throw new Error('Page switching changed a submitted context snapshot')
    const stored = JSON.parse(await raw())
    if (stored.pages[0].nodes[0].text !== 'Legacy page' || stored.pages.length !== 2) throw new Error('Creating a page lost legacy content')
    await openActions()
    action('Rename page').click()
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
    await write(agentDocument)
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
    await write(later)
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
    for (const id of ['constructor', '__proto__']) {
      await patchCanvas(sessionId, { page: id, create: true, name: id, nodes: [{ id: 'same', type: 'text', text: 'Reserved page', x: 900, y: 500, width: 240, height: 160 }] })
      await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
      await choose(id)
      await until(() => node()?.textContent?.includes('Reserved page') === true && inView(), `Page ${id} did not fit its initial viewport`)
      const viewport = transform()
      const rect = node()!.getBoundingClientRect()
      await window.smoke.pointer('mouseDown', rect.x + rect.width / 2, rect.y + rect.height / 2)
      await window.smoke.pointer('mouseUp', rect.x + rect.width / 2, rect.y + rect.height / 2)
      await until(() => Boolean(node()?.classList.contains('selected')), `Page ${id} item was not selected`)
      await window.smoke.key('Right')
      await until(async () => JSON.parse(await raw()).pages.find((page: { id: string }) => page.id === id).nodes[0].x === 901, `Page ${id} edit did not persist`)
      await window.smoke.key('z', ['meta'])
      await until(async () => JSON.parse(await raw()).pages.find((page: { id: string }) => page.id === id).nodes[0].x === 900, `Page ${id} undo did not restore its item`)
      await choose('Overview')
      await until(() => transform() === firstViewport, `Page ${id} changed the first page viewport`)
      await choose(id)
      await until(() => transform() === viewport, `Page ${id} did not retain its viewport`)
    }
    await remount('consumed-link', '__proto__')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: {} })
    await remount('reload', '__proto__')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId: 'canvas-pages-other' }, search: {} })
    await until(() => control()?.textContent === 'Page 1', 'Another thread inherited the previous thread page')
    const outdated = await getCanvas(sessionId)
    client.setQueryData(keys.sessionCanvas(sessionId), { ...outdated, version: 'outdated', pages: outdated.pages.filter((page) => page.id !== '__proto__') })
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: {} })
    await until(() => control()?.textContent === '__proto__', 'A stale cached document lost the remembered page before refresh')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId: 'canvas-pages-other' }, search: {} })
    await until(() => control()?.textContent === 'Page 1', 'The other thread did not reopen')
    client.setQueryData(keys.sessionCanvas(sessionId), { ...outdated, version: 'outdated', pages: outdated.pages.filter((page) => page.id !== '__proto__') })
    const originalFetch = window.fetch
    window.fetch = (input, init) => new URL(String(input)).pathname === `/v1/sessions/${sessionId}/canvas`
      ? Promise.resolve(new Response('Canvas temporarily unavailable', { status: 503 }))
      : originalFetch(input, init)
    try {
      await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: {} })
      await until(() => client.getQueryState(keys.sessionCanvas(sessionId))?.status === 'error', 'The refresh failure did not reach the canvas query')
      await router.navigate({ to: '/sessions/$sessionId', params: { sessionId: 'canvas-pages-other' }, search: {} })
      await until(() => control()?.textContent === 'Page 1', 'The failed refresh prevented leaving the thread')
    } finally {
      window.fetch = originalFetch
    }
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: {} })
    await until(() => control()?.textContent === '__proto__', 'A failed refresh erased the remembered page')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId: 'canvas-pages-other' }, search: {} })
    await until(() => control()?.textContent === 'Page 1', 'The other thread did not reopen before the explicit link')
    client.setQueryData(keys.sessionCanvas(sessionId), { ...outdated, version: 'outdated', pages: outdated.pages.filter((page) => page.id !== 'page-1') })
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: { page: 'page-1', item: 'same' } })
    await until(() => control()?.textContent === 'Overview' && Boolean(node()?.classList.contains('selected')), 'A fresh item link to an uncached page was consumed before refresh')
    await remount('explicit-page', 'Overview')
    const originalDocument = JSON.parse(await raw())
    const reordered = JSON.parse(JSON.stringify(originalDocument))
    reordered.pages = [...reordered.pages.slice(2), ...reordered.pages.slice(0, 2)]
    reordered.pages.find((page: { id: string }) => page.id === 'page-1').name = 'Pinned overview'
    await write(reordered)
    await until(() => control().textContent === 'Pinned overview', 'Reordering pages switched the default page')
    reordered.pages = reordered.pages.filter((page: { id: string }) => page.id !== 'page-1')
    reordered.pages[0].name = 'Fallback page'
    await write(reordered)
    await until(() => control().textContent === 'Fallback page', 'Deleting the active page did not choose a fallback')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: {} })
    await remount('deleted-page', 'Fallback page')
    reordered.pages[0].name = 'Retained fallback'
    reordered.pages.unshift(originalDocument.pages[0])
    await write(reordered)
    await until(() => control().textContent === 'Retained fallback', 'Restoring a deleted page overrode the fallback choice')
    await write(originalDocument)
    await choose('Agent plan')
    await until(inView, 'The navigated page did not retain its viewport')
    element.querySelector<HTMLElement>('[data-canvas-fixture-controls]')!.hidden = true
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    checkControls()
    await window.smoke.capture('canvas-pages-desktop')
    const tabs = [...element.querySelectorAll<HTMLElement>('.canvas-pages [data-page]')]
    const pageNames = JSON.parse(await raw()).pages.map((page: { name: string }) => page.name)
    if (tabs.length < 2 || tabs.map((tab) => tab.textContent).join() !== pageNames.join() || tabs.some((tab, index) => index > 0 && tab.getBoundingClientRect().left < tabs[index - 1].getBoundingClientRect().right)) throw new Error(`Canvas pages: tabs ${tabs.map((tab) => tab.textContent).join()} are not every page left to right (${pageNames.join()})`)
    await openActions()
    await new Promise((resolve) => setTimeout(resolve, 150))
    await window.smoke.capture('canvas-pages-actions')
    await choose('Overview')
    if (document.querySelector('[role="menu"]')) throw new Error('Canvas pages: switching pages retained actions for the old page')
    tab('Agent plan')!.dispatchEvent(new MouseEvent('contextmenu', { clientX: 100, clientY: 100, bubbles: true, cancelable: true }))
    await until(() => document.querySelector('[role="menu"]')?.getAttribute('aria-label') === 'Agent plan actions', 'Right-clicking another tab did not open that page\'s actions')
    if (control().textContent !== 'Overview') throw new Error('Canvas pages: right-clicking another tab switched pages')
    await window.smoke.key('Escape')
    await until(() => !document.querySelector('[role="menu"]') && document.activeElement === tab('Agent plan'), 'Closing another tab\'s actions did not return focus to it')
    control().focus()
    await window.smoke.key('F10', ['shift'])
    await until(() => document.activeElement === action('Rename page'), 'Keyboard actions did not focus Rename page')
    await window.smoke.key('Enter')
    await until(() => document.activeElement === element.querySelector('[aria-label="Page name"]'), 'Keyboard Rename did not focus the name field')
    const keyboardName = element.querySelector<HTMLInputElement>('[aria-label="Page name"]')!
    keyboardName.select()
    for (const key of 'Keyboard overview') await window.smoke.key(key)
    await window.smoke.key('Enter')
    await until(async () => JSON.parse(await raw()).pages[0].name === 'Keyboard overview' && document.activeElement === control(), () => `Keyboard Rename did not persist or restore focus (selector ${control()?.textContent}, focus ${document.activeElement?.outerHTML})`)
    await write(originalDocument)
    await until(() => control().textContent === 'Overview', 'Restoring the original page name did not finish')
    await window.smoke.key('F10', ['shift'])
    await until(() => document.activeElement === action('Rename page'), 'Keyboard rename actions did not reopen')
    await window.smoke.key('Enter')
    await until(() => document.activeElement === element.querySelector('[aria-label="Page name"]'), 'Keyboard Rename did not reopen')
    await window.smoke.key('Escape')
    await until(() => document.activeElement === control(), 'Cancelling Rename did not restore the selector focus')
    await window.smoke.key('F10', ['shift'])
    await until(() => document.activeElement === action('Rename page'), 'Keyboard actions did not reopen')
    await window.smoke.key('Escape')
    await until(() => document.activeElement === control() && !document.querySelector('[role="menu"]'), 'Closing actions did not restore the selector focus')
    await choose('Agent plan')
    control().focus()
    await window.smoke.key('F10', ['shift'])
    await until(() => document.activeElement === action('Rename page'), 'Keyboard deletion actions did not open')
    await window.smoke.key('Down')
    await until(() => document.activeElement === action('Delete page'), 'Keyboard actions did not focus Delete page')
    await window.smoke.key('Enter')
    await until(async () => !JSON.parse(await raw()).pages.some((page: { id: string }) => page.id === second) && control().textContent === 'Overview', 'Deleting the active page did not persist or select its fallback')
    if (node()?.textContent?.includes('Later agent edit')) throw new Error('Canvas pages: deleted page content leaked into its fallback')
    await write(originalDocument)
    await choose('Agent plan')
    await window.smoke.resize(430, 860)
    setInset!(340)
    await until(() => window.innerWidth === 430, 'Narrow canvas resize did not settle')
    const controls = element.querySelector<HTMLElement>('.canvas-pages')!.getBoundingClientRect()
    if (controls.left < 0 || controls.right > window.innerWidth) throw new Error('Page controls overflow the narrow canvas')
    checkControls()
    setInset!(0)
    await until(() => element.querySelector<HTMLElement>('.canvas')!.style.getPropertyValue('--canvas-inset') === '0px', 'Narrow inset did not settle')
    element.querySelector<HTMLButtonElement>('button[title="Fit everything"]')!.click()
    await until(inView, 'Narrow page target is outside the viewport')
    await window.smoke.capture('canvas-pages-narrow')
    const held = tab('Overview')!
    const box = held.getBoundingClientRect()
    const finger = new Touch({ identifier: 1, target: held, clientX: box.x + box.width / 2, clientY: box.y + box.height / 2 })
    held.dispatchEvent(new TouchEvent('touchstart', { touches: [finger], changedTouches: [finger], bubbles: true, cancelable: true }))
    await until(() => Boolean(action('Delete page')), 'Touch long-press did not open page actions')
    held.dispatchEvent(new TouchEvent('touchend', { touches: [], changedTouches: [finger], bubbles: true, cancelable: true }))
    held.click()
    if (control().textContent !== 'Agent plan' || document.querySelector('[role="menu"]')?.getAttribute('aria-label') !== 'Overview actions') throw new Error('Canvas pages: long-press switched pages on release')
    await new Promise((resolve) => setTimeout(resolve, 150))
    await window.smoke.capture('canvas-pages-touch-actions')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId }, search: {} })
    for (const method of ['setItem', 'getItem'] as const) {
      const original = Storage.prototype[method]
      Object.defineProperty(Storage.prototype, method, { configurable: true, writable: true, value: function (this: Storage, key: string, ...args: string[]) {
        if (this === localStorage && key.startsWith('jaz.canvasPage.')) throw new DOMException('Storage unavailable', 'QuotaExceededError')
        return Reflect.apply(original, this, [key, ...args])
      } })
      try {
        await remount(`storage-${method}`, method === 'getItem' ? 'Overview' : 'Agent plan')
        await choose(method === 'getItem' ? 'Agent plan' : 'Overview')
      } finally {
        Storage.prototype[method] = original
      }
    }
  } finally {
    await window.smoke.resize(1050, 850)
    root.unmount()
    client.clear()
    element.remove()
    setApiBaseUrl(before)
  }
}
