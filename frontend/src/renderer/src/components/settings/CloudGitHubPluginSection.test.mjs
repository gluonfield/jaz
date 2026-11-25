import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { ApiError } from '@/lib/api/response'

const bundle = await build({
  entrypoints: [new URL('./CloudGitHubPluginSection.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    'lucide-react',
    '@tanstack/react-query',
    '@/components/ui/toast',
    '@/components/ui/Collapse',
    '@/lib/api/cloudGitHub',
    '@/lib/cloudAccount',
    '@/lib/cloudSession',
    '@/lib/connection',
  ],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

const casWorkspace = {
  id: 'ws_cas',
  url: 'https://cas.jaz.example',
  org_id: 'org_cas',
  org_name: 'Cambridge Advanced Systems',
  email: 'admin@cas.example',
  display_name: 'Admin',
  role: 'admin',
}

function renderSection({
  cloudState,
  statusQueryState,
  cloudOnly = false,
  isViewer = false,
}) {
  const mod = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        return React
      }
      if (name === '@tanstack/react-query') {
        return {
          useMutation: () => ({ mutate: () => {}, isPending: false, variables: undefined }),
          useQueryClient: () => ({ invalidateQueries: () => Promise.resolve() }),
        }
      }
      if (name === '@/components/ui/toast') {
        return { useToast: () => () => {} }
      }
      if (name === '@/components/ui/Collapse') {
        return { Collapse: ({ open, children }) => (open ? children : null) }
      }
      if (name === '@/lib/api/cloudGitHub') {
        return {
          DEFAULT_GITHUB_APP_NAME: 'Jaz',
          deleteCloudGitHubInstallation: async () => ({ deleted: true, installation_id: 1 }),
          isCloudAuthExpiredError: (err) => err instanceof ApiError && err.status === 401,
          openSafeGitHubUrl: () => {},
          patchCloudGitHubInstallation: async () => ({}),
          splitRepoFullName: () => null,
          startCloudGitHubInstall: async () => ({ install_url: 'https://github.com', scope: 'org', app_slug: 'jaz' }),
        }
      }
      if (name === '@/lib/cloudAccount') {
        return { attachCloudSignIn: async () => null }
      }
      if (name === '@/lib/cloudSession') {
        return {
          useCloudSession: () => cloudState,
          useIsCloudOnly: () => cloudOnly,
          useIsCloudViewer: () => isViewer,
        }
      }
      if (name === '@/lib/connection') {
        return { startCloudSignIn: async () => null }
      }
      return require(name)
    },
    mod,
    mod.exports,
  )
  return renderToStaticMarkup(
    React.createElement(mod.exports.CloudGitHubPluginSection, {
      statusQuery: {
        isLoading: false,
        isError: false,
        error: null,
        data: undefined,
        refetch: () => Promise.resolve(),
        ...statusQueryState,
      },
    }),
  )
}

test('CloudGitHubPluginSection stays empty when Local workspace is active', () => {
  const html = renderSection({
    cloudState: {
      active: false,
      source: 'attached',
      status: 'disconnected',
      activeWorkspaceId: 'local',
      workspaces: [casWorkspace],
      url: '',
      user: null,
      error: null,
      oidcLoginUrl: '',
      capabilities: {},
      lastSyncAt: '',
    },
    statusQueryState: {
      isError: true,
      error: new ApiError(401, 'not signed in to Jaz Cloud'),
    },
  })

  expect(html).toBe('')
})

test('CloudGitHubPluginSection shows a clean sign-in prompt instead of a red error when Cloud session expires', () => {
  const html = renderSection({
    cloudState: {
      active: true,
      source: 'attached',
      status: 'connected',
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace],
      url: casWorkspace.url,
      user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'admin@cas.example', display_name: 'Admin', role: 'admin' },
      error: null,
      oidcLoginUrl: '/v1/auth/login',
      capabilities: {},
      lastSyncAt: '',
    },
    statusQueryState: {
      isError: true,
      error: new ApiError(401, 'not signed in to Jaz Cloud'),
    },
  })

  expect(html).toContain('Sign in to Jaz Cloud')
  expect(html).toContain('Cambridge Advanced Systems')
  expect(html).toContain('Sign in')
  expect(html).not.toContain('role="alert"')
  expect(html).not.toContain('not signed in to Jaz Cloud')
})

test('CloudGitHubPluginSection shows a quiet non-DevOps notice and zero App-creation UI when unconfigured', () => {
  const html = renderSection({
    cloudState: {
      active: true,
      source: 'attached',
      status: 'connected',
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace],
      url: casWorkspace.url,
      user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'admin@cas.example', display_name: 'Admin', role: 'admin' },
      error: null,
      oidcLoginUrl: '/v1/auth/login',
      capabilities: {},
      lastSyncAt: '',
    },
    statusQueryState: {
      data: {
        configured: false,
        user_linked: false,
        installations: [],
      },
    },
  })

  expect(html).toContain('GitHub is not configured yet')
  expect(html).toContain('deployment operator')
  expect(html).not.toContain('Create Jaz GitHub App')
  expect(html).not.toContain('<input')
})

test('CloudGitHubPluginSection renders install controls for admins and enforces read-only RBAC for viewers', () => {
  const configuredData = {
    configured: true,
    app_name: 'Jaz',
    app_slug: 'jaz',
    user_linked: true,
    github_login: 'octocat',
    install_manage_url: 'https://github.com/apps/jaz/installations/new',
    installations: [
      {
        org_id: 'org_cas',
        installation_id: 42,
        account_login: 'acme-org',
        account_type: 'Organization',
        scope: 'org',
        repository_selection: 'selected',
        allowed_repos: ['acme-org/app'],
        protected_refs: [],
      },
    ],
  }

  const adminHtml = renderSection({
    cloudState: {
      active: true,
      source: 'attached',
      status: 'connected',
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace],
      url: casWorkspace.url,
      user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'admin@cas.example', display_name: 'Admin', role: 'admin' },
      error: null,
      oidcLoginUrl: '/v1/auth/login',
      capabilities: {},
      lastSyncAt: '',
    },
    statusQueryState: { data: configuredData },
  })
  expect(adminHtml).toContain('Repository access')
  expect(adminHtml).toContain('Install on organization')
  expect(adminHtml).toContain('Install on personal account')
  expect(adminHtml).toContain('Manage on GitHub')
  expect(adminHtml).toContain('acme-org')
  expect(adminHtml).not.toContain('Create Jaz GitHub App')

  const viewerHtml = renderSection({
    cloudState: {
      active: true,
      source: 'attached',
      status: 'connected',
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace],
      url: casWorkspace.url,
      user: { id: 'u2', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'viewer@cas.example', display_name: 'Viewer', role: 'viewer' },
      error: null,
      oidcLoginUrl: '/v1/auth/login',
      capabilities: {},
      lastSyncAt: '',
    },
    statusQueryState: { data: configuredData },
    isViewer: true,
  })
  expect(viewerHtml).toContain('View-only access')
  expect(viewerHtml).toContain('acme-org')
  expect(viewerHtml).not.toContain('Install on organization')
  expect(viewerHtml).not.toContain('Install on personal account')
  expect(viewerHtml).not.toContain('Manage on GitHub')
})
