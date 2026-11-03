import { describe, expect, test } from 'bun:test'
import { acpAgentEnableable, selectableACPModelProviders } from './agentRuntimes'

describe('selectableACPModelProviders', () => {
  test('surfaces supported local providers', () => {
    const settings = {
      acp_options: {
        codex: {
          provider_mode: 'agent_defaults',
          model_providers: [
            { id: 'openai', label: 'OpenAI' },
            { id: 'ollama', label: 'Ollama' },
          ],
        },
      },
    }

    expect(selectableACPModelProviders(settings, 'codex').map((provider) => provider.id)).toEqual([
      'openai',
      'ollama',
    ])
  })
})

describe('native agent readiness', () => {
  test.each(['kimi', 'muse'])('%s requires an authenticated, ready runtime', (agent) => {
    const settings = {
      agents: [agent],
      acp: { [agent]: { enabled: false } },
      acp_options: { [agent]: { supports_auth: true } },
      acp_auth: { [agent]: { authenticated: false, ready: false } },
    }
    expect(acpAgentEnableable(settings, agent)).toBe(false)
    settings.acp_auth[agent].authenticated = true
    expect(acpAgentEnableable(settings, agent)).toBe(false)
    settings.acp_auth[agent].ready = true
    expect(acpAgentEnableable(settings, agent)).toBe(true)
    settings.acp_auth[agent].authenticated = false
    expect(acpAgentEnableable(settings, agent)).toBe(false)
  })
})

test('OpenCode native mode requires a ready native profile', () => {
  const settings = {
    agents: ['opencode'],
    acp: { opencode: { enabled: false, model_provider: 'opencode-native' } },
    acp_options: { opencode: { provider_mode: 'agent_defaults', model_providers: [{ id: 'opencode-native', requires_api_key: true }] } },
    acp_auth: { opencode: { authenticated: true, ready: true, auth_evidence: 'api_key_env' } },
  }
  expect(acpAgentEnableable(settings, 'opencode')).toBe(false)
  settings.acp_auth.opencode.auth_evidence = 'opencode_native'
  expect(acpAgentEnableable(settings, 'opencode')).toBe(true)
  settings.acp_auth.opencode.ready = false
  expect(acpAgentEnableable(settings, 'opencode')).toBe(false)
})
