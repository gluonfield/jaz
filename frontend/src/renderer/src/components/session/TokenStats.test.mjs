import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

mock.module('@/components/ui/Popover', () => ({
  Popover: ({ children }) => createElement('div', null, children),
}))
mock.module('@/lib/models', () => ({
  useContextWindow: () => 1_000_000,
}))

const render = async (usage) => {
  const { TokenStats } = await import('./TokenStats')
  return renderToStaticMarkup(createElement(TokenStats, { session: { usage } }))
}

// Backend fixture: a first turn that cached most of what it sent
// (TestUsageFromRawReadsClaudeAgentPromptResult). Everything here is new
// content, so Input is the whole 23.51k -- not the 6.09k full-rate tail.
test('input counts cache writes, because written tokens are new content', async () => {
  const html = await render({
    input_tokens: 23_514,
    cached_input_tokens: 0,
    cached_write_tokens: 17_424,
    output_tokens: 15,
  })

  expect(html).toContain('23.51k')
  expect(html).not.toContain('6.09k')
})

test('input excludes cache reads, so a long cached thread shows what it actually sent', async () => {
  const html = await render({
    input_tokens: 16_055_720,
    cached_input_tokens: 15_540_000,
    cached_write_tokens: 515_560,
    output_tokens: 168_880,
    context_tokens: 338_410,
  })

  // 16,055,720 - 15,540,000 = 515,720 new tokens across the thread.
  expect(html).toContain('515.72k')
  // Neither the uncached-tail reading (160) nor the raw inclusive one (16.06M).
  expect(html).not.toContain('>160<')
  expect(html).not.toContain('16.06M')
  // Input + cache read + output add up to Total.
  expect(html).toContain('16.22M')
  expect(html).toContain('Cache write counts inside Input.')
})
