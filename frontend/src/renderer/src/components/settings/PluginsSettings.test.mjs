import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const siblingExternals = new Set([
  './AddPluginModal',
  './CloudGitHubPluginSection',
  './ConnectionQRModal',
  './MarketplacesModal',
  './PluginDetailModal',
  './pluginFormatting',
  './PluginVisuals',
  './useCloudGitHubPlugin',
  './usePluginSignIn',
])

const bundle = await build({
  entrypoints: [new URL('./PluginsSettings.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    'lucide-react',
    '@tanstack/react-query',
    '@/components/settings/CloudSessionExpiredCard',
    '@/components/ui/Button',
    '@/components/ui/IconButton',
    '@/components/ui/PeekList',
    '@/components/ui/Popover',
    '@/components/ui/SearchField',
    '@/components/ui/Skeleton',
    '@/components/ui/toast',
    '@/lib/api/cloudGitHub',
    '@/lib/api/plugins',
    '@/lib/cloudSession',
    '@/lib/cloudWorkspaceDisplay',
    '@/lib/query/keys',
  ],
  plugins: [
    {
      name: 'externalize-siblings',
      setup(builder) {
        builder.onResolve({ filter: /^\.\// }, (args) => {
          if (siblingExternals.has(args.path)) {
            return { path: args.path, external: true }
          }
          return null
        })
      },
    },
  ],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function renderPluginsSettings({
  isCloudActive,
  role,
  sources = [],
  plugins = [],
}) {
  let refreshCalls = 0
  const effects = []
  const isViewer = Boolean(isCloudActive && role === 'viewer')
  const workspace = isCloudActive
    ? {
        id: 'ws_cas',
        url: 'https://cas.jaz.example',
        org_id: 'org_cas',
        org_name: 'CAS',
        email: `${role}@cas.example`,
        display_name: role,
        role,
      }
    : null

  const mod = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        return {
          ...React,
          useEffect: (fn) => {
            effects.push(fn)
          },
        }
      }
      if (name === '@tanstack/react-query') {
        return {
          useMutation: () => ({ mutate: () => {}, isPending: false, variables: undefined }),
          useQueryClient: () => ({
            invalidateQueries: () => Promise.resolve(),
            setQueryData: () => {},
          }),
        }
      }
      if (name === '@/components/ui/toast') {
        return { useToast: () => () => {} }
      }
      if (name === '@/components/ui/Button') {
        return {
          Button: ({ children, ...props }) => React.createElement('button', props, children),
        }
      }
      if (name === '@/components/ui/IconButton') {
        return {
          IconButton: ({ children, ...props }) => React.createElement('button', props, children),
        }
      }
      if (name === '@/components/ui/PeekList') {
        return { PeekList: ({ items }) => React.createElement('div', null, items) }
      }
      if (name === '@/components/ui/Popover') {
        return {
          Popover: ({ trigger }) => trigger,
          MenuRow: ({ children }) => React.createElement('button', null, children),
        }
      }
      if (name === '@/components/ui/SearchField') {
        return { SearchField: () => React.createElement('input', { type: 'search' }) }
      }
      if (name === '@/components/ui/Skeleton') {
        return { SkeletonRows: () => React.createElement('div') }
      }
      if (name === '@/components/settings/CloudSessionExpiredCard') {
        return { CloudSessionExpiredCard: () => null }
      }
      if (name === '@/lib/api/cloudGitHub') {
        return { isCloudAuthExpiredError: () => false }
      }
      if (name === '@/lib/api/plugins') {
        return {
          addPluginSource: async () => ({}),
          disconnectPluginAccount: async () => ({}),
          installPlugin: async () => ({ plugins: [] }),
          refreshPluginSources: async () => {
            refreshCalls += 1
            return { plugins, sources }
          },
          removePlugin: async () => ({}),
          removePluginSource: async () => ({}),
          setPluginEnabled: async () => ({}),
          updatePlugin: async () => ({ plugins: [] }),
        }
      }
      if (name === '@/lib/cloudSession') {
        return {
          useCloudSession: () => ({
            active: isCloudActive,
            source: isCloudActive ? 'primary' : null,
            status: isCloudActive ? 'connected' : 'disconnected',
            activeWorkspaceId: workspace?.id ?? 'local',
            workspaces: workspace ? [workspace] : [],
            url: workspace?.url ?? '',
            user: workspace
              ? {
                  id: 'usr_1',
                  org_id: 'org_cas',
                  org_name: 'CAS',
                  email: workspace.email,
                  display_name: workspace.display_name,
                  role,
                }
              : null,
            error: null,
            oidcLoginUrl: '/v1/auth/login',
            capabilities: {},
            lastSyncAt: '',
          }),
          useIsCloudViewer: () => isViewer,
        }
      }
      if (name === '@/lib/cloudWorkspaceDisplay') {
        return { activeCloudWorkspace: () => workspace }
      }
      if (name === '@/lib/query/keys') {
        return { keys: { plugins: ['plugins'], mcpServers: ['mcpServers'], mcpApps: ['mcpApps'] } }
      }
      if (name === './AddPluginModal') {
        return { AddPluginModal: () => null }
      }
      if (name === './CloudGitHubPluginSection') {
        return { CloudGitHubPluginSection: () => null }
      }
      if (name === './ConnectionQRModal') {
        return { ConnectionQRModal: () => null }
      }
      if (name === './MarketplacesModal') {
        return { MarketplacesModal: () => null }
      }
      if (name === './PluginDetailModal') {
        return { PluginDetailModal: () => null }
      }
      if (name === './pluginFormatting') {
        return {
          categoryLabel: () => 'Other',
          pluginCanConnect: () => false,
          pluginSummary: () => '',
        }
      }
      if (name === './PluginVisuals') {
        return { PluginAvatar: () => null }
      }
      if (name === './useCloudGitHubPlugin') {
        return {
          CLOUD_GITHUB_ACCOUNT_ID: 'github-default',
          useCloudGitHubPlugin: () => ({
            id: 'github',
            plugin: null,
            statusQuery: {},
            canQuickConnect: false,
            connecting: false,
            disconnecting: false,
            connect: () => {},
            disconnect: () => {},
            startInstallRefresh: () => {},
          }),
        }
      }
      if (name === './usePluginSignIn') {
        return {
          usePluginSignIn: () => ({
            catalog: {
              isPending: false,
              isSuccess: true,
              isError: false,
              error: null,
              data: { plugins, sources },
              refetch: () => Promise.resolve(),
            },
            plugins,
            isConnecting: false,
            connectingPluginID: null,
            start: () => {},
            qrModal: {},
          }),
        }
      }
      return require(name)
    },
    mod,
    mod.exports,
  )

  const html = renderToStaticMarkup(React.createElement(mod.exports.PluginsSettings))
  for (const fn of effects) {
    fn()
  }
  return { html, refreshCalls }
}

test('PluginsSettings skips mount auto-refresh and hides admin refresh controls for Cloud viewers and non-admin members', () => {
  const viewerEmpty = renderPluginsSettings({ isCloudActive: true, role: 'viewer', sources: [] })
  expect(viewerEmpty.refreshCalls).toBe(0)
  expect(viewerEmpty.html).not.toContain('Refresh plugin catalogs')
  expect(viewerEmpty.html).not.toContain('Manage Git marketplaces')

  const viewerStale = renderPluginsSettings({
    isCloudActive: true,
    role: 'viewer',
    sources: [{ id: 'src_1', repository: 'https://git.example/plugins', checked_at: '2020-01-01T00:00:00Z' }],
  })
  expect(viewerStale.refreshCalls).toBe(0)

  const memberEmpty = renderPluginsSettings({ isCloudActive: true, role: 'member', sources: [] })
  expect(memberEmpty.refreshCalls).toBe(0)
  expect(memberEmpty.html).not.toContain('Refresh plugin catalogs')
})

test('PluginsSettings auto-refreshes stale/empty catalogs and exposes refresh controls for Cloud admins and Local sessions', () => {
  const cloudAdmin = renderPluginsSettings({ isCloudActive: true, role: 'admin', sources: [] })
  expect(cloudAdmin.refreshCalls).toBe(1)
  expect(cloudAdmin.html).toContain('Refresh plugin catalogs')
  expect(cloudAdmin.html).toContain('Manage Git marketplaces')

  const localSession = renderPluginsSettings({ isCloudActive: false, role: '', sources: [] })
  expect(localSession.refreshCalls).toBe(1)
  expect(localSession.html).toContain('Refresh plugin catalogs')
})
