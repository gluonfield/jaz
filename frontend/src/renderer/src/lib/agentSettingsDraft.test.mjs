import { expect, test } from 'bun:test'
import { agentSettingsDraft, agentSettingsDraftDirty } from './agentSettingsDraft'
import { inputFromSettings } from './agentSettings'

test('live metadata refreshes preserve edits and reverting clears dirty state', () => {
  const server = {
    agents: ['muse'],
    acp: { muse: { enabled: false, model: '' } },
    acp_keys: {},
    acp_auth: { muse: { authenticated: false } },
    acp_options: {},
    providers: [{ id: 'example', configured: false }],
  }
  let draft = agentSettingsDraft(null, { type: 'refresh', settings: server })
  expect(agentSettingsDraftDirty(draft)).toBe(false)
  const edited = {
    ...server,
    acp: { muse: { ...server.acp.muse, model: 'edited-model' } },
    acp_keys: { muse: 'entered-key' },
  }
  draft = agentSettingsDraft(draft, { type: 'edit', settings: edited })
  const refreshed = {
    ...server,
    providers: [{ id: 'example', configured: true }],
    acp_auth: { muse: { authenticated: true } },
    acp_options: { muse: { models: [{ id: 'new-model', reasoning: { efforts: [] } }] } },
  }
  draft = agentSettingsDraft(draft, { type: 'refresh', settings: refreshed })
  const view = { ...draft.server, ...draft.input }
  expect(view.providers[0].configured).toBe(true)
  expect(view.acp_auth.muse.authenticated).toBe(true)
  expect(view.acp_options.muse.models[0].id).toBe('new-model')
  expect(view.acp.muse.model).toBe('edited-model')
  expect(view.acp_keys.muse).toBe('entered-key')
  expect(agentSettingsDraftDirty(draft)).toBe(true)
  expect(Object.keys(inputFromSettings(view)).sort()).toEqual(['acp', 'acp_keys'])
  draft = agentSettingsDraft(draft, { type: 'edit', settings: refreshed })
  expect(agentSettingsDraftDirty(draft)).toBe(false)
  const changedDefaults = { ...refreshed, acp: { muse: { enabled: true, model: 'server-model' } } }
  draft = agentSettingsDraft(draft, { type: 'refresh', settings: changedDefaults })
  expect(draft.input.acp.muse.model).toBe('server-model')
  draft = agentSettingsDraft(draft, { type: 'edit', settings: edited })
  draft = agentSettingsDraft(draft, { type: 'reset', settings: changedDefaults })
  expect(agentSettingsDraftDirty(draft)).toBe(false)
  expect(draft.input.acp.muse.enabled).toBe(true)
})
