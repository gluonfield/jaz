import { afterEach, expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

let inlineDiffs = false
let inlineShellCommands = false

afterEach(() => {
  inlineDiffs = false
  inlineShellCommands = false
})

globalThis.window = {
  jaz: undefined,
  addEventListener: () => {},
  matchMedia: () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }),
}
globalThis.localStorage = {
  getItem: () => null,
  setItem: () => {},
  removeItem: () => {},
}
globalThis.document = {
  documentElement: {
    classList: { add: () => {}, remove: () => {}, toggle: () => {}, contains: () => false },
    style: { setProperty: () => {}, removeProperty: () => {} },
  },
}

mock.module('@/lib/appearance', () => ({
  useInlineDiffs: () => inlineDiffs,
  useInlineShellCommands: () => inlineShellCommands,
}))

mock.module('./MessageMarkdown', () => ({
  MessageMarkdown: ({ text }) => createElement('span', null, text),
  UserMessageMarkdown: ({ text }) => createElement('span', null, text),
}))

mock.module('./Bubble', () => ({
  Bubble: ({ message }) => createElement('span', null, message.content),
}))

mock.module('./LiveEvent', () => ({
  LiveEvent: ({ event }) => createElement('span', null, event.content),
}))

mock.module('./EditDiffBlock', () => ({
  hasInlineDiff: (call) => (call.content ?? []).some(
    (block) => block.type === 'diff' && block.old_text !== block.new_text,
  ),
  EditDiffBlock: ({ call }) => createElement(
    'span',
    null,
    call.content?.find((block) => block.type === 'diff')?.new_text,
  ),
}))

const { ActivityDisclosure } = await import('./ActivityDisclosure')
const { Transcript } = await import('./Transcript')

const thought = (text, key = 'thought') => ({ kind: 'thought', text, key })
const tool = (call, key = `tool-${call.id}`) => ({ kind: 'tool', call, key })

test('the production transcript preserves expanded activity order and ACP headers', () => {
  const at = (seconds) => new Date(seconds * 1000).toISOString()
  const acp = (id, fields) => ({
    session_id: 'thread',
    type: fields.content ? 'acp_message' : 'acp_thought',
    content: fields.content,
    at: at(fields.at),
    acp: {
      id,
      agent: fields.agent ?? id,
      session_id: id,
      state: 'running',
      title: fields.title,
      thought: fields.thought,
      tool_calls: fields.tool_calls,
    },
  })
  const html = renderToStaticMarkup(createElement(Transcript, {
    messages: [{ seq: 1, role: 'user', content: 'prompt', created_at: at(1) }],
    events: [
      acp('thread', { at: 2, thought: 'reasoning-one' }),
      acp('thread', {
        at: 3,
        tool_calls: [{ id: 'read-1', tool_name: 'read', title: 'Read contract' }],
      }),
      acp('thread', { at: 4, content: 'visible-commentary' }),
      acp('other-agent', {
        at: 5,
        agent: 'review-agent',
        title: 'Independent review',
        thought: 'reasoning-two',
      }),
    ],
    sessionId: 'thread',
    groupTurns: true,
    working: true,
    findActive: true,
  }))

  const ordered = [
    'reasoning-one',
    'Read contract',
    'visible-commentary',
    'review-agent',
    'Independent review',
    'reasoning-two',
  ].map((value) => html.indexOf(value))
  expect(ordered.every((index) => index >= 0)).toBe(true)
  expect(ordered).toEqual([...ordered].sort((a, b) => a - b))
})

test('the completed production transcript keeps one work fold with searchable activity detail', () => {
  const at = (seconds) => new Date(seconds * 1000).toISOString()
  const html = renderToStaticMarkup(createElement(Transcript, {
    messages: [{ seq: 1, role: 'user', content: 'prompt', created_at: at(1) }],
    events: [
      {
        session_id: 'thread',
        type: 'acp_thought',
        at: at(2),
        acp: {
          id: 'thread', agent: 'codex', session_id: 'thread', state: 'running',
          thought: 'completed-reasoning',
        },
      },
      {
        session_id: 'thread',
        type: 'acp_tool',
        at: at(3),
        acp: {
          id: 'thread', agent: 'codex', session_id: 'thread', state: 'running',
          tool_calls: [{ id: 'read', tool_name: 'read', title: 'Read finished file' }],
        },
      },
      {
        session_id: 'thread',
        type: 'acp_message',
        content: 'completed-answer',
        at: at(4),
        acp: { id: 'thread', agent: 'codex', session_id: 'thread', state: 'completed' },
      },
    ],
    sessionId: 'thread',
    groupTurns: true,
    findActive: true,
  }))

  expect(html.match(/Worked for/g)).toHaveLength(1)
  const ordered = ['completed-reasoning', 'Read finished file', 'completed-answer']
    .map((value) => html.indexOf(value))
  expect(ordered.every((index) => index >= 0)).toBe(true)
  expect(ordered).toEqual([...ordered].sort((a, b) => a - b))
})

test('inline shell preference keeps commands and output on the transcript axis', () => {
  const entries = [
    thought('reasoning-before-command'),
    tool({
      id: 'command-1',
      kind: 'execute',
      title: 'bun test',
      raw_input: { cmd: 'bun test' },
      content: [{ type: 'text', text: 'unique-shell-output' }],
    }),
  ]

  inlineShellCommands = false
  const folded = renderToStaticMarkup(createElement(ActivityDisclosure, {
    entries,
    findActive: true,
  }))
  expect(folded).not.toContain('unique-shell-output')

  inlineShellCommands = true
  const inline = renderToStaticMarkup(createElement(ActivityDisclosure, {
    entries,
    findActive: true,
  }))
  expect(inline).toContain('reasoning-before-command')
  expect(inline).toContain('unique-shell-output')
})

test('inline diff preference keeps file changes on the transcript axis', () => {
  const entries = [tool({
    id: 'edit-1',
    kind: 'edit',
    title: 'Edit example.go',
    content: [{
      type: 'diff',
      path: 'example.go',
      old_text: 'before_unique_line',
      new_text: 'after_unique_line',
    }],
  })]

  inlineDiffs = false
  const folded = renderToStaticMarkup(createElement(ActivityDisclosure, {
    entries,
    findActive: true,
  }))
  expect(folded).not.toContain('after_unique_line')

  inlineDiffs = true
  const inline = renderToStaticMarkup(createElement(ActivityDisclosure, { entries }))
  expect(inline).toContain('after_unique_line')
})
