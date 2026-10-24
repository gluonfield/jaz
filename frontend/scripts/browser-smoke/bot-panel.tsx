import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BotDetails } from '@/components/bots/BotDetails'
import { BotsPanel } from '@/components/bots/BotsPanel'
import { getAppearance, setAppearance } from '@/lib/appearance'
import { botsQuery } from '@/lib/api/bots'
import { Route as botsIndexRoute } from '@/routes/bots.index'
import type { Bot, BotActivity, BotFile, BotOwed, BotStartedThread, Loop, LoopRun } from '@/lib/api/types'

// A bot's side panel against a stubbed server: what it owes, is doing and
// did, its routines and their actions, and its AGENTS.md and MEMORY.md edited
// as documents, saved only when edited and never over the bot's newer write.
export async function exerciseBotPanel(): Promise<void> {
  const minutes = (n: number) => new Date(Date.now() - n * 60_000).toISOString()
  const bot: Bot = { id: 'panel', kind: 'bot', name: 'Jaz Self Improvement Bot', avatar: { shape: 'hex', color: 'amber' }, unread: false, status: 'idle', updated_at: minutes(0) }
  const owed: BotOwed[] = [
    { id: 4, request: 'Keep bot tasks and their results visible across delegated work, restarts and follow-up conversations', status: 'in_progress', next: 'check its report, then send it', opened_at: minutes(14), threads: [{ id: 'side-a', title: 'Commitments and lost results', state: 'working' }] },
    { id: 3, request: 'Which fixes matter this week', status: 'in_progress', for: 'Priorities Bot', opened_at: minutes(95), threads: [{ id: 'side-e', title: 'Fixes triage', state: 'finished' }] },
    { id: 6, request: 'Compare the supplier quotes', status: 'in_progress', stalled: true, next: 'send the comparison', opened_at: minutes(40), threads: [{ id: 'side-h', title: 'Quote comparison', state: 'finished' }] },
    { id: 5, request: 'Overnight stress run', status: 'pending', blocked: true, depends_on: [1], opened_at: minutes(70) },
    { id: 2, request: 'Pause prompt suggestions on hidden turns', status: 'waiting_for_user', question_seq: 15, next: 'wait for his OK, then start the fork change', opened_at: minutes(60 * 3) },
    { id: 1, request: 'Capacity test with 20 threads', status: 'in_progress', opened_at: minutes(60 * 4), threads: [{ id: 'side-f', title: 'Capacity harness', state: 'finished' }, { id: 'side-g', title: 'Capacity run', state: 'failed' }] },
    { id: 7, request: 'Run the login suites', status: 'in_progress', opened_at: minutes(25), threads: [{ id: 'side-i', title: 'Login suites', state: 'background' }, { id: 'side-j', title: 'Login fix', state: 'finished' }] },
  ]
  const activity: (BotActivity | { kind: 'commitment'; at: string; title: string; status: string; text?: string; thread_id?: string })[] = [
    { kind: 'commitment', at: minutes(20), title: 'Calmer bot faces', status: 'completed', thread_id: 'side-d' },
    { kind: 'commitment', at: minutes(60 * 24 + 40), title: 'Muse-style identity cards', status: 'cancelled', text: 'He chose option A instead' },
    { kind: 'side_chat', at: minutes(14), running: true, title: 'Notes cleanup', status: 'running', thread_id: 'side-c2' },
    { kind: 'side_chat', at: minutes(30), title: 'Merged both reviews into one plan', status: 'idle', thread_id: 'side-b' },
    { kind: 'file_changed', at: minutes(62), title: 'MEMORY.md' },
    { kind: 'routine', at: minutes(80), title: 'Grok Bot and Dots watch', status: 'ok', thread_id: 'panel', loop_id: 'l-watch' },
    { kind: 'message_sent', at: minutes(60 * 24 + 5), title: 'Priorities Bot', text: 'Which fixes matter this week?' },
    { kind: 'side_chat', at: minutes(60 * 24 + 90), title: 'Jaz bot scheduling design', status: 'idle', thread_id: 'side-c' },
  ]
  // What the bot's threads started in turn, by the thread that started them.
  const started: Record<string, BotStartedThread[]> = {
    'side-a': [
      { kind: 'side_chat', at: minutes(12), title: 'Commitments design review', status: 'idle', thread_id: 'review-a1' },
      { kind: 'side_chat', at: minutes(4), running: true, title: 'Commitments diff review', status: 'running', thread_id: 'review-a2' },
    ],
    'side-b': [
      { kind: 'side_chat', at: minutes(50), title: 'Async spawn design review', status: 'idle', thread_id: 'review-b1' },
      { kind: 'side_chat', at: minutes(40), title: 'Async spawn diff review', status: 'error', thread_id: 'review-b2' },
    ],
    'review-b2': [{ kind: 'side_chat', at: minutes(38), title: 'Retry rule second opinion', status: 'idle', thread_id: 'review-b3' }],
    'side-f': [{ kind: 'side_chat', at: minutes(60 * 4), title: 'Harness design review', status: 'idle', thread_id: 'review-f1' }],
  }
  const schedule = (expr: string) => ({ kind: 'cron', expr, timezone: 'UTC' })
  const routine = (id: string, name: string, expr: string, extra: Partial<Loop> = {}): Loop => ({
    id, bot_id: bot.id, name, prompt: `${name}: check what changed and tell me only what matters.`, schedule: schedule(expr),
    when_missed: 'skip', thread: 'child', status: 'active', runtime: 'acp', created_at: minutes(9000), updated_at: minutes(10), ...extra,
  })
  let loops = [routine('l-heartbeat', 'Heartbeat', '*/30 * * * *', { when_missed: 'catch_up' }), routine('l-watch', 'Grok Bot and Dots watch', '0 10 * * *', { thread: 'main' })]
  const runs: LoopRun[] = [
    { id: 'r2', loop_id: 'l-heartbeat', thread_id: 'run-child', scheduled_for: minutes(30), started_at: minutes(30), finished_at: minutes(29), status: 'ok', created_at: minutes(30) },
    { id: 'r1', loop_id: 'l-heartbeat', scheduled_for: minutes(60), started_at: minutes(60), finished_at: minutes(60), status: 'error', error: 'Gmail sign-in expired', created_at: minutes(60) },
  ]
  const agents: BotFile = { name: 'AGENTS.md', content: '# Jaz Self Improvement Bot\n\nImprove Jaz. Never merge what you have not audited.', version: 'a1', updated_at: '2026-10-09T10:00:00Z' }
  // Written as a bot writes it, so a save that rewrites lines nobody edited
  // (escapes, the final newline) shows.
  const original = '# Memory\n\n- Nothing merges before an audit [Source: review_notes]\n- Augustinas reviews every release\n'
  let memory: BotFile = { name: 'MEMORY.md', content: original, version: 'm1', updated_at: minutes(62) }
  const requests: { method: string; path: string; body?: { content?: string; version?: string; status?: string } }[] = []
  let activityFails = false
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname === '/v1/bots') {
      const sidebar: Bot[] = [
        { ...bot, id: 'asking', name: 'Asking Bot', status: 'running', waiting: true },
        { ...bot, id: 'busy', name: 'Busy Bot', status: 'running' },
      ]
      return new Response(JSON.stringify({ bots: sidebar }), { headers: { 'Content-Type': 'application/json' } })
    }
    if (!url.pathname.startsWith('/v1/bots/panel/') && !url.pathname.startsWith('/v1/loops')) {
      return originalFetch(input, init)
    }
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    requests.push({ method, path: url.pathname, body })
    const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } })
    switch (`${method} ${url.pathname}`) {
      case 'GET /v1/bots/panel/activity':
        if (activityFails) {
          return json({ error: 'Unavailable' }, 503)
        }
        return json({ items: activity, owed, started })
      case 'GET /v1/bots/panel/files':
        return json({ files: [agents, memory] })
      case 'PUT /v1/bots/panel/files/MEMORY.md':
        if (body.version !== memory.version) {
          return json(memory, 409)
        }
        memory = { ...memory, content: body.content, version: memory.version + '+', updated_at: new Date().toISOString() }
        return json(memory)
      case 'GET /v1/loops':
        return json({ loops })
      case 'GET /v1/loops/l-heartbeat':
        return json({ loop: loops[0], runs })
      case 'POST /v1/loops/l-heartbeat/run':
        return json(runs[0])
      case 'PATCH /v1/loops/l-heartbeat':
        loops = loops.map((loop) => (loop.id === 'l-heartbeat' ? { ...loop, ...body } : loop))
        return json(loops[0])
      case 'DELETE /v1/loops/l-heartbeat':
        loops = loops.filter((loop) => loop.id !== 'l-heartbeat')
        return json({ ok: true })
    }
    return json({ error: `unexpected ${method} ${url.pathname}` }, 404)
  }
  const originalConfirm = window.confirm
  window.confirm = () => true

  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:50;display:flex'
  element.className = 'bg-bg'
  document.body.append(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const originalLastBot = localStorage.getItem('jaz.lastBot')
  localStorage.removeItem('jaz.lastBot')
  const rootRoute = createRootRoute()
  const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/bots', component: botsIndexRoute.options.component })
  const botRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/bots/$botId',
    component: () => (
      <>
        <aside className="hidden w-[260px] flex-col sm:flex">
          <BotsPanel mobile={false} />
        </aside>
        <div className="min-w-0 flex-1" />
        <BotDetails
          bot={bot}
          focusName={false}
          working={false}
          threads={[]}
          subagents={[{ key: 'helper', seq: 1, id: 'helper', name: 'Reviewer', status: 'running', started_at_ms: Date.now() - 6 * 60_000, updated_at: minutes(1) }]}
        />
      </>
    ),
  })
  const threadRoute = createRoute({ getParentRoute: () => rootRoute, path: '/sessions/$sessionId', component: () => <p>thread</p> })
  const router = createRouter({ routeTree: rootRoute.addChildren([indexRoute, botRoute, threadRoute]), history: createMemoryHistory({ initialEntries: ['/bots'] }) })
  const root = createRoot(element)
  // The app renders in StrictMode, which mounts every component twice in
  // development; the panel must work the same way here.
  root.render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </StrictMode>,
  )

  const until = async (what: string, check: () => boolean) => {
    const end = Date.now() + 5000
    while (!check()) {
      if (Date.now() > end) {
        throw new Error(`Bot panel: ${what}; path=${router.state.location.pathname}`)
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const text = () => document.body.textContent ?? ''
  const find = (label: string) => [...document.querySelectorAll<HTMLElement>('button, a')].find((item) => (item.getAttribute('aria-label') === label || item.textContent?.trim().startsWith(label)) && item.getBoundingClientRect().height)
  const click = async (label: string) => {
    let target: HTMLElement | undefined
    await until(`no control "${label}"`, () => Boolean(target = find(label)))
    target!.scrollIntoView({ block: 'nearest' })
    await new Promise(requestAnimationFrame)
    const bounds = target!.getBoundingClientRect()
    const x = Math.round(bounds.x + bounds.width / 2)
    const y = Math.round(bounds.y + bounds.height / 2)
    if (!target!.contains(document.elementFromPoint(x, y))) {
      throw new Error(`Bot panel: "${label}" is covered`)
    }
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const type = async (letters: string) => {
    for (const letter of letters) {
      await window.smoke.key(letter)
    }
  }
  const settle = () => new Promise((resolve) => setTimeout(resolve, 350))
  const capture = async (name: string) => {
    await settle()
    await window.smoke.capture(name)
  }
  const wasDark = document.documentElement.classList.contains('dark')
  const dark = (on: boolean) => document.documentElement.classList.toggle('dark', on)
  const editor = () => document.querySelector<HTMLElement>('[role="dialog"] .ProseMirror')
  const taskStates = () => [...document.querySelectorAll('section[aria-label="Tasks"] button > span:last-child')].map((node) => node.textContent)

  try {
    await until('a fresh Bots page did not focus its first bot', () => router.state.location.pathname === '/bots/asking' && element.querySelector('a[href="/bots/asking"]')?.getAttribute('aria-current') === 'page')
    await capture('bot-first-selected')
    localStorage.setItem('jaz.lastBot', 'busy')
    await router.navigate({ to: '/bots' })
    await until('Bots did not restore the remembered selection', () => router.state.location.pathname === '/bots/busy')
    localStorage.setItem('jaz.lastBot', 'removed')
    const listed = client.getQueryData(botsQuery.queryKey)!
    client.setQueryData(botsQuery.queryKey, listed.map((item) => ({ ...item, pinned: item.id === 'busy' ? 1 : 2 })))
    await router.navigate({ to: '/bots' })
    await until('an unavailable remembered bot did not fall back to the first pin', () => router.state.location.pathname === '/bots/busy' && element.querySelector('a[href="/bots/busy"]')?.getAttribute('aria-current') === 'page')
    client.setQueryData(botsQuery.queryKey, [])
    await router.navigate({ to: '/bots' })
    await until('an empty Bots page did not keep its creation control', () => router.state.location.pathname === '/bots' && Boolean(find('New bot')))
    client.setQueryData(botsQuery.queryKey, listed)
    await router.navigate({ to: '/bots/$botId', params: { botId: 'panel' } })
    dark(false)
    await until('Activity did not open with tasks and history', () => find('Activity')?.getAttribute('aria-pressed') === 'true' && text().includes('Open Tasks 7') && text().includes('Learned something new'))
    if (find('Tasks') || !find('Activity')?.textContent?.includes('Activity')) {
      throw new Error('Bot panel: tasks have a separate tab instead of appearing in Activity')
    }
    const states = taskStates()
    if (states.length !== 7 || states.filter((state) => state === 'Active').length !== 6 || states.filter((state) => state === 'Blocked by Human').length !== 1) {
      throw new Error(`Bot panel: tasks should have two visible states, got ${states}`)
    }
    const title = find(owed[0].request)?.querySelector('span.truncate')
    if (!title || title.getBoundingClientRect().height > parseFloat(getComputedStyle(title).lineHeight) + 1 || !title.classList.contains('truncate')) {
      throw new Error('Bot panel: a long task title expands the compact activity row')
    }
    await capture('bot-panel-tasks')
    dark(true)
    await capture('bot-panel-tasks-dark')
    dark(false)
    await window.smoke.resize(390, 844)
    await capture('bot-panel-tasks-phone')
    const tasks = document.querySelector<HTMLElement>('section[aria-label="Tasks"]')!
    for (const task of owed) {
      const row = find(task.request)!
      if (row.textContent !== task.request + (task.status === 'waiting_for_user' ? 'Blocked by Human' : 'Active') || row.getBoundingClientRect().height > 42) {
        throw new Error('Bot panel: a task row shows a subtitle or body instead of one compact title')
      }
    }
    if (tasks.textContent?.includes('Next:') || tasks.textContent?.includes('For you')) {
      throw new Error('Bot panel: task rows repeat work details instead of showing compact labels and status')
    }
    if ([tasks, ...tasks.querySelectorAll('button')].some((node) => node.getBoundingClientRect().right > innerWidth || node.getBoundingClientRect().left < 0)) {
      throw new Error('Bot panel: tasks overflow on a phone')
    }
    for (const label of ['Activity', 'Routines', 'Identity', 'Agent']) {
      const bounds = find(label)!.getBoundingClientRect()
      if (bounds.width < 40 || bounds.height < 40) {
        throw new Error(`Bot panel: ${label} has a small click target (${bounds.width} × ${bounds.height})`)
      }
    }
    await window.smoke.resize(1200, 900)
    await click(owed[0].request)
    await until('a task with one worker did not open its details', () => document.querySelector('[role="dialog"]')?.textContent?.includes('Next: check its report') ?? false)
    const link = (thread: string) => document.querySelector<HTMLElement>(`a[href$="/sessions/${thread}"]`)
    const follows = (row: string, nested: string) => link(row)?.nextElementSibling?.contains(link(nested)) ?? false
    if (!follows('side-a', 'review-a2') || !link('review-a2')?.textContent?.endsWith('Running for 4m')) {
      throw new Error('Bot panel: task details lost the worker descendants or their running state')
    }
    activityFails = true
    await client.invalidateQueries({ queryKey: ['bots', bot.id, 'activity'] })
    await settle()
    if (!document.querySelector('section[aria-label="Tasks"]') || !document.querySelector('[role="dialog"]')?.textContent?.includes('Next: check its report')) {
      throw new Error('Bot panel: a failed background refresh hid the last loaded tasks or their open details')
    }
    activityFails = false
    owed[0].next = 'Deliver the verified report'
    await client.invalidateQueries({ queryKey: ['bots', bot.id, 'activity'] })
    await until('open task details did not refresh', () => document.querySelector('[role="dialog"]')?.textContent?.includes(owed[0].next!) ?? false)
    await click('Commitments design review')
    await until('a task worker did not open', () => router.state.location.pathname === '/sessions/review-a1')
    await router.navigate({ to: '/bots/$botId', params: { botId: 'panel' } })
    await until('the panel did not return to Activity', () => find('Activity')?.getAttribute('aria-pressed') === 'true' && text().includes('Open Tasks 7'))
    await click('Pause prompt suggestions on hidden turns')
    await until('a commitment without a thread did not open its details', () => document.querySelector('[role="dialog"]')?.textContent?.includes('Next: wait for his OK') ?? false)
    await click('Close')
    await until('the commitment did not close', () => !document.querySelector('[role="dialog"]'))
    await click('Capacity test with 20 threads')
    await until('a commitment with several threads did not list them with what they started', () => /^Capacity harnessFinished\|Harness design review.+\|Capacity runFailed$/.test([...document.querySelectorAll('[role="dialog"] a')].map((link) => link.textContent).join('|')))
    await capture('bot-panel-commitment')
    await click('Close')
    await until('the commitment did not close', () => !document.querySelector('[role="dialog"]'))
    await click('Run the login suites')
    await until('a thread working in the background did not show as working', () => [...document.querySelectorAll('[role="dialog"] a')].map((link) => link.textContent).join('|') === 'Login suitesWorking in the background|Login fixFinished' && (document.querySelector('[role="dialog"]')?.textContent?.includes('For you · Active') ?? false))
    await click('Close')
    await until('the commitment did not close', () => !document.querySelector('[role="dialog"]'))
    await click('Activity')
    await until('Activity never listed Now and today', () => text().includes('Now') && text().includes('Today') && text().includes('Learned something new'))
    if (text().includes('Delivered to you') || text().includes('He chose option A instead') || text().includes('Calmer bot faces') || text().includes('Muse-style identity cards')) {
      throw new Error('Bot panel: completed or cancelled task history leaked into Activity')
    }
    find('Merged both reviews into one plan')?.scrollIntoView({ block: 'center' })
    await capture('bot-panel-started')
    if (!follows('side-b', 'review-b1') || !follows('side-b', 'review-b2')) {
      throw new Error('Bot panel: a side chat lost its worker descendants')
    }
    const indent = (thread: string) => link(thread)!.querySelector('svg')!.getBoundingClientRect().left
    if (!(indent('review-b3') > indent('review-b2') + 10) || !link('review-b2')?.textContent?.endsWith('Failed')) {
      throw new Error('Bot panel: worker nesting or failure state is missing')
    }
    if (!text().includes('Reviewer')) {
      throw new Error('Bot panel: the running helper is missing')
    }
    // In the sidebar a bot waiting on the person always has a dot; a working
    // one has its face, and a dot only once effects are off and the face
    // holds still.
    const dotOf = (name: string) => [...document.querySelectorAll('a')].find((link) => link.textContent?.includes(name))?.querySelector('[role="img"]')?.getAttribute('aria-label') ?? null
    await until('the sidebar never listed the bots', () => text().includes('Asking Bot') && text().includes('Busy Bot'))
    if (dotOf('Asking Bot') !== 'Waiting for your answer' || dotOf('Busy Bot') !== null) {
      throw new Error(`Bot panel: sidebar dots are ${dotOf('Asking Bot')} and ${dotOf('Busy Bot')}`)
    }
    const effects = getAppearance().effects
    setAppearance({ effects: false })
    await until('a working bot with a still face showed no dot', () => dotOf('Busy Bot') === 'Running')
    setAppearance({ effects })
    await capture('bot-panel-activity')
    dark(true)
    await capture('bot-panel-activity-dark')
    dark(false)

    await click('Routines')
    await until('routines are not grouped by cadence', () => text().includes('Through the day') && text().includes('Every 30 minutes') && text().includes('Daily'))
    await capture('bot-panel-routines')
    await click('Heartbeat')
    await until('the routine modal is missing its facts', () => text().includes('Catches up with one run') && text().includes('A side chat of its own') && text().includes('Gmail sign-in expired'))
    await capture('bot-panel-routine')
    await click('Run now')
    await until('Run now sent nothing', () => requests.some((request) => request.method === 'POST' && request.path === '/v1/loops/l-heartbeat/run'))
    await click('Pause')
    await until('Pause did not pause and offer Resume', () => requests.some((request) => request.method === 'PATCH' && request.body?.status === 'paused') && Boolean(find('Resume')))
    await click('Delete')
    await until('Delete did not delete and close the routine', () => requests.some((request) => request.method === 'DELETE') && !document.querySelector('[role="dialog"]'))

    await click('Identity')
    const cards = () => [...document.querySelectorAll<HTMLElement>('button')].filter((card) => /Who it is|What it knows/.test(card.textContent ?? ''))
    await until('the identity cards are missing', () => cards().length === 2 && text().includes('Who it is') && text().includes('What it knows'))
    if (cards().some((card) => card.textContent?.includes('.md'))) {
      throw new Error('Bot panel: an identity card shows a file name: ' + cards().map((card) => card.textContent).join(' | '))
    }
    await capture('bot-panel-identity')
    dark(true)
    await capture('bot-panel-identity-dark')
    dark(false)

    await click('Agents')
    await until('AGENTS.md did not open', () => Boolean(editor()))
    if (document.querySelector('[role="dialog"]')!.getBoundingClientRect().width >= window.innerWidth * 0.9) {
      throw new Error('Bot panel: AGENTS.md opened over the whole window instead of in a modal')
    }
    await click('Close')
    await until('AGENTS.md did not close', () => !editor())
    if (requests.some((request) => request.method === 'PUT')) {
      throw new Error('Bot panel: opening and closing a file without editing saved it')
    }

    await click('Memory')
    await until('MEMORY.md did not open as a document', () => editor()?.querySelector('h1')?.textContent === 'Memory' && editor()!.querySelectorAll('li').length === 2)
    await window.smoke.key('Escape')
    await until('MEMORY.md did not close', () => !editor())
    if (requests.some((request) => request.method === 'PUT')) {
      throw new Error('Bot panel: opening and closing a file that ends in a list saved it')
    }
    await click('Memory')
    await until('MEMORY.md did not reopen for editing', () => Boolean(editor()))
    await type('Owls')
    await click('Bold')
    await capture('bot-file-editor')
    dark(true)
    await capture('bot-file-editor-dark')
    dark(false)
    await click('Close')
    await until('closing did not save the edit', () => !editor() && memory.version === 'm1+')
    const save = requests.find((request) => request.method === 'PUT')!
    if (save.body?.version !== 'm1' || save.body.content !== original.replace('every release', 'every releaseOwls')) {
      throw new Error('Bot panel: the edit saved as ' + JSON.stringify(save.body))
    }

    await click('Memory')
    await until('MEMORY.md did not reopen', () => Boolean(editor()))
    memory = { ...memory, content: memory.content + '\n- The bot wrote this meanwhile', version: 'bot-write' }
    await type('Bees')
    await until('a save over the bot\'s newer write was not refused', () => text().includes('changed Memory while you were editing'))
    await capture('bot-file-conflict')
    await click('Keep mine')
    await until('Keep mine did not save over the version now on disk', () => memory.version === 'bot-write+' && !text().includes('while you were editing'))
    if (!memory.content.includes('every releaseOwlsBees')) {
      throw new Error('Bot panel: Keep mine lost the edit: ' + memory.content)
    }
    await window.smoke.key('Escape')
    await until('Escape did not close the editor', () => !editor())

    await click('Memory')
    await until('MEMORY.md did not open a third time', () => Boolean(editor()))
    await type('Cats')
    await router.navigate({ to: '/sessions/$sessionId', params: { sessionId: 'side-a' } })
    await until('leaving the bot with an unsaved edit lost it', () => !editor() && memory.content.includes('BeesCats'))
    await router.navigate({ to: '/bots/$botId', params: { botId: 'panel' } })

    await click('Identity')
    await click('Memory')
    await until('MEMORY.md did not open a fourth time', () => Boolean(editor()))
    memory = { ...memory, content: memory.content + '\n- The bot wrote this too', version: 'bot-write-2' }
    await type('Dogs')
    void router.navigate({ to: '/sessions/$sessionId', params: { sessionId: 'side-a' } })
    await until('leaving with an edit the bot overtook did not stay with the draft', () => Boolean(editor()) && text().includes('while you were editing'))
    await click('Keep mine')
    await until('keeping the draft after a refused departure did not save it', () => memory.version === 'bot-write-2+' && memory.content.includes('CatsDogs'))
    await window.smoke.key('Escape')
    await until('Escape did not close the editor after the save', () => !editor())

    await click('Activity')
    const completed = owed.splice(0)
    await client.invalidateQueries({ queryKey: ['bots', bot.id, 'activity'] })
    await until('completed tasks did not leave the open list', () => text().includes('No open tasks') && !text().includes(completed[0].request))
    owed.push(...completed)
    await client.invalidateQueries({ queryKey: ['bots', bot.id, 'activity'] })
    await until('new tasks did not appear', () => text().includes(completed[0].request))
    element.style.setProperty('--side-panel-width', '420px')
    await capture('bot-panel-wide')

    activity.splice(0)
    for (const parent of Object.keys(started)) {
      delete started[parent]
    }
    for (const motion of [{ threads: [{ id: 'background-worker', title: 'Report worker', state: 'background' as const }] }, { moving: true }]) {
      owed.splice(0, owed.length, { id: 8, request: 'Deliver the report', status: 'in_progress', opened_at: minutes(1), next: 'Check the report', ...motion })
      await client.invalidateQueries({ queryKey: ['bots', bot.id, 'activity'] })
      await until('the moving task did not appear', () => taskStates().includes('Active'))
      owed[0].status = 'waiting_for_user'
      await until('a moving task waited longer than the active polling interval to refresh', () => taskStates().includes('Blocked by Human'))
    }
  } finally {
    if (originalLastBot === null) localStorage.removeItem('jaz.lastBot')
    else localStorage.setItem('jaz.lastBot', originalLastBot)
    dark(wasDark)
    root.unmount()
    element.remove()
    window.fetch = originalFetch
    window.confirm = originalConfirm
  }
}
