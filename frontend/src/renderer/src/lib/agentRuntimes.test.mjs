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

test.each(['missing', 'downloading', 'failed', 'unsupported'])('provider credentials do not bypass a %s ACP installation', (state) => {
  const settings = {
    acp: { codex: { model_provider: 'openrouter' } },
    acp_options: { codex: { provider_mode: 'agent_defaults', model_providers: [{ id: 'openrouter', configured: true }] } },
    acp_auth: { codex: { managed_adapter: { state } } },
  }
  expect(acpAgentEnableable(settings, 'codex')).toBe(false)
  settings.acp_auth.codex.managed_adapter.state = 'ready'
  expect(acpAgentEnableable(settings, 'codex')).toBe(true)
})
