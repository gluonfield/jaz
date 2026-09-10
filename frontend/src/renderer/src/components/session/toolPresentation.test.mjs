import { expect, test } from 'bun:test'
import codexAgentCalls from '@/components/session/fixtures/codexAgentCalls.json'
import {
  toolCallCategory,
  toolCallPresentation,
  toolNameLabel,
  toolRunLabel,
} from '@/components/session/toolPresentation'

// ACP payloads from codex-acp-app-server's collab-agent-tool-call-flow.json and
// subagent-activity-flow.json snapshots, mapped to Jaz's stored tool-call shape.
test('both Codex agent event formats get a readable label without a tool_name', () => {
  for (const call of codexAgentCalls) {
    expect(toolCallCategory(call)).toBe('agent')
    expect(toolCallPresentation(call).label).toBe('Created an agent')
    expect(toolRunLabel([call])).toBe('Created an agent')
  }
})

test('agent identity survives omitted raw details', () => {
  for (const [name, label] of [
    ['spawnAgent', 'Created an agent'],
    ['wait', 'Waited for an agent'],
    ['started', 'Created an agent'],
    ['completed', 'Agent finished'],
  ]) {
    const call = { id: 'large-input', tool_name: `codex.${name}`, status: 'completed' }
    expect(toolCallCategory(call)).toBe('agent')
    expect(toolRunLabel([call])).toBe(label)
  }
})

test('agent lifecycle rows distinguish the action and its outcome', () => {
  const [call] = codexAgentCalls
  for (const [title, completed, running, failed] of [
    ['spawnAgent', 'Created an agent', 'Creating an agent', 'Failed to create an agent'],
    ['resumeAgent', 'Resumed an agent', 'Resuming an agent', 'Failed to resume an agent'],
    ['sendInput', 'Messaged an agent', 'Messaging an agent', 'Failed to message an agent'],
    ['wait', 'Waited for an agent', 'Waiting for an agent', 'Failed to wait for an agent'],
    ['closeAgent', 'Closed an agent', 'Closing an agent', 'Failed to close an agent'],
  ]) {
    expect(toolRunLabel([{ ...call, title }])).toBe(completed)
    expect(toolRunLabel([{ ...call, title, status: 'in_progress' }])).toBe(running)
    expect(toolRunLabel([{ ...call, title, status: 'failed' }])).toBe(failed)
  }
})

test('new agent activity preserves the provider action and ignores unrelated waits', () => {
  const [collaboration, activity] = codexAgentCalls
  for (const [activityKind, label] of [
    ['interacted', 'Interacted with an agent'],
    ['interrupted', 'Interrupted an agent'],
    ['completed', 'Agent finished'],
  ]) {
    expect(toolRunLabel([{
      ...activity,
      raw_input: { ...activity.raw_input, activityKind },
    }])).toBe(label)
  }
  expect(toolRunLabel([{
    ...collaboration,
    title: 'wait',
    raw_input: { ...collaboration.raw_input, receiverThreadIds: ['first', 'second'] },
  }])).toBe('Waited for 2 agents')
  expect(toolCallCategory({ id: 'wait', title: 'wait', raw_input: { cell_id: '42' } })).toBe('tool')
  expect(toolCallCategory({ id: 'command', kind: 'execute', raw_input: { activityKind: 'started' } })).toBe('command')
  expect(toolCallCategory({ ...activity, raw_input: { ...activity.raw_input, activityKind: 'unknown' } })).toBe('tool')
})

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

test('running protocol statuses stay out of visible tool metadata', () => {
  expect(
    toolCallPresentation({
      id: 'running',
      tool_name: 'memory_search',
      status: 'in_progress',
    }).meta,
  ).toBe('')
})

test('run summaries use the same typed categories as individual rows', () => {
  expect(
    toolRunLabel([
      { id: 'edit', tool_name: 'apply_patch' },
      { id: 'read-1', tool_name: 'read' },
      { id: 'read-2', tool_name: 'read' },
      { id: 'command-1', tool_name: 'exec_command', status: 'failed' },
      { id: 'command-2', tool_name: 'exec_command' },
    ]),
  ).toBe('Edited a file, read files, ran commands, 1 failed')
})
