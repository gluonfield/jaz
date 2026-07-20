import { expect, test } from 'bun:test'
import {
  toolCallCategory,
  toolCallPresentation,
  toolNameLabel,
  toolRunLabel,
} from '@/components/session/toolPresentation'

test('filesystem output containing a URL remains a filesystem search', () => {
  expect(
    toolCallCategory({
      id: 'local-search',
      title: 'Search Deployink in .',
      kind: 'search',
      content: [{ type: 'text', text: 'backend/plugin.go:8: https://mcp.deployink.com' }],
    }),
  ).toBe('search')
})

test('canonical tool names drive common presentation', () => {
  expect(toolCallCategory({ id: 'web-search', tool_name: 'WebSearch' })).toBe('web_search')
  expect(toolCallCategory({ id: 'web-fetch', tool_name: 'WebFetch' })).toBe('web_fetch')
  expect(toolCallCategory({ id: 'command', tool_name: 'exec_command' })).toBe('command')
  expect(toolNameLabel('exec_command')).toBe('Command')
  expect(toolCallCategory({ id: 'stdin', tool_name: 'write_stdin' })).toBe('command')
  expect(toolCallCategory({ id: 'patch', tool_name: 'apply_patch' })).toBe('edit')
  expect(toolCallCategory({ id: 'image', tool_name: 'view_image' })).toBe('image')
  expect(toolCallCategory({ id: 'resource', tool_name: 'read_mcp_resource' })).toBe('read')
})

test('one presentation owns array command parsing and its readable label', () => {
  const presentation = toolCallPresentation({
    id: 'command',
    tool_name: 'exec_command',
    raw_input: { command: ['git', 'status', '--short'] },
  })
  expect(presentation.command).toBe('git status --short')
  expect(presentation.label).toBe('git status --short')
})

test('web result totals remain truthful while the preview stays bounded', () => {
  const presentation = toolCallPresentation({
    id: 'search',
    tool_name: 'WebSearch',
    title: 'tool presentation',
    content: Array.from({ length: 5 }, (_, index) => ({
      type: 'link',
      uri: `https://${index}.example/result`,
      title: `Result ${index}`,
    })),
  })
  expect(presentation.meta).toBe('5 results')
  expect(presentation.preview).toEqual({
    type: 'web_results',
    total: 5,
    items: Array.from({ length: 3 }, (_, index) => ({
      url: `https://${index}.example/result`,
      title: `Result ${index}`,
    })),
  })
})

test('run summaries use the same typed categories as individual rows', () => {
  expect(
    toolRunLabel([
      { id: 'read', tool_name: 'read' },
      { id: 'command', tool_name: 'exec_command' },
      { id: 'failed', tool_name: 'apply_patch', status: 'failed' },
    ]),
  ).toBe('Explored 1 file, ran 1 command, edited 1 file, 1 failed')
})
