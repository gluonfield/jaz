import { describe, expect, test } from 'bun:test'
import { agentAPIKeyCopy, agentLabel, authProviderLabel, onboardingAgentLabel } from './agentLabel'

describe('Kimi labels', () => {
  test('distinguishes the agent, OAuth provider, and CLI', () => {
    expect(agentLabel('kimi')).toBe('Kimi')
    expect(authProviderLabel('kimi')).toBe('Moonshot AI')
    expect(onboardingAgentLabel('kimi')).toBe('Kimi Code')
  })
})

describe('Qwen labels', () => {
  test('distinguishes the agent, subscription, and CLI', () => {
    expect(agentLabel('qwen')).toBe('Qwen')
    expect(authProviderLabel('qwen')).toBe('Qwen Coding Plan')
    expect(onboardingAgentLabel('qwen')).toBe('Qwen Code')
  })

  test('describes its subscription key without offering OAuth', () => {
    expect(agentAPIKeyCopy('qwen', 'Qwen Code', false)).toEqual({
      placeholder: 'Paste your sk-sp-… subscription key',
      description: 'Uses your Alibaba Cloud Coding Plan subscription; Qwen OAuth is discontinued.',
      connected: 'Connected to Qwen Coding Plan',
    })
  })
})
