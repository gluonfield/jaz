import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

mock.module('./EditDiffBlock', () => ({
  hasInlineDiff: () => true,
  EditDiffBlock: () => createElement('span', null, 'after_unique_preview'),
}))

test('collapsed tool rows leave previews unmounted inside the disclosure', async () => {
  const { ToolCallDetail } = await import('./ToolCallContent')
  const call = {
    id: 'edit-1',
    title: 'Edit example.go',
    status: 'completed',
    kind: 'edit',
    raw_input: { path: 'example.go' },
    content: [
      {
        type: 'diff',
        path: 'example.go',
        old_text: 'before',
        new_text: 'after_unique_preview',
      },
    ],
  }

  const html = renderToStaticMarkup(createElement(ToolCallDetail, { call }))
  expect(html).toContain('grid-rows-[0fr]')
  // The row itself renders, so the absent preview below is a real signal.
  expect(html).toContain('Edit example.go')
  // Closed disclosures don't mount their preview. This also catches a preview
  // escaping the Collapse: outside it, the markup would show up here.
  expect(html).not.toContain('after_unique_preview')
})

test('active tool rows show a spinner instead of the protocol status', async () => {
  const { ToolCallDetail } = await import('./ToolCallContent')
  const call = {
    id: 'search-1',
    title: 'Tool: jaztools/memory_search',
    status: 'in_progress',
    raw_input: { query: 'open questions' },
  }

  const active = renderToStaticMarkup(createElement(ToolCallDetail, { call, active: true }))
  expect(active).toContain('animate-spin')
  expect(active).not.toContain('in_progress')

  const stale = renderToStaticMarkup(createElement(ToolCallDetail, { call }))
  expect(stale).not.toContain('animate-spin')
  expect(stale).not.toContain('in_progress')
})
