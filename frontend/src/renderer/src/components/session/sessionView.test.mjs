import { describe, expect, test } from 'bun:test'
import { deriveSessionView } from './sessionView'

const session = {
  id: 'thread',
  slug: 'thread',
  title: 'Thread',
  status: 'running',
  runtime: 'acp',
  runtime_ref: { agent: 'codex', session_id: 'acp-thread' },
  created_at: '2026-07-18T12:00:00Z',
  updated_at: '2026-07-18T12:00:00Z',
}

function planEvent(seq, statuses) {
  return {
    session_id: session.id,
    seq,
    type: 'acp',
    at: `2026-07-18T12:00:0${seq}Z`,
    acp: {
      id: session.id,
      slug: session.slug,
      agent: 'codex',
      session_id: 'acp-thread',
      state: 'running',
      modes: { current_mode_id: 'full-access', plan_mode_id: 'plan' },
      plan: [
        { content: 'Inspect the code', status: statuses[0] },
        { content: 'Fix the bug', status: statuses[1] },
      ],
    },
  }
}

describe('ACP progress', () => {
  test('uses the latest normal-mode plan replacement', () => {
    const data = {
      session,
      messages: [],
      events: [
        planEvent(1, ['in_progress', 'pending']),
        planEvent(2, ['completed', 'in_progress']),
      ],
    }

    const view = deriveSessionView(data, [])

    expect(view.planActive).toBe(false)
    expect(view.panelProgress?.entries.map((entry) => entry.status)).toEqual([
      'completed',
      'in_progress',
    ])
  })

  test('live progress replaces the persisted snapshot', () => {
    const data = {
      session,
      messages: [],
      events: [planEvent(1, ['in_progress', 'pending'])],
    }

    const view = deriveSessionView(data, [planEvent(2, ['completed', 'in_progress'])])

    expect(view.panelProgress?.entries.map((entry) => entry.status)).toEqual([
      'completed',
      'in_progress',
    ])
  })
})
