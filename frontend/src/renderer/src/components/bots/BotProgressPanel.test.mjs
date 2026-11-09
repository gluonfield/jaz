import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterContextProvider } from '@tanstack/react-router'
import { renderToStaticMarkup } from 'react-dom/server'
import { BotProgressPanel } from './BotProgressPanel'
import { deriveBotProgress } from '@/lib/botProgress'

const render = (progress, activities = []) => renderToStaticMarkup(createElement(BotProgressPanel, { name: 'meshbot', progress, activities }))

test('activity panel renders phase, safe tool, delegated work and event age on distinct rows', () => {
  const progress = deriveBotProgress({ running: true, now: 1500,
    activity: { phase: 'responding', at: new Date(1400).toISOString() },
    toolCalls: [{ id: 'tool', kind: 'execute', status: 'in_progress', program: 'ssh', updated_at: new Date(1300).toISOString() }],
  })
  const html = render(progress, ['Running: Inspect radio', 'Thread: Firmware worker'])
  expect(html).toContain('Responding')
  expect(html).toContain('terminal · › ssh')
  expect(html).toContain('Running: Inspect radio</p>')
  expect(html).toContain('Thread: Firmware worker</p>')
  expect(html).toContain('Last event: 0s ago')
  expect(html).not.toContain('meshbot is')
})

test('quiet activity retains the last observed phase and explicitly leaves its reason unknown', () => {
  const html = render(deriveBotProgress({ running: true, now: 150000, activity: { phase: 'reasoning', at: new Date(1000).toISOString() } }))
  expect(html).toContain('Waiting for a new agent event')
  expect(html).toContain('Last observed phase: Reasoning')
  expect(html).toContain('The agent has not reported a reason for the pause')
})

test('reported retry reason is shown without a guessed cause', () => {
  const html = render(deriveBotProgress({ running: true, now: 2000, activity: { phase: 'waiting', reason: 'provider_retry', at: new Date(1000).toISOString() } }))
  expect(html).toContain('The provider reported a retry')
  expect(html).not.toContain('The agent has not reported a reason for the pause')
})

test('failed activity stays visible after the running flag ends', () => {
  const progress = deriveBotProgress({ running: false, now: 200000, activity: { phase: 'error', reason: 'provider_error', at: new Date(1000).toISOString() } })
  const html = render(progress)
  expect(html).toContain('Agent error')
  expect(html).toContain('The agent reported an error')
  expect(html).not.toContain('Waiting for a new agent event')
})

test('an active worker shows its latest message next to its progress and opens the worker thread', async () => {
  const root = createRootRoute()
  const routeTree = root.addChildren([createRoute({ getParentRoute: () => root, path: '/sessions/$sessionId' })])
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: ['/sessions/bot'] }) })
  await router.load()
  const progress = deriveBotProgress({
    running: true,
    now: 150000,
    activity: { phase: 'reasoning', at: new Date(1000).toISOString() },
    lastEventAt: new Date(145000).toISOString(),
    plan: [{ status: 'in_progress', content: 'Verify rollback' }],
  })
  const panel = createElement(BotProgressPanel, {
    name: 'jaz bot',
    workers: [{ id: 'child', title: 'Release verification', message: 'The rollback check passed.', progress }],
  })
  const html = renderToStaticMarkup(createElement(RouterContextProvider, { router }, panel))
  expect(html).toContain('href="/sessions/child"')
  expect(html).toContain('Latest agent message')
  expect(html).toContain('The rollback check passed.')
  expect(html).toContain('Plan: Verify rollback')
  expect(html).toContain('Last event: 5s ago')
})
