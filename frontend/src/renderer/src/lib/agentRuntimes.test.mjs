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

describe('Qwen provider auth', () => {
  test('uses the Coding Plan key directly and inherits other provider keys', () => {
    const settings = {
      agents: ['qwen'],
      providers: [
        { id: 'qwen-coding-plan', label: 'Qwen Coding Plan', requires_api_key: true },
        { id: 'qwen-token-plan', label: 'Qwen Token Plan', requires_api_key: true, configured: true },
        { id: 'modelstudio-us', label: 'ModelStudio', requires_api_key: true, configured: true },
      ],
      acp: { qwen: { enabled: false, model_provider: 'qwen-coding-plan' } },
      acp_options: {
        qwen: {
          supports_auth: true,
          provider_mode: 'agent_defaults',
          auth_provider_id: 'qwen-coding-plan',
          model_provider_ids: ['qwen-coding-plan', 'qwen-token-plan', 'modelstudio-us'],
        },
      },
      acp_auth: { qwen: { authenticated: false } },
    }
    expect(acpAgentEnableable(settings, 'qwen')).toBe(false)
    settings.acp_auth.qwen.authenticated = true
    expect(acpAgentEnableable(settings, 'qwen')).toBe(true)
    settings.acp_auth.qwen.authenticated = false
    settings.acp.qwen.model_provider = 'qwen-token-plan'
    expect(acpAgentEnableable(settings, 'qwen')).toBe(true)
    settings.acp.qwen.model_provider = 'modelstudio-us'
    expect(acpAgentEnableable(settings, 'qwen')).toBe(true)
  })
})
