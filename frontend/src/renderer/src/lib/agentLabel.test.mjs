import { describe, expect, test } from 'bun:test'
import { agentLabel, authProviderLabel, onboardingAgentLabel } from './agentLabel'

describe('Microsoft Copilot Work/School display labels', () => {
  test('uses exact Microsoft Copilot Work/School display name distinct from GitHub Copilot', () => {
    expect(agentLabel('microsoft_copilot')).toBe('Microsoft Copilot Work/School')
    expect(onboardingAgentLabel('microsoft_copilot')).toBe('Microsoft Copilot Work/School')
    expect(authProviderLabel('microsoft_copilot')).toBe('Microsoft Work/School')
    expect(agentLabel('copilot')).toBe('GitHub Copilot')
    expect(authProviderLabel('copilot')).toBe('GitHub')
  })
})
