import { describe, expect, test } from 'bun:test'
import { providerSubagentsFromSources, subagentRunning } from './providerSubagents'

function subagent(owner, name) {
  return {
    session_id: 'parent',
    type: 'provider_subagent',
    projection_key: `provider_subagent:${owner}:codex:worker`,
    provider_subagent: {
      provider: 'codex',
      id: 'worker',
      name,
      status: 'running',
    },
    at: '2026-07-19T12:00:00Z',
  }
}

describe('provider subagent projections', () => {
  test('uses backend identity when provider-local ids collide', () => {
    const subagents = providerSubagentsFromSources(undefined, [
      subagent('first-child', 'Newton'),
      subagent('second-child', 'Noether'),
    ])

    expect(subagents).toHaveLength(2)
    expect(subagents.map((item) => item.name).sort()).toEqual(['Newton', 'Noether'])
  })

  test('keeps complete stored history while merging sparse live updates', () => {
    const subagents = providerSubagentsFromSources(
      [{
        key: 'provider_subagent:codex:/root/newton',
        seq: 10,
        provider: 'codex',
        id: '/root/newton',
        name: 'Newton',
        task: 'Audit the proof',
        status: 'running',
        updated_at: '2026-07-19T12:00:00Z',
      }],
      [{
        session_id: 'parent',
        type: 'provider_subagent',
        provider_subagent: { provider: 'codex', id: '/root/newton', status: 'completed' },
        at: '2026-07-19T13:00:00Z',
      }],
    )

    expect(subagents).toHaveLength(1)
    expect(subagents[0]).toMatchObject({ name: 'Newton', task: 'Audit the proof', status: 'completed' })
  })

  test('does not let an older transcript event regress the complete projection', () => {
    const subagents = providerSubagentsFromSources(
      [{
        key: 'provider_subagent:codex:/root/newton',
        seq: 20,
        provider: 'codex',
        id: '/root/newton',
        name: 'Newton',
        status: 'completed',
        updated_at: '2026-07-19T13:00:00Z',
      }],
      [{
        session_id: 'parent',
        type: 'provider_subagent',
        provider_subagent: { provider: 'codex', id: '/root/newton', status: 'running' },
        seq: 10,
        at: '2026-07-19T12:00:00Z',
      }],
    )

    expect(subagents).toHaveLength(1)
    expect(subagents[0]).toMatchObject({ name: 'Newton', status: 'completed' })
    expect(subagents[0].updated_at).toBe('2026-07-19T13:00:00Z')
  })

  test('shows legacy events without projection keys', () => {
    const subagents = providerSubagentsFromSources(undefined, [{
      session_id: 'parent',
      type: 'provider_subagent',
      provider_subagent: { provider: 'codex', id: '/root/legacy', name: 'Legacy' },
      at: '2026-07-19T12:00:00Z',
    }])

    expect(subagents).toHaveLength(1)
    expect(subagents[0].name).toBe('Legacy')
  })

  test('lists the subagents still at work first and counts only those as running', () => {
    const report = (id, status, at) => ({
      session_id: 'parent',
      type: 'provider_subagent',
      provider_subagent: { provider: 'codex', id, status },
      at,
    })
    const subagents = providerSubagentsFromSources(undefined, [
      report('reader', 'running', '2026-07-19T10:00:00Z'),
      report('newcomer', undefined, '2026-07-19T11:00:00Z'),
      report('writer', 'completed', '2026-07-19T12:00:00Z'),
      report('checker', 'failed', '2026-07-19T13:00:00Z'),
      report('closer', 'cancelled', '2026-07-19T14:00:00Z'),
    ])

    expect(subagents.map((item) => item.id)).toEqual(['newcomer', 'reader', 'closer', 'checker', 'writer'])
    expect(subagents.filter(subagentRunning).map((item) => item.id)).toEqual(['newcomer', 'reader'])
  })
})
