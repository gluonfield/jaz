import { describe, expect, test } from 'bun:test'
import { buildOutline, outlineParagraphs } from './outline'

const message = (seq, role, content, at) => ({
  seq,
  role,
  content,
  blocks: [{ type: 'text', text: content }],
  created_at: new Date(at).toISOString(),
})

const event = (content, at, type = 'acp_message') => ({
  session_id: 'thread',
  type,
  content,
  at: new Date(at).toISOString(),
})

describe('outlineParagraphs', () => {
  test('drops fenced code and keeps the prose around it', () => {
    expect(outlineParagraphs('Run this:\n\n```sh\npnpm test\n```\n\nIt passes.')).toEqual([
      'Run this:',
      'It passes.',
    ])
  })

  test('keeps link labels and mention names, drops targets and images', () => {
    expect(outlineParagraphs('Fixed [$review](/skills/review) in ![x](a.png)[AuthProvider.tsx](a.tsx).')).toEqual([
      'Fixed $review in AuthProvider.tsx.',
    ])
  })

  test('strips emphasis and code ticks but never snake_case underscores', () => {
    expect(outlineParagraphs('**Bold** `data_message_seq` stays *intact*.')).toEqual([
      'Bold data_message_seq stays intact.',
    ])
  })

  test('a heading stands alone instead of fusing with the prose under it', () => {
    expect(outlineParagraphs('## Findings\nNo blockers.')).toEqual(['Findings', 'No blockers.'])
  })

  test('list items collapse into one paragraph and table rows drop out', () => {
    expect(outlineParagraphs('Verified:\n- typecheck\n- lint\n\n| a | b |\n| --- | --- |')).toEqual([
      'Verified: typecheck lint',
    ])
  })
})

describe('buildOutline', () => {
  const messages = [
    message(1, 'user', 'There is an issue with homepage login.', 1000),
    message(2, 'user', 'Now review it.', 5000),
  ]
  const events = [
    event('Let me check the auth provider.', 2000),
    event('Fixed the homepage One Tap flow.\n\nGoogleOneTap now refreshes the app auth user.\n\nTrailing detail.', 3000),
    event('No blockers found.', 6000),
  ]

  test('titles each turn by its prompt and previews it with the closing answer', () => {
    const [first, second] = buildOutline(messages, events)
    expect(first.seq).toBe(1)
    expect(first.title).toBe('There is an issue with homepage login.')
    expect(first.preview).toEqual([
      'Fixed the homepage One Tap flow.',
      'GoogleOneTap now refreshes the app auth user.',
    ])
    expect(second.title).toBe('Now review it.')
    expect(second.preview).toEqual(['No blockers found.'])
  })

  test('weight counts every answer block in the turn, not just the last', () => {
    const [first, second] = buildOutline(messages, events)
    expect(first.weight).toBeGreaterThan(second.weight)
  })

  test('reads assistant messages when a thread has no ACP events', () => {
    const native = [message(1, 'user', 'Hi', 1000), message(2, 'assistant', 'Hello there.', 2000)]
    expect(buildOutline(native, [])[0].preview).toEqual(['Hello there.'])
  })

  test('a turn still answering has a title and an empty preview', () => {
    expect(buildOutline([message(1, 'user', 'Pending prompt', 1000)], [])).toEqual([
      { seq: 1, title: 'Pending prompt', preview: [], weight: 0 },
    ])
  })
})
