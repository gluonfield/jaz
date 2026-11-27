import { expect, test } from 'bun:test'
import { isAppDraftExpired, isAppToolCall, parseAppToolOutput } from './appTool.ts'

test('both app tools are recognised directly, prefixed and through deferred call_tool', () => {
  for (const tool of ['publish_app', 'prepare_app_draft']) {
    expect(isAppToolCall({ tool_name: tool })).toBe(true)
    expect(isAppToolCall({ tool_name: `mcp__jaztools__${tool}` })).toBe(true)
    expect(isAppToolCall({ title: `mcp.jaztools.${tool}` })).toBe(true)
    expect(isAppToolCall({ tool_name: 'call_tool', raw_input: { name: tool, arguments: { title: 'Clock' } } })).toBe(true)
    // An unrelated tool that happens to carry a `name` argument is not an app tool.
    expect(isAppToolCall({ tool_name: 'write_file', raw_input: { name: tool } })).toBe(false)
  }
  expect(isAppToolCall({ tool_name: 'visualise_show_widget' })).toBe(false)
})

test('a published app and a staged draft are read from every agent envelope, and errors from none', () => {
  const published = { app_id: 'a'.repeat(26), title: 'World Clock', kind: 'artifact', version: 1, visibility: 'private' }
  const app = { app: { id: published.app_id, title: 'World Clock', kind: 'artifact' } }
  expect(parseAppToolOutput(JSON.stringify(published))).toEqual(app)
  expect(parseAppToolOutput({ error: null, result: { content: [{ type: 'text', text: JSON.stringify(published) }] } })).toEqual(app)
  expect(parseAppToolOutput({ result: { structuredContent: published } })).toEqual(app)

  const staged = {
    draft_id: 'adraft_123',
    bundle_sha256: 'a'.repeat(64),
    title: 'Summariser',
    kind: 'document',
    capabilities: ['storage:personal', 'ai:complete'],
    allowed_tools: [],
    expires_at: '2026-11-27T18:00:00Z',
    requires_user_confirmation: true,
  }
  expect(parseAppToolOutput(JSON.stringify(staged))).toEqual({
    draft: {
      draft_id: 'adraft_123',
      bundle_sha256: 'a'.repeat(64),
      target_app_id: undefined,
      title: 'Summariser',
      kind: 'document',
      capabilities: ['storage:personal', 'ai:complete'],
      allowed_tools: [],
      expires_at: '2026-11-27T18:00:00Z',
    },
  })

  expect(parseAppToolOutput({ error: 'bundle exceeds limit' })).toBeNull()
  expect(parseAppToolOutput({ isError: true, content: [{ type: 'text', text: JSON.stringify(published) }] })).toBeNull()
  expect(parseAppToolOutput(undefined)).toBeNull()

  expect(isAppDraftExpired('2026-11-27T16:00:00Z', Date.parse('2026-11-27T17:00:00Z'))).toBe(true)
  expect(isAppDraftExpired('2026-11-27T18:00:00Z', Date.parse('2026-11-27T17:00:00Z'))).toBe(false)
})
