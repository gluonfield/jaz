import { expect, test } from 'bun:test'
import { toolCallCategory, toolNameLabel } from '@/components/session/toolCallCategory'

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

test('provider variants retain common tool presentation without a tool name', () => {
  expect(toolCallCategory({ id: 'web-search', kind: 'fetch', raw_input: { variant: 'WebSearch' } })).toBe('web_search')
  expect(toolCallCategory({ id: 'x-search', raw_input: { variant: 'XSearch' } })).toBe('web_search')
  expect(toolCallCategory({ id: 'read', raw_input: { variant: 'ReadFile' } })).toBe('read')
  expect(toolCallCategory({ id: 'list', raw_input: { variant: 'ListDir' } })).toBe('read')
  expect(toolCallCategory({ id: 'command', raw_input: { variant: 'Bash' } })).toBe('command')
})

test('native protocol tool names use their common presentation', () => {
  expect(toolCallCategory({ id: 'command', tool_name: 'exec_command' })).toBe('command')
  expect(toolNameLabel('exec_command')).toBe('Command')
  expect(toolCallCategory({ id: 'stdin', tool_name: 'write_stdin' })).toBe('command')
  expect(toolCallCategory({ id: 'patch', tool_name: 'apply_patch' })).toBe('edit')
  expect(toolCallCategory({ id: 'image', tool_name: 'view_image' })).toBe('image')
  expect(toolCallCategory({ id: 'resource', tool_name: 'read_mcp_resource' })).toBe('read')
})
