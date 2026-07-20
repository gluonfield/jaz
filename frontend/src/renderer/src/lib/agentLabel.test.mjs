import { describe, expect, test } from 'bun:test'
import { agentLabel, authProviderLabel, onboardingAgentLabel } from './agentLabel'

describe('Kimi labels', () => {
  test('distinguishes the agent, OAuth provider, and CLI', () => {
    expect(agentLabel('kimi')).toBe('Kimi')
    expect(authProviderLabel('kimi')).toBe('Moonshot AI')
    expect(onboardingAgentLabel('kimi')).toBe('Kimi Code')
  })
})
