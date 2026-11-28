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
  completeCloudACPLogin,
  deleteCloudACPProfile,
  getCloudACPLogin,
  listCloudACPProfiles,
  refreshCloudACPProfile,
  startCloudACPLogin,
  saveCloudACPKey,
  toACPAuthLogin,
} = await import('./cloudACP')
const { cloudProfileToAgentOption } = await import('./cloudThreads')

const originalFetch = globalThis.fetch

afterEach(() => {
  globalThis.fetch = originalFetch
})

test('Cloud ACP routes OAuth sign-in, refresh, and the Grok key to the selected workspace', async () => {
  const seen = []
  const signal = new AbortController().signal
  globalThis.fetch = async (url, init) => {
    const parsed = new URL(String(url))
    const method = init?.method || 'GET'
    expect(new Headers(init?.headers).get('X-Jaz-Cloud-Workspace')).toBe('workspace-a')
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
    const [, , , , agent, action, id, command] = parsed.pathname.split('/')
    if (method === 'POST' && action === 'login' && !id && agent !== 'codex') {
      if (agent === 'antigravity') expect(JSON.parse(init.body)).toEqual({ gcp_project: 'fixture-project', gcp_location: 'global' })
      return Response.json({ session: { id: 'acplog_2', agent, status: 'awaiting_user' } })
    }
    if (method === 'POST' && agent === 'claude' && command === 'complete') {
      expect(JSON.parse(init.body)).toEqual({ code: 'test-code#test-state' })
      return Response.json({ session: { id, agent, status: 'completed' } })
    }
    if (method === 'GET' && agent === 'antigravity' && action === 'login') {
      return Response.json({ session: { id, agent, status: 'completed' } })
    }
    if (method === 'POST' && action === 'refresh') {
      expect(JSON.parse(init.body)).toEqual({ force: true })
      return Response.json({ profile: { agent, status: 'connected', version: 2 } })
    }
    if (method === 'PUT' && parsed.pathname === '/v1/acp/profiles/grok') {
      expect(JSON.parse(init.body)).toEqual({ api_key: 'fake-workspace-key' })
      return Response.json({ profile: { status: 'connected' } })
    }
    if (method === 'DELETE' && parsed.pathname === '/v1/acp/profiles/codex') {
      return Response.json({ deleted: true, agent: 'codex' })
    }
    return new Response('not found', { status: 404 })
  }

  const list = await listCloudACPProfiles(undefined, 'workspace-a')
  expect(list.profiles).toHaveLength(1)

  const codexStart = await startCloudACPLogin('codex', 'workspace-a', signal)
  const codexPoll = await getCloudACPLogin('codex', codexStart.id, 'workspace-a', signal)
  expect(toACPAuthLogin(codexPoll)).toMatchObject({
    id: 'acplog_1',
    agent: 'codex',
    status: 'running',
    auth_url: 'https://auth.openai.com/codex/device',
    auth_code: 'ABCD-1234',
  })

  const claude = await startCloudACPLogin('claude', 'workspace-a', signal)
  const completed = await completeCloudACPLogin('claude', claude.id, 'test-code#test-state', 'workspace-a', signal)
  expect(toACPAuthLogin(completed).status).toBe('succeeded')
  const antigravity = await startCloudACPLogin('antigravity', 'workspace-a', signal, { gcp_project: 'fixture-project', gcp_location: 'global' })
  const googlePoll = await getCloudACPLogin('antigravity', antigravity.id, 'workspace-a', signal)
  expect(toACPAuthLogin(googlePoll).status).toBe('succeeded')
  for (const agent of ['claude', 'antigravity']) {
    expect((await refreshCloudACPProfile(agent, 'workspace-a', signal)).profile.version).toBe(2)
  }
  await saveCloudACPKey('grok', 'fake-workspace-key', 'workspace-a', signal)

  const deleted = await deleteCloudACPProfile('codex', 'workspace-a')
  expect(deleted.deleted).toBe(true)

  expect(seen).toEqual([
    'GET /v1/acp/profiles',
    'POST /v1/acp/profiles/codex/login',
    'GET /v1/acp/profiles/codex/login/acplog_1',
    'POST /v1/acp/profiles/claude/login',
    'POST /v1/acp/profiles/claude/login/acplog_2/complete',
    'POST /v1/acp/profiles/antigravity/login',
    'GET /v1/acp/profiles/antigravity/login/acplog_2',
    'POST /v1/acp/profiles/claude/refresh',
    'POST /v1/acp/profiles/antigravity/refresh',
    'PUT /v1/acp/profiles/grok',
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
