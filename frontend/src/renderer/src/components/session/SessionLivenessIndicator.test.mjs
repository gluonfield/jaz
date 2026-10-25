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

test('live sessions shimmer instead of spinning', () => {
  const html = renderIndicator()
  expect(html).toContain('live-shimmer')
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
  expect(html).not.toContain('live-shimmer')
})

test('reported thinking changes the live label while compaction and stale state take precedence', () => {
  expect(renderIndicator({ activity: 'thinking' })).toContain('Thinking')
  expect(renderIndicator({ activity: 'thinking', activeOperation: 'compact' })).toContain('Compacting')
  expect(renderIndicator({ activity: 'thinking', updatedAt: new Date(Date.now() - 600_000).toISOString() }))
    .toContain('Codex is still marked running')
  expect(renderIndicator({ activity: 'thinking', running: false })).toBe('')
})

test('a starting session names its agent until stale state takes over', () => {
  expect(renderIndicator({ activity: 'starting' })).toContain('Starting Codex')
  expect(renderIndicator({ activity: 'starting', updatedAt: new Date(Date.now() - 600_000).toISOString() }))
    .toContain('Codex is still marked running')
})

test('a provider retry replaces the stale warning with its own status', () => {
  const html = renderIndicator({ retrying: 'Reconnecting... 2/5', updatedAt: new Date(Date.now() - 5 * 60_000).toISOString() })
  expect(html).toContain('Reconnecting... 2/5')
  expect(html).toContain('- 5m ago')
  expect(html).toContain('text-danger')
  expect(html).not.toContain('still marked running')
  expect(renderIndicator({ retrying: 'Reconnecting... 2/5', running: false })).toBe('')
})
