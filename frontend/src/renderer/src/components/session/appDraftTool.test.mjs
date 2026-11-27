import { describe, expect, test } from 'bun:test'
import {
  extractAppDraftToolError,
  isAppDraftExpired,
  isAppDraftToolCall,
  isAppDraftToolName,
  parseAppDraftToolOutput,
} from './appDraftTool.ts'

describe('prepare_app_draft tool call parsing', () => {
  test('matches direct, prefixed, and deferred call_tool prepare_app_draft invocations without false positives', () => {
    expect(isAppDraftToolName('prepare_app_draft')).toBe(true)
    expect(isAppDraftToolName('mcp.jaztools.prepare_app_draft')).toBe(true)
    expect(isAppDraftToolName('mcp__jaztools__prepare_app_draft')).toBe(true)
    expect(isAppDraftToolName('visualise_show_widget')).toBe(false)

    expect(
      isAppDraftToolCall({
        tool_name: 'call_tool',
        raw_input: { name: 'prepare_app_draft', arguments: { title: 'Clock' } },
      }),
    ).toBe(true)
    expect(isAppDraftToolCall({ title: 'mcp.jaztools.prepare_app_draft' })).toBe(true)

    // Unrelated tools that happen to carry a `name` argument must NOT be matched.
    expect(
      isAppDraftToolCall({
        tool_name: 'write_file',
        raw_input: { name: 'prepare_app_draft' },
      }),
    ).toBe(false)
  })

  test('parses staged AppDraft, rejects error envelopes, surfaces error messages, and detects expiry', () => {
    const payload = {
      draft_id: 'adraft_123',
      bundle_sha256: 'a'.repeat(64),
      title: 'World Clock',
      kind: 'artifact',
      entry_path: 'index.html',
      size_bytes: 2048,
      file_count: 1,
      files: [{ path: 'index.html', size_bytes: 2048, sha256: 'b'.repeat(64), content_type: 'text/html' }],
      capabilities: ['storage:personal'],
      allowed_tools: [],
      external_urls: [],
      warnings: ['uses inline script'],
      expires_at: '2026-11-27T18:00:00Z',
      requires_user_confirmation: true,
    }

    const fromRaw = parseAppDraftToolOutput(JSON.stringify(payload))
    expect(fromRaw?.draft_id).toBe('adraft_123')
    expect(fromRaw?.title).toBe('World Clock')
    expect(fromRaw?.warnings).toEqual(['uses inline script'])

    const fromCodexEnvelope = parseAppDraftToolOutput({
      error: null,
      result: {
        content: [{ type: 'text', text: JSON.stringify(payload) }],
      },
    })
    expect(fromCodexEnvelope?.draft_id).toBe('adraft_123')
    expect(fromCodexEnvelope?.bundle_sha256).toBe('a'.repeat(64))

    const fromStructured = parseAppDraftToolOutput({
      result: { structuredContent: payload },
    })
    expect(fromStructured?.file_count).toBe(1)

    expect(parseAppDraftToolOutput({ error: 'bundle exceeds limit' })).toBeNull()
    expect(parseAppDraftToolOutput({ isError: true, content: [{ type: 'text', text: 'missing entry' }] })).toBeNull()
    expect(extractAppDraftToolError({ error: { message: 'bundle exceeds limit' } })).toBe('bundle exceeds limit')
    expect(extractAppDraftToolError({ result: { content: [{ type: 'text', text: 'invalid manifest' }] } })).toBe(
      'invalid manifest',
    )

    expect(isAppDraftExpired('2026-11-27T16:00:00Z', Date.parse('2026-11-27T17:00:00Z'))).toBe(true)
    expect(isAppDraftExpired('2026-11-27T18:00:00Z', Date.parse('2026-11-27T17:00:00Z'))).toBe(false)
  })
})
