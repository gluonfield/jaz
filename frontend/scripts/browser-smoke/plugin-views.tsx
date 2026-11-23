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
import pdfFixture from './fixtures/one-page.pdf?url'

export async function exercisePluginViews(): Promise<void> {
  const id = 'saved-plugin-view'
  const at = '2026-11-17T10:00:00Z'
  const code = (title: string) => `<h1>${title}</h1><p id="value"></p><button id="next">Open quiz</button><script>
const draw=data=>{
  document.getElementById('value').textContent=data.title;
  parent.postMessage({type:'fixture-render',title:'${title}',value:data.title},'*');
  if(data.open==='${title}'&&data.action!==window.lastAction){
    window.lastAction=data.action;
    document.getElementById('next').click();
  }
};
document.getElementById('next').addEventListener('click',()=>openView('quiz','Quiz'));
draw(window.data);
window.onData(draw);
</script>`
  const view: PluginView = { reference: 'example/status', title: 'Progress', file: 'status.json', widget_code: code('Progress') }
  const page: SessionMessages = { session: { id, slug: id, runtime: 'acp', status: 'idle', created_at: at, updated_at: at }, messages: [{ seq: 950, role: "user", content: '[Open draft](/work/draft.md) · [Open source](/work/source.pdf)', blocks: [], created_at: at }], events: [], has_earlier: true, before_event_seq: 900, latest_event_seq: 999, history_revision: 1 }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  client.setQueryData(keys.computerSettings, { enabled: false })
  client.setQueryData(keys.sessionOverview(id), { threads: [], subagents: [], plugin_views: [view] })
  client.setQueryData(keys.sessionRepo(id), { git: false })
  client.setQueryData(sessionFileQuery(id, view.file).queryKey, { path: '/work/status.json', content: '{"title":"Saved state"}', size: 23 })
  let draft = { path: '/work/draft.md', content: '# Cached query sheet\n\nEditable draft.', size: 36, version: 'v1' }
  client.setQueryData(sessionFileQuery(id, draft.path).queryKey, draft)
  const originalFetch = window.fetch
  let pdfReads = 0
  let replayed = false
  let opened = 0
  let savedSibling: PluginView | undefined
  let currentView = view
  let savedPresentations = 0
  let fileContent = '{"title":"Saved state"}'
  let fileReads = 0
  const drawn = new Map<string, string>()
  const observe = (event: MessageEvent) => {
    if ([...element.querySelectorAll('iframe')].some((frame) => frame.contentWindow === event.source) && event.data?.type === 'fixture-render') drawn.set(event.data.title, event.data.value)
  }
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname === `/v1/sessions/${id}/overview`) {
      return Response.json({ threads: [], subagents: [], plugin_views: savedSibling ? [savedSibling, currentView] : [currentView] })
    }
    if (url.pathname === `/v1/sessions/${id}/messages`) return Response.json(url.searchParams.has('before_event_seq') ? { ...page, before_event_seq: 600, messages: [{ ...page.messages[0], seq: 600 }] } : page)
    if (url.pathname === `/v1/sessions/${id}/events`) {
      replayed = url.searchParams.get('after_seq') !== '999'
      return new Response('', { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.pathname === `/v1/sessions/${id}/file`) {
      if (url.searchParams.get('path') === draft.path) {
        if (init?.method === 'PUT') {
          if (new Headers(init.headers).get('If-Match') !== draft.version) return new Response(null, { status: 412 })
          const content = String(init.body)
          draft = { ...draft, content, size: content.length, version: draft.version + '+' }
          return new Response(null, { status: 204, headers: { ETag: draft.version } })
        }
        return Response.json(draft)
      }
      if (url.searchParams.get('path') === '/work/source.pdf') {
        pdfReads++
        return originalFetch(pdfFixture)
      }
      fileReads++
      return Response.json({ path: '/work/status.json', content: fileContent, size: fileContent.length })
    }
    if (url.pathname.endsWith('/plugin-views')) {
      const input = JSON.parse(String(init?.body))
      if (url.pathname !== `/v1/sessions/${id}/plugin-views` || !['example/status', 'example/quiz'].includes(input.reference) || input.file !== view.file || input.name !== 'quiz') throw new Error('Bridge changed the caller scope')
      opened++
      if (!savedSibling) {
        savedSibling = { ...view, reference: 'example/quiz', title: input.title, widget_code: code('Quiz') }
        savedPresentations++
        client.setQueryData(keys.sessionOverview(id), { threads: [], subagents: [], plugin_views: [savedSibling, view] })
      }
      return Response.json(savedSibling)
    }
    if (url.pathname.endsWith('/seen')) return Response.json({ ok: true })
    return new Response(null, { status: 503 })
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  window.addEventListener('message', observe)
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
  const until = async (check: () => boolean, timeout = 4000) => {
    const deadline = Date.now() + timeout
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
    const switchThread = () => [...element.querySelectorAll('button')].find((button) => button.textContent === 'Switch thread')!.click()
    switchThread()
    await until(() => element.textContent?.includes('Another thread') === true)
    switchThread()
    await until(() => element.querySelectorAll('iframe[title="Progress"]').length === 1)
    drawn.clear()
    flushSync(() => root.render(null))
    mount()
    await until(() => drawn.get('Progress') === 'Saved state' && !client.isFetching({ queryKey: sessionFileQuery(id, view.file).queryKey }))
    await window.smoke.capture('plugin-views-reopened')
    const update = (value: object) => {
      fileContent = JSON.stringify(value)
      client.setQueryData(sessionFileQuery(id, view.file).queryKey, { path: '/work/status.json', content: fileContent, size: fileContent.length })
    }
    const select = (title: string) => [...element.querySelectorAll<HTMLElement>('[role="tab"]')].find((tab) => tab.textContent === title)!.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }))
    const work = async (state: 'running' | 'idle') => {
      client.setQueryData<SessionMessages>(keys.sessionMessages(id), (current) => current && { ...current, session: { ...current.session, status: state }, acp_state: state })
      await new Promise(requestAnimationFrame)
      await new Promise(requestAnimationFrame)
    }
    update({ title: 'New data', open: 'Progress', action: 1 })
    await until(() => opened === 1 && element.querySelectorAll('[role="tab"]').length === 2)
    select('Progress')
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'Progress' && !client.isFetching({ queryKey: sessionFileQuery(id, view.file).queryKey }))
    update({ title: 'New data', open: 'Progress', action: 2 })
    await until(() => opened === 2)
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'Quiz')
    if (savedPresentations !== 1 || element.querySelectorAll('[role="tab"]').length !== 2) throw new Error('Reopening a sibling added a presentation or duplicated its tab')
    await work('running')
    fileContent = '{"title":"Updated at work end"}'
    await work('idle')
    await until(() => drawn.get('Progress') === 'Updated at work end' && drawn.get('Quiz') === 'Updated at work end')
    const readsBeforeVisibility = fileReads
    fileContent = '{"title":"Updated while hidden"}'
    select('Progress')
    await until(() => fileReads > readsBeforeVisibility && drawn.get('Progress') === 'Updated while hidden')
    const idleReads = fileReads
    await new Promise((resolve) => setTimeout(resolve, 5250))
    if (fileReads !== idleReads) throw new Error('Idle view keeps polling its file')
    flushSync(() => root.render(null))
    mount()
    await until(() => element.querySelectorAll('[role="tab"]').length === 2)
    element.querySelector<HTMLButtonElement>('[aria-label="Close Quiz"]')!.click()
    await until(() => element.querySelectorAll('[role="tab"]').length === 1)
    element.querySelector<HTMLButtonElement>('[aria-label="Plugin views"]')!.click()
    await until(() => [...document.querySelectorAll('[role="menuitem"]')].some((item) => item.textContent === 'Quiz'))
    const quizItem = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find((item) => item.textContent === 'Quiz')!
    quizItem.click()
    await until(() => element.querySelectorAll('[role="tab"]').length === 2)
    await window.smoke.capture('plugin-views-siblings')
    const link = (text: string) => [...element.querySelectorAll<HTMLElement>('[role="link"]')].find((node) => node.textContent === text)!
    link('Open draft').click()
    await until(() => Boolean(element.querySelector('.tiptap')))
    const editor = element.querySelector<HTMLElement>('.tiptap')!
    editor.focus()
    document.execCommand('insertText', false, 'Retained edit. ')
    await until(() => editor.textContent?.includes('Retained edit.') === true)
    select('Progress')
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'Progress')
    select('draft.md')
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'draft.md')
    if (element.querySelector('.tiptap') !== editor || !editor.textContent?.includes('Retained edit.')) throw new Error('Switching tabs discarded the mounted editor or its edit')
    element.querySelector<HTMLButtonElement>('[aria-label="Close Quiz"]')!.click()
    await until(() => !element.querySelector('[aria-label="Close Quiz"]'))
    const progressFrame = element.querySelector('iframe[title="Progress"]')
    const tabOrder = [...element.querySelectorAll('[role="tab"]')].map((tab) => tab.id).join()
    currentView = { ...view, widget_code: code('Updated Progress') }
    await until(() => drawn.get('Updated Progress') === 'Updated while hidden', 8000)
    if (element.querySelector('iframe[title="Progress"]') !== progressFrame || [...element.querySelectorAll('[role="tab"]')].map((tab) => tab.id).join() !== tabOrder || element.querySelector('[role="tab"][aria-selected="true"]')?.textContent !== 'draft.md') {
      throw new Error('Overview polling replaced the mounted tab, changed tab order or selection, or reopened Quiz')
    }
    currentView = { ...view, widget_code: '' }
    await client.invalidateQueries({ queryKey: keys.sessionOverview(id) })
    await until(() => !element.querySelector('iframe[title="Progress"]') && Boolean(element.textContent?.includes('Plugin view unavailable.')))
    currentView = view
    await client.invalidateQueries({ queryKey: keys.sessionOverview(id) })
    await until(() => Boolean(element.querySelector('iframe[title="Progress"]')))
    link('Open source').click()
    await until(() => pdfReads === 1 && Boolean(element.querySelector('.pdfViewer .page')))
    select('Progress')
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'Progress')
    switchThread()
    await until(() => element.textContent?.includes('Another thread') === true)
    drawn.delete('Progress')
    switchThread()
    await until(() => drawn.has('Progress'))
    if (element.querySelector('[aria-label="Close Quiz"]')) throw new Error('Returning to the thread reopened the dismissed view')
    if (element.querySelector('.tiptap, .pdfViewer') || pdfReads !== 1) throw new Error('Restoring a thread mounted or downloaded hidden documents')
    select('source.pdf')
    await until(() => pdfReads === 2 && Boolean(element.querySelector('.pdfViewer .page')))
    select('draft.md')
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'draft.md')
    switchThread()
    await until(() => element.textContent?.includes('Another thread') === true)
    switchThread()
    await until(() => element.querySelector('[role="tab"][aria-selected="true"]')?.textContent === 'draft.md' && element.querySelector('.tiptap')?.textContent?.includes('Retained edit.') === true)
    if (element.querySelector('.pdfViewer') || pdfReads !== 2) throw new Error('Restoring the active file mounted or downloaded a background PDF')
  } finally {
    root.unmount()
    element.remove()
    client.clear()
    window.fetch = originalFetch
    window.removeEventListener('message', observe)
  }
}
