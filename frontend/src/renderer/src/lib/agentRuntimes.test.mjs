import { describe, expect, test } from 'bun:test'
import {
  acpAgentChecking,
  acpAgentEnableable,
  acpAgentModelsReady,
  enabledACPAgents,
  enabledACPAgentsChecking,
  hasCheckingACPAgent,
  normalizeACPAgentsEnabled,
  selectableACPModelProviders,
} from './agentRuntimes'

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
  test.each(['kimi', 'muse', 'microsoft_copilot'])('%s requires an authenticated, ready runtime', (agent) => {
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
    settings.acp_auth[agent].ready = false
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


test.each(['copilot', 'claude', 'custom'])('%s discovery trusts server readiness, never draft credentials', (agent) => {
  const settings = {
    acp: { [agent]: { enabled: true } },
    acp_keys: { [agent]: 'new-unsaved-key' },
    acp_options: { [agent]: { local: true } },
    acp_auth: { [agent]: { ready: false } },
  }
  expect(acpAgentModelsReady(settings, agent)).toBe(false)
  settings.acp_auth[agent].ready = true
  expect(acpAgentModelsReady(settings, agent)).toBe(true)
})

test('catalogue readiness follows the requested provider and native auth follows the persisted route', () => {
  const settings = {
    acp: { codex: { model_provider: 'openrouter' } },
    acp_options: { codex: { provider_mode: 'agent_defaults', supports_auth: true, auth_provider_id: 'openai', model_providers: [
      { id: 'openai' },
      { id: 'openrouter', configured: true },
      { id: 'other', requires_api_key: true },
    ] } },
    acp_auth: { codex: { ready: true } },
  }
  expect(acpAgentModelsReady(settings, 'codex', 'openrouter')).toBe(true)
  expect(acpAgentModelsReady(settings, 'codex', 'other')).toBe(false)
  expect(acpAgentModelsReady(settings, 'codex', 'openai')).toBe(false)
  settings.acp.codex.model_provider = 'openai'
  expect(acpAgentModelsReady(settings, 'codex', 'openai')).toBe(true)
  settings.acp_auth.codex.ready = false
  expect(acpAgentModelsReady(settings, 'codex', 'openai')).toBe(false)
  expect(acpAgentModelsReady(settings, 'codex', 'openrouter')).toBe(true)
})

test('saving a newly entered key can still enable the agent before server discovery becomes ready', () => {
  const settings = {
    acp: { claude: { enabled: true } },
    acp_keys: { claude: 'new-unsaved-key' },
    acp_auth: { claude: { ready: false } },
    agents: ['claude'],
  }
  expect(normalizeACPAgentsEnabled(settings).acp.claude.enabled).toBe(true)
  expect(acpAgentModelsReady(settings, 'claude')).toBe(false)
})

test('a slow checking agent does not block fast ready agents from appearing', () => {
  const settings = {
    agents: ['codex', 'claude', 'antigravity'],
    acp: {
      codex: { enabled: true },
      claude: { enabled: true },
      antigravity: { enabled: true },
    },
    acp_auth: {
      codex: { authenticated: true, ready: true },
      claude: { authenticated: true, ready: true },
      antigravity: { authenticated: false, ready: false, checking: true },
    },
  }
  expect(enabledACPAgents(settings)).toEqual(['codex', 'claude'])
  expect(acpAgentChecking(settings, 'antigravity')).toBe(true)
  expect(acpAgentChecking(settings, 'codex')).toBe(false)
  expect(enabledACPAgentsChecking(settings)).toBe(true)
  expect(hasCheckingACPAgent(settings)).toBe(true)

  settings.acp_auth.antigravity = { authenticated: true, ready: true }
  expect(enabledACPAgents(settings)).toEqual(['codex', 'claude', 'antigravity'])
  expect(enabledACPAgentsChecking(settings)).toBe(false)
  expect(hasCheckingACPAgent(settings)).toBe(false)
})
