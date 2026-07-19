import { describe, expect, test } from 'bun:test'
import { providerSubagentsFromEvents } from './providerSubagents'

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
    const subagents = providerSubagentsFromEvents([
      subagent('first-child', 'Newton'),
      subagent('second-child', 'Noether'),
    ])

    expect(subagents).toHaveLength(2)
    expect(subagents.map((item) => item.name).sort()).toEqual(['Newton', 'Noether'])
  })
})
