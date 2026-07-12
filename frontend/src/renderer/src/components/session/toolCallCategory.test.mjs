import { expect, test } from 'bun:test'
import { toolCallCategory } from '@/components/session/toolCallCategory'

test('URLs in filesystem search output do not turn it into a web search', () => {
  expect(
    toolCallCategory({
      id: 'local-search',
      title: 'Search Deployink in .',
      kind: 'search',
      content: [{ type: 'text', text: '```sh\nbackend/plugin.go:8: https://mcp.deployink.com\n```' }],
    }),
  ).toBe('search')
})

test('fetch calls retain web search and page fetch presentation', () => {
  expect(toolCallCategory({ id: 'web-search', kind: 'fetch', tool_name: 'WebSearch' })).toBe('web_search')
  expect(toolCallCategory({ id: 'web-fetch', kind: 'fetch', tool_name: 'WebFetch' })).toBe('web_fetch')
  expect(toolCallCategory({ id: 'legacy-fetch', title: 'Searching for: Deployink', kind: 'fetch' })).toBe('web_fetch')
})
