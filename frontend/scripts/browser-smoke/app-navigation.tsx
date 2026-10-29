import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createHashHistory, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { createRoot } from 'react-dom/client'
import { Route as rootRoute } from '@/routes/__root'
import { Route as appRoute } from '@/routes/apps.$serverId.$tool'
import { Route as newRoute } from '@/routes/new'
import type { AgentSettings } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { NEW_SESSION_AGENT_KEY, NEW_SESSION_DIRECTORY_KEY, NEW_SESSION_DRAFT_KEY } from '@/lib/newSessionConfig'

export async function exerciseAppNavigation() {
  const sourceURL = location.href
  const html = await fetch('/navigation-app.html').then((response) => response.text())
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const originalFetch = window.fetch
  const locations = new Map<MessageEventSource, { path: string; instance: number }>()
  let thread = ''
  const observe = (event: MessageEvent) => {
    if (event.data?.type === 'navigation-peer' && event.source) locations.set(event.source, event.data)
    if (event.data?.type === 'navigation-peer-thread') {
      thread = event.data.result
    }
  }
  const settings: AgentSettings = {
    providers: [], agents: ['codex'],
    acp: { codex: { enabled: true, model: 'gpt', reasoning_effort: 'medium' } },
    acp_options: { codex: { local: true, supports_auth: false, reasoning_efforts: [], models: [{ value: 'gpt', label: 'GPT', reasoning: { status: 'ready', efforts: ['medium'], default_effort: 'medium' } }] } },
  }
  const sessions: { directory: string; worktree: boolean }[] = []
  let workspaceChecks = false
  let workspaceError = false
  let heldCheck: ReturnType<typeof Promise.withResolvers<void>> | undefined
  let checkingWorkspace = false
  const prompts: { message: { text: string } }[] = []
  const originalAgent = localStorage.getItem(NEW_SESSION_AGENT_KEY)
  const originalDirectory = localStorage.getItem(NEW_SESSION_DIRECTORY_KEY)
  const originalSidebar = localStorage.getItem('jaz.sidebar')
  localStorage.setItem(NEW_SESSION_AGENT_KEY, 'codex')
  localStorage.setItem(NEW_SESSION_DIRECTORY_KEY, '')
  window.addEventListener('message', observe)
  window.fetch = async (input, init) => {
    const url = String(input)
    if (workspaceChecks && url.endsWith('/v1/projects')) return Response.json({ projects: [{ name: 'Repo', path: '/fixture/repo', git: true }, { name: 'Plain', path: '/fixture/plain', git: false }] })
    if (workspaceChecks && url.includes('/v1/filesystem/dirs?')) {
      if (heldCheck) {
        checkingWorkspace = true
        await heldCheck.promise
      }
      if (workspaceError) return Response.json({ error: 'Git check unavailable' }, { status: 503 })
      const path = new URL(url).searchParams.get('path')
      return Response.json({ path, git: path === '/fixture/repo', revision: path === '/fixture/repo' ? { branch: 'feature/current', head: 'abcdef1234567890' } : undefined, dirs: [] })
    }
    if (url.endsWith('/v1/mcp/apps')) return Response.json({ entrypoints: ['crm', 'tasks'].map((id) => ({ server_id: id, tool: 'show', title: id, type: 'global' })) })
    if (url.includes('/resource?tool=')) return Response.json({ html })
    if (url.endsWith('/tools/call')) return Response.json({ content: [] })
    if (url.endsWith('/v1/settings/agents')) {
      return Response.json(settings)
    }
    if (url.endsWith('/v1/sessions') && init?.method === 'POST') {
      sessions.push(JSON.parse(String(init.body)))
      return Response.json({ id: 'thread-smoke' })
    }
    if (url.endsWith('/v1/sessions/thread-smoke/queue')) {
      prompts.push(JSON.parse(String(init?.body)))
      return Response.json({ id: 'thread-smoke' })
    }
    if (url.includes('/v1/')) return new Response('Unavailable in this fixture', { status: 503 })
    return originalFetch(input, init)
  }
  window.history.replaceState(window.history.state, '', location.pathname)
  const history = createHashHistory()
  history.replace('/apps/crm/show?path=%2Fr%2Fjim')
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      createRoute({ getParentRoute: () => rootRoute, path: '/apps/$serverId/$tool', validateSearch: appRoute.options.validateSearch }),
      createRoute({ getParentRoute: () => rootRoute, path: '/new', component: newRoute.options.component, validateSearch: newRoute.options.validateSearch }),
      createRoute({ getParentRoute: () => rootRoute, path: '/sessions/$sessionId', component: () => <p>Thread</p> }),
      createRoute({ getParentRoute: () => rootRoute, path: '/bots', component: () => <p>Bots</p> }),
    ]),
    history,
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 5000
    while (!check()) {
      if (Date.now() > deadline) throw new Error(`App navigation: ${check.toString()}; host=${router.state.location.href}; peers=${JSON.stringify([...locations.values()])}`)
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const opaque = (target: Element | null) => {
    if (!target) return false
    for (let node: Element | null = target; node; node = node.parentElement) {
      if (Number(getComputedStyle(node).opacity) < 0.99) return false
    }
    return true
  }
  const settled = () => element.getAnimations({ subtree: true }).every((animation) =>
    animation.playState !== 'running' || animation.effect?.getComputedTiming().iterations === Infinity,
  )
  const frame = (id: string) => element.querySelector<HTMLIFrameElement>(`iframe[title="${id}"]`)!
  const path = () => router.state.location.search as { path?: string; input?: object }
  const at = (id: string, destination: string) =>
    router.state.location.pathname === `/apps/${id}/show` && path().path === destination && locations.get(frame(id).contentWindow!)?.path === destination
  const click = async (target: Element) => {
    const bounds = target.getBoundingClientRect()
    const x = Math.round(bounds.x + bounds.width / 2)
    const y = Math.round(bounds.y + bounds.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const visit = (id: string, search: { path?: string; input?: Record<string, unknown> } = {}) => router.navigate({ to: '/apps/$serverId/$tool', params: { serverId: id, tool: 'show' }, search })
  const step = async (direction: 'back' | 'forward') => {
    const button = element.querySelector<HTMLButtonElement>(`button[aria-label="Go ${direction}"]`)!
    if (button.disabled) throw new Error(`The ${direction} arrow is disabled`)
    await click(button)
  }
  try {
    root.render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
    await until(() => locations.size === 2 && at('crm', '/r/jim'))
    await router.navigate({ to: '/new' })
    await visit('crm', { path: '/people' })
    await until(() => at('crm', '/people'))
    const instance = locations.get(frame('crm').contentWindow!)!.instance
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', button: 'person' }, '*')
    await until(() => at('crm', '/r/jim'))
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', button: 'back' }, '*')
    await until(() => at('crm', '/people'))
    await step('forward')
    await until(() => at('crm', '/r/jim'))
    await step('back')
    await until(() => at('crm', '/people'))
    await step('back')
    await until(() => router.state.location.pathname === '/new')
    await step('forward')
    await until(() => at('crm', '/people'))
    await step('forward')
    await until(() => at('crm', '/r/jim'))
    await visit('tasks', { path: '/issue/CAS-1' })
    await until(() => at('tasks', '/issue/CAS-1'))
    await step('back')
    await until(() => at('crm', '/r/jim'))
    await step('forward')
    await until(() => at('tasks', '/issue/CAS-1'))
    await visit('crm')
    await until(() => at('crm', '/r/jim'))
    if (locations.get(frame('crm').contentWindow!)!.instance !== instance) throw new Error('Switching apps remounted CRM')
    await visit('crm', { input: { path: '/r/jane' } })
    await until(() => at('crm', '/r/jane') && !path().input)
    await step('back')
    await until(() => at('crm', '/r/jim'))
    await step('forward')
    await until(() => at('crm', '/r/jane'))
    await visit('crm', { input: { path: '/r/jane' } })
    await until(() => at('crm', '/r/jane') && !path().input)
    await visit('crm', { path: '/people?tags=Advisor' })
    await until(() => at('crm', '/people?tags=Advisor'))
    frame('crm').focus()
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', key: '[' }, '*')
    await until(() => at('crm', '/r/jane'))
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', key: ']' }, '*')
    await until(() => at('crm', '/people?tags=Advisor'))
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', key: 'å', code: 'BracketLeft' }, '*')
    await until(() => at('crm', '/r/jane'))
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', key: '¨', code: 'BracketRight' }, '*')
    await until(() => at('crm', '/people?tags=Advisor'))
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', button: 'replace' }, '*')
    await until(() => at('crm', '/people?tags=Customer'))
    await step('back')
    await until(() => at('crm', '/r/jane'))
    await visit('tasks', { path: '/issue/CAS-2' })
    await until(() => at('tasks', '/issue/CAS-2'))
    if (!element.querySelector<HTMLButtonElement>('button[aria-label="Go forward"]')!.disabled) throw new Error('A new app visit kept the abandoned forward branch')
    frame('crm').contentWindow!.postMessage({ type: 'navigation-peer-action', button: 'filter' }, '*')
    await until(() => locations.get(frame('crm').contentWindow!)?.path === '/people?tags=Advisor')
    if (!at('tasks', '/issue/CAS-2')) throw new Error('An inactive app changed Jaz navigation')
    await window.smoke.capture('app-navigation')
    frame('crm').focus()
    frame('tasks').contentWindow!.postMessage({ type: 'navigation-peer-action', button: 'thread' }, '*')
    await until(() => thread.includes('Only the app in use can start a chat'))
    if (sessions.length) {
      throw new Error('An app the user was not using started a thread')
    }
    const draft = JSON.stringify({ text: 'My own draft', tokens: [] })
    sessionStorage.setItem(NEW_SESSION_DRAFT_KEY, draft)
    thread = ''
    frame('tasks').focus()
    frame('tasks').contentWindow!.postMessage({ type: 'navigation-peer-action', button: 'thread' }, '*')
    await until(() => thread !== '')
    if (thread !== 'started') {
      throw new Error(`The app in use could not start a thread: ${thread}`)
    }
    await until(() => router.state.location.pathname === '/sessions/thread-smoke')
    if (JSON.stringify(sessions) !== JSON.stringify([{ title: 'Find the biggest city', runtime: 'acp', agent: 'codex', directory: '', worktree: false, model: 'gpt', reasoning_effort: 'medium' }]) || prompts[0]?.message.text !== 'Find the biggest city') {
      throw new Error(`The app's thread started wrong: ${JSON.stringify({ sessions, prompts })}`)
    }
    if (sessionStorage.getItem(NEW_SESSION_DRAFT_KEY) !== draft) {
      throw new Error('Starting an app thread discarded the composer draft')
    }
    await step('back')
    await until(() => at('tasks', '/issue/CAS-2'))
    await new Promise((resolve) => setTimeout(resolve, 300))
    if (sessions.length !== 1) {
      throw new Error('Going back from the app thread started it again')
    }

    // With the sidebar hidden, Chats offer a new chat in the titlebar and
    // Bots, which start bots from their own panel, do not.
    const toggle = () => element.querySelector<HTMLButtonElement>('button[aria-label="Hide sidebar"], button[aria-label="Show sidebar"]')
    const newChat = () => element.querySelector('a[aria-label="New chat"]')
    await router.navigate({ to: '/new' })
    await until(() => Boolean(toggle()))
    if (toggle()!.getAttribute('aria-label') === 'Hide sidebar') {
      await click(toggle()!)
    }
    await until(() => Boolean(newChat()))
    await router.navigate({ to: '/bots' })
    await until(() => router.state.location.pathname === '/bots' && toggle()?.getAttribute('aria-label') === 'Show sidebar')
    if (newChat()) {
      throw new Error('Bots offered a new chat in the titlebar')
    }
    workspaceChecks = true
    await client.invalidateQueries({ queryKey: keys.projects })
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' } })
    const workspace = () => document.querySelector<HTMLSelectElement>('select[aria-label="Task workspace"]')
    await until(() => workspace()?.value === 'worktree' && Boolean(element.textContent?.includes('From feature/current · abcdef1')))
    await until(() => opaque(workspace()) && settled())
    await window.smoke.capture('new-task-worktree')
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' }, state: { appPrompt: 'Isolated task' } })
    await until(() => sessions.length === 2 && router.state.location.pathname === '/sessions/thread-smoke')
    if (sessions[1]?.directory !== '/fixture/repo' || sessions[1]?.worktree !== true) throw new Error('A Git task bypassed managed worktree creation')
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' } })
    await until(() => workspace()?.value === 'worktree')
    workspace()!.value = 'local'
    workspace()!.dispatchEvent(new Event('change', { bubbles: true }))
    await until(() => workspace()?.value === 'local')
    heldCheck = Promise.withResolvers<void>()
    const refreshingWorkspace = client.invalidateQueries({ queryKey: keys.filesystemDirs('/fixture/repo') })
    await until(() => checkingWorkspace)
    if (workspace()!.disabled || !element.querySelector('textarea:not(:disabled)')) throw new Error('A known-metadata refresh blocked an already usable draft')
    heldCheck.resolve()
    await refreshingWorkspace
    heldCheck = undefined
    if (workspace()?.value !== 'local') throw new Error('Refreshing project metadata overwrote the explicit workspace choice')
    await router.navigate({ to: '/new', search: { project: '/fixture/plain' } })
    await until(() => !workspace())
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' } })
    await until(() => workspace()?.value === 'local')
    await window.smoke.resize(390, 844)
    const phoneOptions = () => [...element.querySelectorAll<HTMLButtonElement>('button[aria-haspopup="dialog"]')].find((button) => button.textContent?.startsWith('GPT'))
    await until(() => Boolean(phoneOptions()))
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    phoneOptions()!.click()
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    await until(() => Boolean(document.querySelector('[role="dialog"][aria-label="Thread settings"] select[aria-label="Task workspace"]')))
    await until(() => opaque(document.querySelector('[role="dialog"][aria-label="Thread settings"]')) && settled())
    await window.smoke.capture('new-task-local-phone')
    if (document.documentElement.scrollWidth > innerWidth) throw new Error('Workspace choice overflows the phone viewport')
    await window.smoke.resize(1050, 850)
    workspaceError = true
    await client.invalidateQueries({ queryKey: keys.filesystemDirs('/fixture/repo') })
    await until(() => Boolean([...element.querySelectorAll('button')].find((button) => button.textContent === 'Retry project check')))
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' }, state: { appPrompt: 'Explicit local task' } })
    await until(() => sessions.length === 3 && router.state.location.pathname === '/sessions/thread-smoke')
    if (sessions[2]?.worktree !== false) throw new Error('Explicit Local checkout was blocked by Git metadata or changed workspace')
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' } })
    await until(() => Boolean([...element.querySelectorAll('button')].find((button) => button.textContent === 'Retry project check')))
    await router.navigate({ to: '/new', search: { project: '/fixture/repo' }, state: { appPrompt: 'Unknown default workspace' } })
    await new Promise((resolve) => setTimeout(resolve, 100))
    if (sessions.length !== 3) throw new Error('Unknown Git state silently launched a default-workspace task')
    workspaceError = false
    const retryProject = [...element.querySelectorAll('button')].find((button) => button.textContent === 'Retry project check')!
    retryProject.click()
    await until(() => sessions.length === 4 && router.state.location.pathname === '/sessions/thread-smoke')
    if (sessions[3]?.worktree !== true) throw new Error('Default worktree choice was lost during Git-check recovery')
    await router.navigate({ to: '/new', search: { project: '/fixture/plain' }, state: { appPrompt: 'Plain directory task' } })
    await until(() => sessions.length === 5 && router.state.location.pathname === '/sessions/thread-smoke')
    if (sessions[4]?.directory !== '/fixture/plain' || sessions[4]?.worktree !== false) throw new Error('A non-Git directory requested a worktree')

  } catch (error) {
    await window.smoke.capture('app-navigation-failure')
    throw error
  } finally {
    root.unmount()
    client.clear()
    history.destroy()
    window.history.replaceState(window.history.state, '', sourceURL)
    element.remove()
    window.fetch = originalFetch
    window.removeEventListener('message', observe)
    sessionStorage.removeItem(NEW_SESSION_DRAFT_KEY)
    for (const [key, value] of [[NEW_SESSION_AGENT_KEY, originalAgent], [NEW_SESSION_DIRECTORY_KEY, originalDirectory], ['jaz.sidebar', originalSidebar]] as const) {
      if (value === null) {
        localStorage.removeItem(key)
      } else {
        localStorage.setItem(key, value)
      }
    }
  }
}
