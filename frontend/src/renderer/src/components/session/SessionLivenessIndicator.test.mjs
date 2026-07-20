import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { SessionLivenessIndicator } from './SessionLivenessIndicator'

function renderIndicator(overrides = {}) {
  return renderToStaticMarkup(
    createElement(SessionLivenessIndicator, {
      agent: 'codex',
      running: true,
      updatedAt: new Date().toISOString(),
      ...overrides,
    }),
  )
}

test('live sessions read "Working - live 1s ago"', () => {
  const html = renderIndicator()
  expect(html).toContain('Working')
  expect(html).toContain('- live 1s ago')
  expect(html).not.toContain('Codex is working')
})

test('live sessions show the breathing bars instead of a spinner', () => {
  const html = renderIndicator()
  expect(html).toContain('live-bars')
  expect(html).not.toContain('animate-spin')
})

test('quiet sessions keep the Working label with a quiet detail', () => {
  const quiet = new Date(Date.now() - 60_000).toISOString()
  const html = renderIndicator({ updatedAt: quiet })
  expect(html).toContain('Working')
  expect(html).toContain('- quiet for 1m')
})

test('stale sessions keep the agent name and alert treatment', () => {
  const stale = new Date(Date.now() - 10 * 60_000).toISOString()
  const html = renderIndicator({ updatedAt: stale })
  expect(html).toContain('Codex is still marked running')
  expect(html).toContain('no updates for 10m')
  expect(html).not.toContain('live-bars')
})
