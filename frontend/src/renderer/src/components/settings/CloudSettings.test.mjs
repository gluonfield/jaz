import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./CloudSettings.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: ['react', 'react/jsx-runtime', '@/components/*', '@/lib/cloudAccount', '@/lib/cloudSession', '@/lib/connection'],
})
if (!bundle.success) throw new AggregateError(bundle.logs)

const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function renderCloudSettings(connected, error) {
  const workspace = { id: 'ws_example', url: 'https://workspace.example', org_name: 'Example Systems', last_error: error }
  const module = { exports: {} }
  const mocks = {
    '@/components/connection/AddCloudWorkspaceModal': { AddCloudWorkspaceModal: () => null },
    '@/components/settings/TeamMembersCard': { TeamMembersCard: () => null },
    '@/components/sidebar/WorkspaceMark': { WorkspaceMark: () => null },
    '@/components/ui/ActionMenu': { ActionMenu: () => null },
    '@/components/ui/Button': { Button: ({ children }) => createElement('button', null, children) },
    '@/components/ui/toast': { useToast: () => () => {} },
    '@/lib/cloudAccount': {},
    '@/lib/cloudSession': {
      useIsCloudOnly: () => false,
      useCloudSession: () => ({
        active: connected,
        status: connected ? 'connected' : 'disconnected',
        activeWorkspaceId: workspace.id,
        workspaces: [workspace],
      }),
    },
    '@/lib/connection': {},
  }
  evaluate((name) => mocks[name] ?? require(name), module, module.exports)
  return renderToStaticMarkup(createElement(module.exports.CloudSettings))
}

test('Cloud Settings keeps sync errors separate from a request to sign in again', () => {
  const syncing = renderCloudSettings(true, 'Initial workspace sync failed')
  expect(syncing).toContain('Initial workspace sync failed')
  expect(syncing).not.toContain('<button>Sign in</button>')

  const expired = renderCloudSettings(false, 'Session expired. Sign in again.')
  expect(expired).toContain('Example Systems')
  expect(expired).toContain('<button>Sign in</button>')
})
