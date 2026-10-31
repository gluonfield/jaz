import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { createRoot } from 'react-dom/client'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanelStateProvider } from '@/components/session/SidePanelState'
import { ThreadView } from '@/components/session/ThreadView'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import { ToastProvider } from '@/components/ui/toast'
import type { AgentSessionConfigOption, Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { TitlebarActionsOutlet, TitlebarProvider, TitlebarSlotOutlet } from '@/lib/titlebar'
import { VoiceProvider } from '@/lib/voice/VoiceProvider'

export async function exerciseSessionConfig(): Promise<void> {
  const originalSize = { width: window.innerWidth, height: window.innerHeight }
  const savedChatWidth = localStorage.getItem('jaz.canvasChatWidth')
  localStorage.setItem('jaz.canvasChatWidth', '420')
  const session: Session = {
    id: 'config-smoke', slug: 'config-smoke', title: 'Composer settings', runtime: 'acp', status: 'running',
    created_at: '', updated_at: '', last_attention_at: '', runtime_ref: { type: 'acp', agent: 'codex' },
  }
  const fast: AgentSessionConfigOption = {
    id: 'fast-mode', name: 'Fast mode', category: 'model_config', current_value: 'off',
    options: [{ value: 'off', name: 'Off' }, { value: 'on', name: 'On' }],
  }
  const save = Promise.withResolvers<void>()
  const refresh = Promise.withResolvers<void>()
  let saving = false
  let refreshing = false
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = String(input)
    if (url.includes('/v1/sessions/config-smoke/agent/config')) {
      const body = JSON.parse(String(init?.body))
      if (body.id !== fast.id || body.value !== 'on') {
        throw new Error('Unexpected setting change: ' + JSON.stringify(body))
      }
      saving = true
      await save.promise
      fast.current_value = body.value
      return new Response(null, { status: 204 })
    }
    if (url.includes('/v1/sessions/config-smoke/messages')) {
      return Response.json({ session, messages: [], events: [], latest_event_seq: 0 })
    }
    if (url.includes('/v1/sessions/config-smoke/seen')) {
      return Response.json(session)
    }
    if (url.includes('/v1/sessions/config-smoke/canvas')) {
      return Response.json({ version: 'canvas-fixture', dir: '/workspace', pages: [{ id: 'page-1', name: 'Page 1', nodes: [
        { id: 'notes', type: 'text', x: 0, y: 0, width: 240, height: 120, text: 'Canvas Notes' },
      ], edges: [], files: {}, components: {} }] })
    }
    if (url.includes('/v1/sessions/config-smoke/overview')) {
      if (saving) {
        refreshing = true
        await refresh.promise
      }
      return Response.json({ agent_events: [{ session_id: session.id, seq: 1, at: new Date().toISOString(), type: 'agent_session', agent_session: { config_options: [fast] } }] })
    }
    if (url.includes('/v1/sessions/config-smoke/repo')) {
      return Response.json({ git: false })
    }
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const route = createRootRoute({ component: () => (
    <TitlebarProvider><BrowserWorkspace><SidePanelStateProvider>
      <div className="flex h-full flex-col">
        <header className="relative z-shell flex h-12 shrink-0 items-center gap-3 px-4">
          <TitlebarSlotOutlet />
          <div className="ml-auto"><TitlebarActionsOutlet /></div>
        </header>
        <div className="min-h-0 flex-1"><ThreadView sessionId={session.id} /></div>
      </div>
    </SidePanelStateProvider></BrowserWorkspace></TitlebarProvider>
  ) })
  const router = createRouter({ routeTree: route, history: createMemoryHistory({ initialEntries: ['/'] }) })
  const until = async (check: () => boolean | Promise<boolean>) => {
    const deadline = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > deadline) {
        throw new Error('Session config: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const toggle = () => document.querySelector<HTMLButtonElement>('button[aria-label="Fast Mode"]')
  const open = async () => {
    element.querySelector<HTMLButtonElement>('button[aria-label="Composer options"]')!.click()
    await until(() => Boolean(toggle()))
  }
  try {
    root.render(<QueryClientProvider client={client}><ToastProvider><VoiceProvider>
      <RouterProvider router={router} />
    </VoiceProvider></ToastProvider></QueryClientProvider>)
    await until(() => Boolean(element.querySelector('button[aria-label="Composer options"]')))
    if (element.querySelector('button[aria-label^="Model:"]')) {
      throw new Error('Existing chat shows a model picker')
    }
    await open()
    toggle()!.click()
    await until(() => saving && Boolean(toggle()?.disabled))
    await window.smoke.key('Escape')
    await until(() => !toggle())
    await open()
    if (!toggle()!.disabled) {
      throw new Error('Closing the menu lost the pending setting change')
    }
    save.resolve()
    await until(() => refreshing)
    await new Promise((resolve) => setTimeout(resolve, 50))
    if (!toggle()!.disabled) {
      throw new Error('Setting unlocked before refreshed native state arrived')
    }
    refresh.resolve()
    await until(() => toggle()?.getAttribute('aria-checked') === 'true' && !toggle()?.disabled)
    await window.smoke.key('Escape')
    await until(() => !toggle())

    client.setQueryData(keys.sessionMessages(session.id), {
      session: { ...session, status: 'idle', runtime_ref: { ...session.runtime_ref!, artifact_surface: 'canvas' } },
      messages: [], events: [], latest_event_seq: 0,
    })
    await until(() => Boolean(element.querySelector('.canvas')))
    const resizeHandle = () => element.querySelector<HTMLElement>('[aria-label="Resize canvas chat"]')!
    const chatWidth = () => element.querySelector('.canvas-chat')!.parentElement!.getBoundingClientRect().width
    const dragChat = async (targetWidth: number) => {
      const grip = resizeHandle().getBoundingClientRect()
      const x = Math.round(grip.x + grip.width / 2)
      const y = Math.round(grip.y + grip.height / 3)
      const next = Math.round(x + targetWidth - chatWidth())
      await window.smoke.pointer('mouseMove', x, y)
      await window.smoke.pointer('mouseDown', x, y)
      await window.smoke.pointer('mouseMove', next, y)
      const canvas = element.querySelector<HTMLElement>('.canvas')!
      if (Math.abs(Number.parseFloat(getComputedStyle(canvas).getPropertyValue('--canvas-inset')) - chatWidth()) > 1) {
        throw new Error('Canvas controls lag behind the chat during a drag')
      }
      await window.smoke.pointer('mouseUp', next, y)
      if (document.body.style.cursor === 'col-resize' || document.body.style.userSelect === 'none') {
        throw new Error('Chat resize did not release pointer state')
      }
    }
    await until(() => chatWidth() === 420)
    await dragChat(500)
    await until(() => chatWidth() === 500)
    if (document.activeElement !== resizeHandle()) {
      throw new Error('Dragging the chat divider lost keyboard focus')
    }
    await window.smoke.key('Right')
    await until(() => chatWidth() === 524)
    await window.smoke.key('Left')
    await until(() => chatWidth() === 500)
    await dragChat(10)
    await until(() => chatWidth() === Number(resizeHandle().getAttribute('aria-valuemin')))
    await dragChat(window.innerWidth - 20)
    await until(() => chatWidth() === Number(resizeHandle().getAttribute('aria-valuemax')))
    await dragChat(500)
    element.style.width = '0px'
    await until(() => element.querySelector('.canvas')!.getBoundingClientRect().width === 0)
    await until(() => chatWidth() === 0)
    element.style.removeProperty('width')
    await until(() => chatWidth() === 500)
    const composer = element.querySelector('textarea')!.getBoundingClientRect()
    const chatBounds = element.querySelector('.canvas-chat')!.getBoundingClientRect()
    if (composer.left < chatBounds.left || composer.right > chatBounds.right) {
      throw new Error('The composer did not follow the resized chat')
    }
    await window.smoke.capture('canvas-chat-resized')
    const chatToggle = () => element.querySelector<HTMLButtonElement>('button[aria-label$=" chat"]')!
    const transcript = element.querySelector('.scrollbar-quiet.overflow-y-auto')!
    const chatArea = transcript.getBoundingClientRect()
    const boardLeft = element.querySelector('.canvas')!.getBoundingClientRect().left
    const onBoard = () => Boolean(document.elementFromPoint(chatArea.x + chatArea.width / 2, chatArea.y + chatArea.height / 3)?.closest('.canvas'))
    chatToggle().click()
    await until(() => chatToggle().getAttribute('aria-label') === 'Show chat' && !transcript.checkVisibility({ visibilityProperty: true }) && !resizeHandle())
    const draft = element.querySelector('textarea')!.getBoundingClientRect()
    if (!onBoard() || element.querySelector('.canvas')!.getBoundingClientRect().left !== boardLeft || !document.elementFromPoint(draft.x + draft.width / 2, draft.y + draft.height / 2)?.closest('textarea')) {
      throw new Error('Hiding the chat did not leave only the composer over an unmoved board')
    }
    chatToggle().click()
    await until(() => chatToggle().getAttribute('aria-label') === 'Hide chat' && transcript.checkVisibility({ visibilityProperty: true }) && !onBoard())
    if (chatWidth() !== 500) {
      throw new Error('Hiding and showing the chat lost its resized width')
    }
    // Desktop moves the controls into the open panel's header; phones keep them in the titlebar.
    const panelButton = () => element.querySelector<HTMLButtonElement>('button[aria-label="Side Panel"]')!
    if (!panelButton() || !element.querySelector('button[aria-label="Overview"]')) {
      throw new Error('Canvas mode hides the side panel controls')
    }
    panelButton().click()
    await until(() => panelButton().getAttribute('aria-pressed') === 'true')
    element.querySelector<HTMLButtonElement>('button[aria-label="New tab"]')!.click()
    await until(() => [...document.querySelectorAll<HTMLButtonElement>('button')].some((button) => button.textContent?.trim() === 'Browser'))
    const browserOption = [...document.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent?.trim() === 'Browser')!
    browserOption.click()
    await until(() => Boolean(element.querySelector('input[placeholder="Website URL or file path"]')))
    const address = element.querySelector<HTMLInputElement>('input[placeholder="Website URL or file path"]')!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(address, location.origin + '/target')
    address.dispatchEvent(new Event('input', { bubbles: true }))
    await new Promise((resolve) => requestAnimationFrame(resolve))
    address.closest('form')!.requestSubmit()
    await until(() => Boolean(element.querySelector('webview')))
    const webview = element.querySelector<PreviewWebviewElement>('webview')!
    await until(() => webview.getBoundingClientRect().width > 200)
    await until(async () => {
      try {
        return await webview.executeJavaScript("document.querySelector('h1')?.textContent === 'Browser control'")
      } catch {
        return false
      }
    })
    const drawer = element.querySelector<HTMLElement>('[style*="--side-panel-width"]')!
    panelButton().click()
    await until(() => panelButton().getAttribute('aria-pressed') === 'false' && drawer.getBoundingClientRect().width < 1)
    panelButton().click()
    await until(() => panelButton().getAttribute('aria-pressed') === 'true' && webview.getBoundingClientRect().width > 200)
    let lastWidth = 0
    let stableFrames = 0
    await until(() => {
      const width = drawer.getBoundingClientRect().width
      stableFrames = Math.abs(width - lastWidth) < 0.25 ? stableFrames + 1 : 0
      lastWidth = width
      return width > 200 && stableFrames > 5
    })
    if ((element.querySelector('.canvas')?.getBoundingClientRect().width ?? 0) < 240 || !element.querySelector('.react-flow__node[data-id="notes"]')) {
      throw new Error('Opening the browser removed the canvas')
    }
    await window.smoke.capture('canvas-side-panel')
    await window.smoke.resize(390, 844)
    await until(() => window.innerWidth === 390 && Math.abs(drawer.getBoundingClientRect().width - 390) < 1 && Math.abs(drawer.getBoundingClientRect().left) < 1)
    if (resizeHandle() || chatWidth() > 390) {
      throw new Error('Desktop chat resizing leaked into the phone layout')
    }
    const controlBounds = panelButton().getBoundingClientRect()
    const x = Math.round(controlBounds.x + controlBounds.width / 2)
    const y = Math.round(controlBounds.y + controlBounds.height / 2)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
    await until(() => panelButton().getAttribute('aria-pressed') === 'false' && drawer.getBoundingClientRect().left >= 389)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
    await until(() => panelButton().getAttribute('aria-pressed') === 'true' && Math.abs(drawer.getBoundingClientRect().left) < 1)
    await window.smoke.capture('canvas-side-panel-mobile')
  } finally {
    save.resolve()
    refresh.resolve()
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    if (savedChatWidth === null) {
      localStorage.removeItem('jaz.canvasChatWidth')
    } else {
      localStorage.setItem('jaz.canvasChatWidth', savedChatWidth)
    }
    await window.smoke.resize(originalSize.width, originalSize.height)
  }
}
