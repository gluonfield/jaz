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

describe('Kimi native auth', () => {
  test('requires a usable model configuration after OAuth', () => {
    const settings = {
      agents: ['kimi'],
      acp: { kimi: { enabled: false } },
      acp_options: { kimi: { supports_auth: true } },
      acp_auth: { kimi: { authenticated: false, ready: false } },
    }
    expect(acpAgentEnableable(settings, 'kimi')).toBe(false)
    settings.acp_auth.kimi.authenticated = true
    expect(acpAgentEnableable(settings, 'kimi')).toBe(false)
    settings.acp_auth.kimi.ready = true
    expect(acpAgentEnableable(settings, 'kimi')).toBe(true)
    settings.acp_auth.kimi.authenticated = false
    expect(acpAgentEnableable(settings, 'kimi')).toBe(false)
  })
})
