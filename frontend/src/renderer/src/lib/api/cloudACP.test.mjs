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
  startCloudACPLogin,
  supportsDesktopCloudLogin,
  toACPAuthLogin,
} = await import('./cloudACP')
const { cloudProfileToAgentOption } = await import('./cloudThreads')

const originalFetch = globalThis.fetch

afterEach(() => {
  globalThis.fetch = originalFetch
})

test('cloudACP API routes web device login and isolated desktop OAuth login through unified session contract', async () => {
  const seen = []
  globalThis.fetch = async (url, init) => {
    const parsed = new URL(String(url))
    const method = init?.method || 'GET'
    seen.push(`${method} ${parsed.pathname}`)

    if (method === 'GET' && parsed.pathname === '/v1/acp/profiles') {
      return Response.json({
        profiles: [
          {
            agent: 'codex',
            profile_id: 'acp:codex',
            display_name: 'Codex',
            status: 'connected',
            account_label: 'dev@example.com',
            version: 2,
            updated_at: '2026-11-24T12:00:00Z',
          },
        ],
        supported_agents: [
          {
            agent: 'codex',
            display_name: 'Codex',
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
        session: { id: 'acplog_1', agent: 'codex', status: 'pending', expires_at: '2026-11-24T12:10:00Z' },
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
    if (method === 'POST' && parsed.pathname === '/v1/cloud/acp-profiles/claude/login') {
      return Response.json({
        session: { id: 'dlog_1', agent: 'claude', status: 'pending', expires_at: '2026-11-24T12:10:00Z' },
      })
    }
    if (method === 'GET' && parsed.pathname === '/v1/cloud/acp-profiles/claude/login/dlog_1') {
      return Response.json({
        session: { id: 'dlog_1', agent: 'claude', status: 'completed', expires_at: '2026-11-24T12:10:00Z' },
      })
    }
    if (method === 'DELETE' && parsed.pathname === '/v1/acp/profiles/codex') {
      return Response.json({ deleted: true, agent: 'codex' })
    }
    return new Response('not found', { status: 404 })
  }

  const list = await listCloudACPProfiles()
  expect(list.profiles).toHaveLength(1)

  const codexStart = await startCloudACPLogin('codex', false)
  const codexPoll = await getCloudACPLogin('codex', codexStart.id, false)
  expect(toACPAuthLogin(codexPoll)).toMatchObject({
    id: 'acplog_1',
    agent: 'codex',
    status: 'running',
    auth_url: 'https://auth.openai.com/codex/device',
    auth_code: 'ABCD-1234',
  })

  expect(supportsDesktopCloudLogin('claude')).toBe(true)
  expect(supportsDesktopCloudLogin('kimi')).toBe(true)
  expect(supportsDesktopCloudLogin('codex')).toBe(false)

  const claudeStart = await startCloudACPLogin('claude', true)
  const claudePoll = await getCloudACPLogin('claude', claudeStart.id, true)
  expect(toACPAuthLogin(claudePoll)?.status).toBe('succeeded')

  const deleted = await deleteCloudACPProfile('codex')
  expect(deleted.deleted).toBe(true)

  expect(seen).toEqual([
    'GET /v1/acp/profiles',
    'POST /v1/acp/profiles/codex/login',
    'GET /v1/acp/profiles/codex/login/acplog_1',
    'POST /v1/cloud/acp-profiles/claude/login',
    'GET /v1/cloud/acp-profiles/claude/login/dlog_1',
    'DELETE /v1/acp/profiles/codex',
  ])
})

test('cloudProfileToAgentOption maps personal OAuth ACP profiles with clean labels and agent icons', () => {
  expect(
    cloudProfileToAgentOption({
      profile_id: 'acp:codex',
      provider: 'codex_oauth',
      display_name: 'Codex · dev@example.com',
      agent: 'codex',
      scope: 'user',
    }),
  ).toEqual({
    id: 'acp:codex',
    label: 'Codex · dev@example.com',
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
    label: 'Claude',
    logoAgent: 'claude',
    provider: 'claude_oauth',
  })
})
