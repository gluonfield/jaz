import { exerciseFrameNavigation } from './links'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider } from '@tanstack/react-router'
import { useLayoutEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanel } from '@/components/session/SidePanel'
import { SessionTitlebar } from '@/components/session/SessionTitlebar'
import { SidePanelDrawer } from '@/components/session/SidePanelDrawer'
import { SidePanelStateProvider, useSidePanelState } from '@/components/session/SidePanelState'
import { FileReaderLinkProvider, PreviewLinkProvider, RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { isPreviewWebviewPending, type PreviewWebviewElement } from '@/components/session/previewWebview'
import type { Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { setApiBaseUrl } from '@/lib/api/client'
import type { BrowserAction, BrowserActionResult } from '@/lib/browserApi'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { setThemePref } from '@/lib/theme'
import { TitlebarActionsOutlet, TitlebarProvider, TitlebarSlotOutlet } from '@/lib/titlebar'

export async function exerciseSidePanelTabs(): Promise<void> {
  const fixture = await fetch('/file-fixture').then((response) => response.json()) as { path: string }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  queryClient.setQueryData(keys.browserSettings, { enabled: false, mode: 'desktop' })
  queryClient.setQueryData(keys.health, { capabilities: { session_file_read: true } })
  queryClient.setQueryData(keys.sessionRepo('tabs'), { git: false })
  queryClient.setQueryData(keys.sessionRepoChanges('tabs'), { files: [{ path: 'change.ts', status: 'modified', added: 1, deleted: 0 }], total_added: 1, total_deleted: 0 })
  const diffKey = keys.sessionRepoDiff('tabs', 'modified:change.ts', '', '')
  queryClient.setQueryData(diffKey, { path: 'change.ts', patch: '@@ -1 +1 @@\n-before\n+after' })
  for (const path of ['/ANALYSIS.md', '/NOTES.md']) {
    queryClient.setQueryData(keys.sessionFile('tabs', path), { path, content: `# ${path.slice(1)}\n\nRetained document content.\n\n[Open notes](/NOTES.md)` })
  }
  const session: Session = {
    id: 'tabs', slug: 'tabs', title: 'Manufacturing research', runtime: 'acp', status: 'idle',
    created_at: '', updated_at: '', last_attention_at: '',
    runtime_ref: { type: 'acp', agent: 'codex', cwd: '/workspace' }, model: 'gpt-6-astra', reasoning_effort: 'xhigh',
  }
  let panel: ReturnType<typeof useSidePanelState>
  let activeSessionId: string
  let chatRenders = 0
  let terminalConnections = 0
  let terminalCloses = 0
  const terminalInput: string[] = []
  const NativeSocket = window.WebSocket
  class TerminalSocket {
    readyState = NativeSocket.OPEN
    onopen: (() => void) | null = null
    onmessage: ((event: { data: string }) => void) | null = null
    onclose: (() => void) | null = null
    constructor() {
      terminalConnections += 1
      setTimeout(() => {
        this.onopen?.()
        this.onmessage?.({ data: JSON.stringify({ type: 'ready', cwd: '/workspace' }) })
        this.onmessage?.({ data: JSON.stringify({ type: 'output', data: 'retained terminal output\r\n$ ' }) })
      }, 20)
    }
    send(data: string) {
      const message = JSON.parse(data)
      if (message.type === 'input') {
        terminalInput.push(message.data)
      }
    }
    close() {
      terminalCloses += 1
      this.readyState = NativeSocket.CLOSED
      this.onclose?.()
    }
  }
  window.WebSocket = new Proxy(NativeSocket, {
    construct(Target, args) {
      return String(args[0]).includes('/tabs/terminal') ? new TerminalSocket() : new Target(...args as [string])
    },
  })
  const noop = async () => {}
  function Chat({ sessionId }: { sessionId: string }) {
    chatRenders += 1
    const state = useSidePanelState(sessionId, true, sessionId === 'browser-background' ? false : undefined)
    const currentSession = { ...session, id: sessionId }
    const isMobile = useIsMobile()
    useLayoutEffect(() => {
      panel = state
      activeSessionId = sessionId
    })
    return <div ref={state.containerRef} className="flex h-full flex-col">
      <SessionTitlebar session={currentSession} panel={state} isMobile={isMobile} sideChatAvailable />
      <header className="titlebar-drag flex h-[52px] shrink-0 items-center gap-2 px-3" style={{ paddingLeft: isMobile ? 96 : 168 }}>
        <div id="titlebar-slot" className="relative z-shell flex min-w-0 items-center gap-1.5"><TitlebarSlotOutlet /></div>
        <div id="titlebar-actions" className="relative z-shell ml-auto flex min-w-0 items-center gap-1.5"><TitlebarActionsOutlet /></div>
      </header>
      <div className="relative flex min-h-0 flex-1">
        <div className="min-w-0 flex-1 p-8 text-ink" data-tab-chat>
          <FileReaderLinkProvider sessionId={sessionId} onOpen={state.openFile}>
            <PreviewLinkProvider onOpen={state.openPreview}>
              <RenderedMarkdown text={`# Product research\n\nReview the [analysis](/ANALYSIS.md) and compare it with the [supplier website](${location.origin}/target?tabs=one).\n\n[Local report](${fixture.path})`} />
            </PreviewLinkProvider>
          </FileReaderLinkProvider>
        </div>
        <SidePanelDrawer panel={state} isMobile={isMobile}>
          <SidePanel session={currentSession} panel={state} subagents={[]} spawnedThreads={[]} working={false} sideChatAvailable sideChatEvents={[]} onSend={noop} onQueuePrompt={noop} onQueueAction={noop} onSendSideChat={noop} />
        </SidePanelDrawer>
      </div>
    </div>
  }
  const rootRoute = createRootRoute({
    component: () => <BrowserWorkspace><SidePanelStateProvider><TitlebarProvider><Outlet /></TitlebarProvider></SidePanelStateProvider></BrowserWorkspace>,
  })
  const chatRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/sessions/$sessionId',
    component: () => {
      const { sessionId } = chatRoute.useParams()
      return <Chat key={sessionId} sessionId={sessionId} />
    },
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([chatRoute]),
    history: createMemoryHistory({ initialEntries: ['/sessions/tabs'] }),
  })
  const until = async (check: () => boolean | Promise<boolean>) => {
    const end = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > end) {
        throw new Error('Side panel check timed out: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const click = async (target: Element | null, pointerButton: 'left' | 'right' = 'left') => {
    if (!target) {
      throw new Error('Missing side panel control')
    }
    target.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    await new Promise(requestAnimationFrame)
    let previous = ''
    let stable = 0
    await until(() => {
      const bounds = target.getBoundingClientRect()
      const position = [bounds.x, bounds.y, bounds.width].map(Math.round).join(',')
      stable = position === previous ? stable + 1 : 0
      previous = position
      return stable >= 3 && target.contains(document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2))
    })
    const bounds = target.getBoundingClientRect()
    const x = Math.round(bounds.x + bounds.width / 2)
    const y = Math.round(bounds.y + bounds.height / 2)
    if (!target.contains(document.elementFromPoint(x, y))) {
      throw new Error('Side panel control is obscured: ' + target.outerHTML)
    }
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y, pointerButton)
    await window.smoke.pointer('mouseUp', x, y, pointerButton)
  }
  const button = (label: string) => [...document.querySelectorAll('button')].find((item) => (item.getAttribute('aria-label') === label || item.textContent?.trim() === label) && item.getBoundingClientRect().height)
  const tab = (id: string) => document.getElementById(`panel-tab-${id}`)
  const webview = (id: string) => document.querySelector(`[data-browser-session="${id}"] webview`) as PreviewWebviewElement
  const evaluate = async (view: PreviewWebviewElement, expression: string) => {
    const response = await window.jaz!.browserCommand({ webContentsId: view.getWebContentsId(), method: 'Runtime.evaluate', params: { expression, returnByValue: true } }) as { result: { value: unknown } }
    return response.result.value
  }
  const ready = async (id: string, condition = 'true') => {
    const view = webview(id)
    if (!view) {
      return false
    }
    try {
      return await evaluate(view, `document.readyState === "complete" && (${condition})`) === true
    } catch (error) {
      if (!isPreviewWebviewPending(error) && !(error instanceof Error && error.message.includes('Cannot find default execution context'))) {
        throw error
      }
      return false
    }
  }
  const tabMenu = async (id: string, label: string) => {
    await click(tab(id), 'right')
    await until(() => Boolean(button(label)))
    await click(button(label)!)
    await until(() => !button(label))
  }
  const add = async (label: string) => {
    await click(element.querySelector('[aria-label="New tab"]'))
    await until(() => Boolean(button(label)))
    await click(button(label)!)
    await until(() => !document.querySelector('[aria-label="New tab"][aria-expanded="true"]'))
  }
  try {
    await window.smoke.resize(1440, 900)
    setThemePref('dark')
    root.render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
    await until(() => Boolean(button('Side Panel')))
    await click(element.querySelector('[data-tab-chat] [role="link"]'))
    await until(() => panel?.activeTab?.kind === 'file' && Boolean(element.querySelector('[role="tabpanel"] h1')))
    if (button('Overview')?.getAttribute('aria-pressed') !== 'false' || button('Side Panel')?.getAttribute('aria-pressed') !== 'true') {
      throw new Error('File links did not select the tabbed panel exclusively')
    }
    await until(() => element.querySelector('[role="tablist"]')!.getBoundingClientRect().width > 100)
    await until(() => Math.abs(element.querySelector('[role="separator"]')!.parentElement!.getBoundingClientRect().width - panel.width) < 1)
    const tabsBounds = element.querySelector('[role="tablist"]')!.getBoundingClientRect()
    const controlsBounds = button('Side Panel')!.getBoundingClientRect()
    if (tabsBounds.height > 28 || Math.abs(tabsBounds.y - controlsBounds.y) > 1 || tabsBounds.right > controlsBounds.left) {
      throw new Error('Tabs and panel controls do not share a compact top row')
    }
    await click(element.querySelector('[role="tabpanel"] .chat-prose-link[role="link"]'))
    await until(() => panel.tabs.length === 2 && panel.activeTab?.id === 'file:/NOTES.md')
    await click(tab('file:/ANALYSIS.md'))
    await window.smoke.key('Right')
    await until(() => document.activeElement === tab('file:/NOTES.md'))
    const drag = tab('file:/ANALYSIS.md')!.getBoundingClientRect()
    const drop = tab('file:/NOTES.md')!.parentElement!.getBoundingClientRect()
    const dragX = Math.round(drag.x + drag.width / 2)
    const dragY = Math.round(drag.y + drag.height / 2)
    await window.smoke.pointer('mouseMove', dragX, dragY)
    await window.smoke.pointer('mouseDown', dragX, dragY)
    for (let step = 1; step <= 8; step += 1) {
      await window.smoke.pointer('mouseMove', Math.round(dragX + (drop.right - dragX) * step / 8), dragY)
    }
    await window.smoke.pointer('mouseUp', Math.round(drop.right), dragY)
    await until(() => panel.tabs[0].id === 'file:/NOTES.md')
    if (panel.activeTab?.id !== 'file:/NOTES.md') {
      throw new Error('Dragging an inactive tab changed the selection')
    }
    await window.smoke.capture('side-panel-tabs-reordered')
    await click(tab('file:/NOTES.md'))
    await window.smoke.key('Right', ['alt'])
    await until(() => panel.tabs[1].id === 'file:/NOTES.md' && document.activeElement === tab('file:/NOTES.md'))
    await click(element.querySelector('[role="tabpanel"]:not([hidden]) input'))
    await window.smoke.key('P', ['control'])
    if (panel.activeTab?.id !== 'file:/NOTES.md') {
      throw new Error('Control+P in a file input was intercepted by the browser tab shortcut')
    }
    await click([...element.querySelectorAll('[data-tab-chat] [role="link"]')].find((item) => item.textContent === 'Local report')!)
    await until(() => ready('tabs'))
    const report = webview('tabs')
    await until(async () => await evaluate(report, 'document.querySelector("output")?.textContent') === 'Relative script loaded')
    if (await evaluate(report, 'getComputedStyle(document.querySelector("h1")).color') !== 'rgb(12, 90, 50)') {
      throw new Error('Local HTML lost its relative stylesheet')
    }
    await until(() => {
      const icon = tab('tabs')?.querySelector('img')
      return Boolean(icon?.src.endsWith('/report-icon.svg') && icon.complete && icon.naturalWidth)
    })
    await evaluate(report, 'document.querySelector("link[rel=icon]").href = "report-icon.svg?updated"')
    await until(() => tab('tabs')?.querySelector('img')?.src.endsWith('/report-icon.svg?updated') === true)
    await evaluate(report, 'document.title = "Actuator research"')
    await until(() => tab('tabs')?.textContent === 'Actuator research')
    if (!tab('tabs')?.querySelector('img')?.src.endsWith('/report-icon.svg?updated')) {
      throw new Error('A title update discarded the page favicon')
    }
    await window.smoke.capture('side-panel-local-html')
    for (const [label, expected] of [['BOM CSV', 'Motor, large'], ['Excel workbook', '12.5'], ['Legacy Excel', '12.5'], ['Calculator', 'Cost calculator'], ['External source', '']] as const) {
      await click(tab('tabs'))
      await until(() => !document.querySelector<HTMLElement>('[data-browser-session="tabs"]')!.inert)
      const count = panel.tabs.length
      const point = await evaluate(report, `(() => {
        const link = Array.from(document.querySelectorAll('a')).find(link => link.textContent === ${JSON.stringify(label)})
        const rect = link.getBoundingClientRect()
        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }
      })()`) as { x: number; y: number }
      for (const type of ['mousePressed', 'mouseReleased']) {
        await window.jaz!.browserCommand({ webContentsId: report.getWebContentsId(), method: 'Input.dispatchMouseEvent', params: { type, ...point, button: 'left', clickCount: 1 } })
      }
      await until(() => panel.tabs.length === count + 1 && panel.activeTab?.id !== 'tabs')
      const opened = panel.activeTab!
      if (label === 'Calculator') {
        await until(() => ready(opened.id))
        await until(async () => await evaluate(webview(opened.id), 'document.querySelector("h1")?.textContent') === expected)
        if (await evaluate(webview(opened.id), 'location.search + location.hash') !== '?volume=500#costs') {
          throw new Error('Local HTML link lost its query or fragment')
        }
      } else if (label !== 'External source') {
        await until(() => document.getElementById(`panel-body-${opened.id}`)?.textContent?.includes(expected) === true)
        const body = document.getElementById(`panel-body-${opened.id}`)!
        if (!tab(opened.id)?.querySelector('.lucide-file-spreadsheet')) {
          throw new Error('Spreadsheet tab is missing its icon')
        }
        if (label === 'BOM CSV' && !body.textContent?.includes('00123')) {
          throw new Error('CSV preview changed the leading zeroes')
        }
        const select = body.querySelector<HTMLSelectElement>('select[aria-label="Worksheet"]')
        if (label !== 'BOM CSV') {
          if (!select) {
            throw new Error('Excel sheets are missing')
          }
          select.value = 'Scenarios'
          select.dispatchEvent(new Event('change', { bubbles: true }))
          await until(() => body.querySelector('table')?.textContent?.includes('500') === true)
        }
        await new Promise(requestAnimationFrame)
        await window.smoke.capture(label === 'BOM CSV' ? 'side-panel-csv' : 'side-panel-excel')
      }
      if ((await window.smoke.popupURLs()).length || (await window.smoke.openedURLs()).includes('https://example.com/')) {
        throw new Error('A local report link escaped its sidebar tab')
      }
      panel.closeTab(opened.id)
      await until(() => panel.tabs.length === count)
    }
    await evaluate(report, 'location.href = "./calculator:one.html"')
    await until(async () => await evaluate(report, 'document.querySelector("h1")?.textContent') === 'Cost calculator')
    await until(() => !tab('tabs')?.querySelector('img'))
    panel.closeTab('tabs')
    await until(() => panel.tabs.length === 2)
    await click(element.querySelector('[data-tab-chat] a'))
    await until(() => ready('tabs'))
    const first = webview('tabs')
    await exerciseFrameNavigation(first)
    const firstID = first.getWebContentsId()
    await evaluate(first, 'window.retainedTabValue = 41')
    const firstSize = await evaluate(first, '[innerWidth,innerHeight].join(",")')
    await add('Browser')
    await until(() => panel.tabs.filter((entry) => entry.kind === 'preview').length === 2)
    const secondId = panel.activeTab!.id
    const input = document.querySelector<HTMLInputElement>(`[data-browser-session="${secondId}"] input`)!
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    setter.call(input, location.origin + '/target?tabs=two')
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.form!.requestSubmit()
    await until(() => ready(secondId))
    await evaluate(webview(secondId), 'window.retainedTabValue = 92')
    await click(tab('tabs'))
    await until(() => !document.querySelector<HTMLElement>('[data-browser-session="tabs"]')!.inert)
    if (webview('tabs').getWebContentsId() !== firstID || await evaluate(first, 'window.retainedTabValue') !== 41 || await evaluate(first, '[innerWidth,innerHeight].join(",")') !== firstSize) {
      throw new Error('Switching tabs replaced or resized the retained browser')
    }
    await tabMenu('tabs', 'Reload')
    await until(() => ready('tabs', 'window.retainedTabValue === undefined'))
    if (webview('tabs').getWebContentsId() !== firstID) {
      throw new Error('Reloading a tab replaced its browser')
    }
    const tabCount = panel.tabs.length
    await tabMenu('tabs', 'Duplicate')
    await until(() => panel.tabs.length === tabCount + 1)
    const copy = panel.tabs[panel.tabs.findIndex((entry) => entry.id === 'tabs') + 1]
    await until(() => ready(copy.id))
    if (panel.activeTab?.id !== copy.id || webview(copy.id).getWebContentsId() === firstID || await evaluate(webview(copy.id), 'location.href') !== await evaluate(first, 'location.href')) {
      throw new Error('Duplicate did not open the same page in a new browser beside its source')
    }
    await tabMenu(copy.id, 'Close')
    await until(() => panel.tabs.length === tabCount)
    const toolbar = document.querySelector('[data-browser-session="tabs"] form')!
    const browserBounds = toolbar.getBoundingClientRect()
    const headerBounds = element.querySelector('header')!.getBoundingClientRect()
    const browserHost = document.querySelector<HTMLElement>('[data-browser-session="tabs"]')!
    const resizeHandle = element.querySelector('[role="separator"]')!
    if (Math.abs(browserBounds.top - headerBounds.bottom) > 1 || getComputedStyle(browserHost).borderTopLeftRadius !== '0px' || getComputedStyle(resizeHandle.firstElementChild!).backgroundColor !== 'rgba(0, 0, 0, 0)') {
      throw new Error('Side panel retains a gap, rounded browser corner or visible resize border')
    }
    if (!button('Side Panel')!.querySelector('svg') || !button('Overview')!.querySelector('svg')) {
      throw new Error('Panel controls are missing icons')
    }
    const back = toolbar.querySelector('[aria-label="Back"]')!.getBoundingClientRect()
    const forward = toolbar.querySelector('[aria-label="Forward"]')!.getBoundingClientRect()
    if (toolbar.getBoundingClientRect().height !== 36 || back.width !== 28 || forward.x - back.x > 30) {
      throw new Error('Browser navigation controls are not compact')
    }
    await window.smoke.capture('side-panel-browser-compact')
    await add('Terminal')
    await until(() => terminalConnections === 1 && element.textContent?.includes('retained terminal output') === true)
    await window.smoke.key('D', ['control'])
    if (terminalInput.join('') !== '\u0004' || panel.activeTab?.kind !== 'terminal') {
      throw new Error('Opening a terminal did not focus it or preserve Control+D input')
    }
    const terminal = element.querySelector('.xterm')
    await click(button('Overview')!)
    await until(() => button('Overview')!.getAttribute('aria-pressed') === 'true')
    if (element.querySelector('[role="tablist"]') || !document.querySelector<HTMLElement>('[data-browser-session="tabs"]')!.inert) {
      throw new Error('Overview left tabs or browser visible')
    }
    await click(button('Side Panel')!)
    await until(() => panel.mode === 'tabs' && panel.open)
    if (terminalConnections !== 1 || terminalCloses || element.querySelector('.xterm') !== terminal) {
      throw new Error('Switching Overview restarted the terminal')
    }
    await add('Side chat')
    await until(() => Boolean(element.querySelector('[role="tabpanel"]:not([hidden]) textarea')))
    const composer = element.querySelector<HTMLTextAreaElement>('[role="tabpanel"]:not([hidden]) textarea')!
    await click(composer)
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(composer, 'Draft for the side chat')
    composer.dispatchEvent(new Event('input', { bubbles: true }))
    await click(tab('file:/ANALYSIS.md'))
    await click(tab('side-chat'))
    if (composer.value !== 'Draft for the side chat') {
      throw new Error('Switching tabs discarded the side-chat draft')
    }
    await click(tab('file:/ANALYSIS.md'))
    await until(() => panel.activeTab?.id === 'file:/ANALYSIS.md')
    await new Promise((resolve) => setTimeout(resolve, 700))
    await window.smoke.capture('side-panel-tabs-dark')
    await click(element.querySelector('[aria-label="New tab"]'))
    await until(() => Boolean(button('Browser')))
    await until(() => getComputedStyle(button('Browser')!.closest('[data-escape-surface]')!).opacity === '1')
    await window.smoke.capture('side-panel-tab-menu')
    await window.smoke.key('Escape')
    setThemePref('light')
    await new Promise((resolve) => setTimeout(resolve, 200))
    await window.smoke.capture('side-panel-tabs-light')
    const rendersBeforeTitleChange = chatRenders
    await evaluate(first, 'document.title = "Updated browser title"')
    await until(() => tab('tabs')?.textContent === 'Updated browser title')
    if (chatRenders !== rendersBeforeTitleChange) {
      throw new Error('A browser title update rerendered the conversation')
    }
    await add('Code diff')
    await until(() => queryClient.getQueryCache().find({ queryKey: diffKey })?.isActive() === true)
    await click(tab('file:/ANALYSIS.md'))
    await until(() => panel.activeTab?.kind === 'file')
    if (queryClient.getQueryCache().find({ queryKey: diffKey })?.isActive()) {
      throw new Error('A hidden code diff still has an active query')
    }
    await queryClient.invalidateQueries({ queryKey: keys.sessionRepo('tabs') })
    if (queryClient.getQueryState(diffKey)?.fetchStatus !== 'idle') {
      throw new Error('Invalidation refetched a hidden code diff')
    }
    panel.closeTab('diff')
    const secondID = webview(secondId).getWebContentsId()
    panel.closeTab(secondId)
    await until(async () => !await window.smoke.browserExists(secondID))
    if (!await window.smoke.browserExists(firstID)) {
      throw new Error('Closing a browser also closed the neighbouring browser')
    }
    await window.smoke.resize(390, 844)
    await until(() => window.innerWidth === 390)
    await click(button('Overview')!)
    await click(button('Side Panel')!)
    await until(() => panel.mode === 'tabs')
    await new Promise((resolve) => setTimeout(resolve, 200))
    await window.smoke.capture('side-panel-tabs-mobile')
    if (element.scrollWidth > window.innerWidth || !element.querySelector('[aria-label="New tab"]')!.getBoundingClientRect().width || tab(panel.activeTab!.id)!.getBoundingClientRect().width < 80) {
      throw new Error('Mobile tabs overflow or hide their add control')
    }
    panel.closeTab('terminal')
    await until(() => terminalCloses === 1)
    for (const entry of [...panel.tabs].filter((entry) => entry.kind !== 'terminal')) {
      panel.closeTab(entry.id)
      await until(() => !panel.tabs.some((tab) => tab.id === entry.id))
    }
    await until(() => Boolean(button('Browser')))
    const openPreview = panel.openPreview
    openPreview(location.origin + '/target?tabs=rapid-one')
    openPreview(location.origin + '/target?tabs=rapid-two')
    await until(() => panel.tabs.filter((entry) => entry.kind === 'preview').length === 2)
    for (const entry of [...panel.tabs]) {
      panel.closeTab(entry.id)
      await until(() => !panel.tabs.some((tab) => tab.id === entry.id))
    }
    await click(button('Browser')!)
    await until(() => panel.activeTab?.kind === 'preview')
    const addTab = panel.addTab
    addTab('preview')
    addTab('preview')
    await until(() => panel.tabs.length === 3)
    const neighbour = panel.tabs[1].id
    await click(tab('tabs'))
    await window.smoke.key('Delete')
    await until(() => panel.activeTab?.id === neighbour && document.activeElement === tab(neighbour))
    await window.smoke.key('Delete')
    await until(() => panel.tabs.length === 1 && document.activeElement === tab(panel.activeTab!.id))
    await window.smoke.key('Delete')
    await until(() => panel.tabs.length === 0 && document.activeElement?.getAttribute('aria-label') === 'New tab')

    await window.smoke.resize(1440, 900)
    await until(() => window.innerWidth === 1440)
    panel.openFile('/ANALYSIS.md:12')
    panel.openFile('/NOTES.md')
    for (const kind of ['terminal', 'side-chat', 'diff'] as const) {
      panel.addTab(kind)
    }
    panel.openPreview(location.origin + '/target?tabs=retained-one')
    panel.openPreview(location.origin + '/target?tabs=retained-two')
    await until(() => panel.tabs.length === 7 && panel.activeTab?.kind === 'preview')
    const retainedTabId = panel.activeTab!.id
    await until(() => ready(retainedTabId))
    const retainedView = webview(retainedTabId)
    const retainedViewId = retainedView.getWebContentsId()
    await evaluate(retainedView, 'window.chatNavigationValue = 73')
    await click(tab(retainedTabId))
    await window.smoke.key('Left', ['alt'])
    await until(() => panel.tabs[5].id === retainedTabId && document.activeElement === tab(retainedTabId))
    panel.resize(720)
    await until(() => panel.width === 720)
    const savedTabs = JSON.stringify(panel.tabs)
    const navigate = async (sessionId: string) => {
      await router.navigate({ to: '/sessions/$sessionId', params: { sessionId } })
      await until(() => activeSessionId === sessionId)
    }
    await navigate('other-chat')
    if (panel.tabs.length || panel.mode !== 'overview') {
      throw new Error('A new chat inherited another chat\'s tabs or mode')
    }
    if (!document.querySelector<HTMLElement>(`[data-browser-session="${retainedTabId}"]`)!.inert) {
      throw new Error('Navigating away left the previous chat browser active')
    }
    panel.addTab('file')
    await until(() => panel.tabs.length === 1)
    panel.close()
    await until(() => !panel.open)
    await navigate('tabs')
    await until(() => !document.querySelector<HTMLElement>(`[data-browser-session="${retainedTabId}"]`)!.inert)
    await until(() => panel.width === 720)
    if (JSON.stringify(panel.tabs) !== savedTabs || panel.activeTab?.id !== retainedTabId || !panel.open || panel.mode !== 'tabs' || panel.width !== 720) {
      throw new Error('Returning to a chat lost its tabs, selection, visibility, mode or width')
    }
    if (webview(retainedTabId).getWebContentsId() !== retainedViewId || await evaluate(retainedView, 'window.chatNavigationValue') !== 73) {
      throw new Error('Returning to a chat replaced its retained browser page')
    }
    panel.closeTab('file:/NOTES.md')
    panel.toggleMode('overview')
    await until(() => panel.mode === 'overview' && panel.tabs.length === 6)
    await navigate('other-chat')
    if (panel.open || panel.mode !== 'tabs' || panel.tabs.length !== 1 || panel.activeTab?.id !== 'file') {
      throw new Error('The other chat lost its independent closed panel and file picker')
    }
    await navigate('tabs')
    if (!panel.open || panel.mode !== 'overview' || panel.tabs.some((entry) => entry.id === 'file:/NOTES.md')) {
      throw new Error('Returning to a chat lost Overview or resurrected a closed tab')
    }
    panel.toggleMode('tabs')
    await until(() => panel.mode === 'tabs')
    panel.selectTab('file:/ANALYSIS.md')
    await until(() => panel.activeTab?.kind === 'file' && Boolean(element.querySelector('[role="tabpanel"] h1')))
    await until(() => Math.abs(element.querySelector('[role="separator"]')!.parentElement!.getBoundingClientRect().width - panel.width) < 1)
    await window.smoke.capture('side-panel-restored-tabs')

    const backend = await window.smoke.backend()
    setApiBaseUrl(backend)
    queryClient.setQueryData(keys.browserSettings, { enabled: true, mode: 'desktop' })
    const action = async (input: BrowserAction | { action: 'tabs' } | { action: 'script'; code: string }): Promise<BrowserActionResult> => {
      const endpoint = input.action === 'script' ? '/exercise-script/tabs' : '/v1/sessions/tabs/browser'
      const response = await fetch(backend + endpoint, { method: 'POST', body: JSON.stringify(input) })
      if (!response.ok) {
        throw new Error(await response.text())
      }
      return response.json()
    }
    await until(async () => (await fetch(`${backend}/v1/sessions/tabs/browser`, { method: 'POST', body: '{"action":"status"}' })).ok)
    panel.selectTab('tabs')
    await until(() => ready('tabs'))
    const agentView = webview('tabs')
    const agentID = agentView.getWebContentsId()
    const agentURL = await evaluate(agentView, 'location.href') as string
    const shown = () => panel.open && panel.mode === 'tabs' && panel.activeTab?.id === 'tabs' && !document.querySelector<HTMLElement>('[data-browser-session="tabs"]')!.inert
    for (const input of [
      { action: 'navigate', url: agentURL },
      { action: 'state' },
      { action: 'script', code: 'await tab.cdp.send("Runtime.evaluate", { expression: "window.visibilityProbe = 73" })' },
    ] satisfies Parameters<typeof action>[0][]) {
      panel.close()
      await until(() => !panel.open)
      await action({ action: 'tabs' })
      if (panel.open) {
        throw new Error('Listing browser tabs opened an idle panel')
      }
      await action(input)
      if (!shown()) {
        throw new Error(`${input.action} did not reveal the retained agent browser`)
      }
      if (webview('tabs').getWebContentsId() !== agentID) {
        throw new Error('Revealing the agent browser replaced its webview')
      }
    }
    panel.toggleMode('overview')
    await until(() => panel.mode === 'overview')
    await action({ action: 'state' })
    if (!shown()) {
      throw new Error('Browser activity did not switch Overview to the browser')
    }
    panel.selectTab(retainedTabId)
    await until(() => panel.activeTab?.id === retainedTabId)
    await action({ action: 'script', code: 'await tab.getAXState()' })
    if (!shown() || await evaluate(retainedView, 'window.chatNavigationValue') !== 73 || await evaluate(agentView, 'window.visibilityProbe') !== 73) {
      throw new Error('Browser activity did not select its own tab while preserving the other page')
    }
    const pending = action({ action: 'script', code: `await tab.cdp.send('Runtime.evaluate', {
expression: 'new Promise(resolve => { window.resumeVisibility = resolve })',
awaitPromise: true
})
await tab.cdp.send('Runtime.evaluate', { expression: 'window.visibilityProbe += 1' })` })
    await until(async () => await evaluate(agentView, 'typeof window.resumeVisibility') === 'function')
    panel.close()
    await until(() => !panel.open)
    await evaluate(agentView, 'window.resumeVisibility()')
    await pending
    if (!shown() || await evaluate(agentView, 'window.visibilityProbe') !== 74) {
      throw new Error('Continuing a raw CDP script did not reveal its browser after panel closure')
    }
    const observed = await action({ action: 'state' })
    const page = observed.data as { elements: { name: string; ref: string }[] }
    const target = page.elements.find((entry) => entry.name === 'Run this step')!
    panel.close()
    await until(() => !panel.open && element.querySelector('[role="separator"]')!.parentElement!.getBoundingClientRect().width === 0)
    await action({ action: 'click', ref: target.ref })
    if (!shown() || !await evaluate(agentView, 'window.clicks === 1 && window.trusted')) {
      throw new Error('Clicking from a closed panel did not reveal the browser and deliver trusted input')
    }
    await window.smoke.capture('browser-activity-reveals-panel')
    panel.close()
    await until(() => !panel.open && element.querySelector('[role="separator"]')!.parentElement!.getBoundingClientRect().width === 0)
    await navigate('other-chat')
    await action({ action: 'state' })
    if (panel.open || panel.activeTab?.id !== 'file') {
      throw new Error('Background browser work opened another conversation’s panel')
    }
    await navigate('tabs')
    await until(shown)
    await until(() => Math.abs(document.querySelector<HTMLElement>('[data-browser-session="tabs"]')!.getBoundingClientRect().width - panel.width) < 1)
    if (webview('tabs').getWebContentsId() !== agentID) {
      throw new Error('Revealing background browser activity replaced its webview')
    }
    await window.smoke.capture('browser-activity-revealed-on-return')
    await navigate('other-chat')
    await window.smoke.resize(1440, 900)
    await until(() => window.innerWidth === 1440)
    await window.smoke.key('S', ['meta', 'shift'])
    await until(() => panel.open && panel.mode === 'tabs')
    await window.smoke.key('S', ['meta', 'shift'])
    await until(() => !panel.open)
    await window.smoke.key('S', ['meta', 'shift'])
    await until(() => panel.open && panel.mode === 'tabs')
    await click(button('Overview')!)
    await until(() => panel.open && panel.mode === 'overview')
    await window.smoke.key('S', ['meta', 'shift'])
    await until(() => !panel.open && panel.mode === 'overview')
    await window.smoke.key('S', ['meta', 'shift'])
    await until(() => panel.open && panel.mode === 'tabs')
    await navigate('browser-background')
    await until(async () => (await fetch(`${backend}/v1/sessions/browser-background/browser`, { method: 'POST', body: '{"action":"status"}' })).ok)
    if (panel.open || panel.tabs.length || webview('browser-background')) {
      throw new Error('Fresh bot browser fixture did not start closed and empty')
    }
    const opened = await fetch(`${backend}/v1/sessions/browser-background/browser`, {
      method: 'POST',
      body: JSON.stringify({ action: 'navigate', url: `${location.origin}/target?fresh-bot` }),
    })
    if (!opened.ok) {
      throw new Error(await opened.text())
    }
    await until(() => ready('browser-background'))
    const botPanel = document.querySelector<HTMLElement>('[data-browser-session="browser-background"]')!
    if (!panel.open || panel.mode !== 'tabs' || panel.activeTab?.id !== 'browser-background' || botPanel.inert || botPanel.getBoundingClientRect().width < 240) {
      throw new Error('First bot navigation succeeded without showing its browser panel')
    }
    await window.smoke.capture('fresh-bot-browser')
  } catch (error) {
    await window.smoke.capture('side-panel-tabs-failure')
    throw error
  } finally {
    root.unmount()
    setApiBaseUrl(location.origin)
    queryClient.clear()
    window.WebSocket = NativeSocket
    element.remove()
    setThemePref('light')
    await window.smoke.resize(1050, 850)
  }
}
