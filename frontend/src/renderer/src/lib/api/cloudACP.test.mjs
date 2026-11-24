import { afterEach, expect, test } from 'bun:test'

if (typeof globalThis.window === 'undefined') {
  globalThis.window = {
    location: { origin: 'http://127.0.0.1:4096', protocol: 'http:', href: 'http://127.0.0.1:4096/' },
    localStorage: {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    },
    addEventListener: () => {},
    removeEventListener: () => {},
  }
}

const {
  deleteCloudACPProfile,
  getCloudACPLogin,
  listCloudACPProfiles,
  refreshCloudACPProfile,
  startCloudACPLogin,
  syncLocalACPProfileToCloud,
} = await import('./cloudACP')
const { cloudProfileToAgentOption } = await import('./cloudThreads')

const originalFetch = globalThis.fetch

afterEach(() => {
  globalThis.fetch = originalFetch
})

test('cloudACP API calls expected endpoints and maps payloads', async () => {
  const seen = []
  globalThis.fetch = async (url, init) => {
    const parsed = new URL(String(url))
    const method = init?.method || 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    seen.push({ method, path: parsed.pathname, body })

    if (method === 'GET' && parsed.pathname === '/v1/acp/profiles') {
      return Response.json({
        profiles: [
          {
            agent: 'codex',
            profile_id: 'acp:codex',
            display_name: 'Codex (Personal)',
            status: 'connected',
            account_label: 'dev@example.com',
            version: 2,
            updated_at: '2026-11-24T12:00:00Z',
          },
        ],
        supported_agents: [
          {
            agent: 'codex',
            display_name: 'Codex (Personal)',
            profile_id: 'acp:codex',
            provider: 'codex_oauth',
            default_model: 'gpt-6.1-sol',
            models: ['gpt-6.1-sol'],
            default_effort: 'high',
            supports_web_auth: true,
            web_auth_kind: 'device_code',
          },
        ],
      })
    }
    if (method === 'POST' && parsed.pathname === '/v1/acp/profiles/codex/login') {
      return Response.json({
        session: {
          id: 'acplog_1',
          agent: 'codex',
          status: 'pending',
          expires_at: '2026-11-24T12:10:00Z',
        },
      })
    }
    if (method === 'GET' && parsed.pathname === '/v1/acp/profiles/codex/login/acplog_1') {
      return Response.json({
        session: {
          id: 'acplog_1',
          agent: 'codex',
          status: 'awaiting_user',
          verification_url: 'https://auth.openai.com/codex/device',
          user_code: 'ABCD-1234',
          expires_at: '2026-11-24T12:10:00Z',
        },
      })
    }
    if (method === 'POST' && parsed.pathname === '/v1/acp/profiles/codex/refresh') {
      return Response.json({
        queued: true,
        profile: { agent: 'codex', profile_id: 'acp:codex', display_name: 'Codex (Personal)', status: 'refreshing', version: 2, updated_at: '2026-11-24T12:01:00Z' },
      })
    }
    if (method === 'DELETE' && parsed.pathname === '/v1/acp/profiles/codex') {
      return Response.json({ deleted: true, agent: 'codex' })
    }
    if (method === 'POST' && parsed.pathname === '/v1/cloud/acp-profiles/claude/sync-local') {
      return Response.json({
        profile: { agent: 'claude', profile_id: 'acp:claude', display_name: 'Claude Code (Personal)', status: 'connected', version: 1, updated_at: '2026-11-24T12:02:00Z' },
        cleared_local: true,
        owned_by_jaz: true,
      })
    }
    return new Response('not found', { status: 404 })
  }

  const list = await listCloudACPProfiles()
  expect(list.profiles).toHaveLength(1)
  expect(list.supported_agents[0].supports_web_auth).toBe(true)

  const started = await startCloudACPLogin('codex')
  expect(started.id).toBe('acplog_1')

  const polled = await getCloudACPLogin('codex', 'acplog_1')
  expect(polled.user_code).toBe('ABCD-1234')

  const refreshed = await refreshCloudACPProfile('codex', true)
  expect(refreshed.queued).toBe(true)

  const deleted = await deleteCloudACPProfile('codex')
  expect(deleted.deleted).toBe(true)

  const synced = await syncLocalACPProfileToCloud('claude', { clearLocal: true })
  expect(synced.cleared_local).toBe(true)
  expect(synced.profile.profile_id).toBe('acp:claude')

  expect(seen.map((s) => `${s.method} ${s.path}`)).toEqual([
    'GET /v1/acp/profiles',
    'POST /v1/acp/profiles/codex/login',
    'GET /v1/acp/profiles/codex/login/acplog_1',
    'POST /v1/acp/profiles/codex/refresh',
    'DELETE /v1/acp/profiles/codex',
    'POST /v1/cloud/acp-profiles/claude/sync-local',
  ])
  expect(seen[5].body).toEqual({ clear_local: true })
})

test('cloudProfileToAgentOption maps personal OAuth ACP profiles with proper agent icons', () => {
  expect(
    cloudProfileToAgentOption({
      profile_id: 'acp:codex',
      provider: 'codex_oauth',
      display_name: 'Codex (Personal) · dev@example.com',
      agent: 'codex',
      scope: 'user',
    }),
  ).toEqual({
    id: 'acp:codex',
    label: 'Codex (Personal) · dev@example.com',
    logoAgent: 'codex',
    provider: 'codex_oauth',
  })

  expect(
    cloudProfileToAgentOption({
      profile_id: 'acp:claude',
      provider: 'claude_oauth',
      agent: 'claude',
      scope: 'user',
    }),
  ).toEqual({
    id: 'acp:claude',
    label: 'Claude Code (Personal)',
    logoAgent: 'claude',
    provider: 'claude_oauth',
  })
})
