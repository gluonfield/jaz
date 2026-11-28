import { expect, spyOn, test } from 'bun:test'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { SidePanelLauncher } from '@/components/session/SidePanelTabs'
import { threadPanelItems } from '@/components/session/threadPanelItems'
import { appsQuery } from '@/lib/api/apps'

const session = { id: 'current', runtime_ref: { cwd: '/workspace' } }
const app = (id, title, source_thread_id, status = 'active') => ({ id, title, source_thread_id, status })
const event = (extra) => ({ session_id: session.id, at: '2026-11-28T08:00:00Z', ...extra })
const page = (nodes = []) => ({ id: 'sketches', name: 'Sketches', nodes, edges: [], files: {}, components: {} })

test('the launcher requests and caches apps by thread and renders only the matching records', async () => {
  const apps = [
    app('policy', 'Policy dashboard', 'other-thread'),
    app('global', 'Workspace app'),
    app('deleted', 'Deleted app', session.id, 'deleting'),
  ]
  const events = [event({
    session_id: 'other-thread',
    type: 'artifact',
    artifact: { kind: 'artifact', artifact: { title: 'Foreign artifact', widget_code: '<svg />' } },
  })]
  const requests = []
  const fetch = spyOn(globalThis, 'fetch').mockImplementation(async (url) => {
    const params = new URL(url).searchParams
    requests.push([...params.entries()])
    return Response.json({ apps })
  })
  const client = new QueryClient()
  const render = (session) => renderToStaticMarkup(createElement(QueryClientProvider, { client },
    createElement(SidePanelLauncher, { session, events, messages: [], live: null, readCanvas: false }),
  ))
  try {
    await client.fetchQuery(appsQuery({ scope: 'mine', sourceThreadId: 'other-thread' }))
    await client.fetchQuery(appsQuery({ scope: 'mine', sourceThreadId: session.id }))
    expect(requests).toEqual([
      [['scope', 'mine'], ['source_thread_id', 'other-thread']],
      [['scope', 'mine'], ['source_thread_id', session.id]],
    ])

    const html = render(session)
    expect(html.match(/<button\b/g)).toHaveLength(4)
    for (const label of ['Files', 'Browser', 'Terminal', 'Code diff']) expect(html).toContain(`>${label}</span>`)
    const otherThread = render({ id: 'other-thread' })
    expect(otherThread).toContain('Policy dashboard')
    expect(otherThread).toContain('Foreign artifact')
  } finally {
    client.clear()
    fetch.mockRestore()
  }
})

test('current thread material keeps its correct panel targets and deduplicates repeated references', () => {
  const linkedAppId = 'l'.repeat(26)
  const inputs = {
    session,
    messages: [{ role: 'assistant', content: [
      '[Launch plan](<docs/Launch plan.md#text=Ship%20date>)',
      '[/fake/Caption.md](<docs/Actual.md>)',
      '[Revenue chart](?page=sketches&item=chart)',
      '[Missing chart](?page=sketches&item=gone)',
      `[Linked app](/apps/${linkedAppId})`,
      '[Other board](/sessions/other-thread?page=sketches&item=chart)',
      '`[Example](docs/never.md)`',
      '```md\n/fake/Nope.md\n```',
    ].join('\n\n'), blocks: [
      { type: 'tool', name: 'show_artifact', input_json: JSON.stringify({ title: 'Preview', html: '<svg />' }) },
      { type: 'tool', name: 'publish_app', result: JSON.stringify({ app_id: 'published', title: 'Published report', kind: 'document' }) },
    ] }],
    events: [
      event({ type: 'artifact', artifact: { kind: 'artifact', artifact: { title: 'Preview', widget_code: '<svg />' } } }),
      event({ type: 'acp_tool', acp: { id: session.id, tool_calls: [
        { tool_name: 'publish_app', raw_output: { draft_id: 'pending', bundle_sha256: 'hash', title: 'Needs approval' } },
        { tool_name: 'show_artifact', status: 'failed', raw_input: { title: 'Failed', html: '<svg />' } },
      ] } }),
    ],
    apps: [app('published', 'Published report', session.id), app('saved', 'Saved from preview', session.id)],
    canvas: { pages: [page([{ id: 'chart', type: 'text', text: 'Revenue' }])] },
  }
  const items = threadPanelItems(inputs)
  expect(items.map(({ label }) => label).sort()).toEqual([
    '/fake/Caption.md', 'Launch plan', 'Linked app', 'Preview', 'Published report', 'Revenue chart', 'Saved from preview', 'Sketches',
  ])
  const tabs = Object.fromEntries(items.map(({ label, tab }) => [label, tab]))
  expect(tabs.Preview).toMatchObject({ kind: 'artifact', artifact: { code: '<svg />' } })
  expect(tabs['Launch plan']).toEqual({ kind: 'file', file: { path: '/workspace/docs/Launch plan.md', location: { text: 'Ship date' } } })
  expect(tabs['/fake/Caption.md']).toEqual({ kind: 'file', file: { path: '/workspace/docs/Actual.md' } })
  expect(tabs['Revenue chart']).toMatchObject({ kind: 'canvas', target: { page: 'sketches', item: 'chart' } })
  expect(tabs['Linked app']).toMatchObject({ kind: 'published-app', app: { id: linkedAppId } })
  expect(tabs['Published report']).toMatchObject({ kind: 'published-app', app: { id: 'published' } })
  expect(threadPanelItems({ ...inputs, session: { ...session, runtime_ref: { artifact_surface: 'canvas' } } }).some(({ tab }) => tab.kind === 'canvas')).toBe(false)
})
