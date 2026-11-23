import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./ExecutionLocationPicker.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    'lucide-react',
    '@/components/ui/Popover',
    '@/lib/clientRuntime',
    '@/lib/cloudSession',
    '@/lib/connectionDisplay',
  ],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function loadPicker(cloudState, { forcePopoverOpen = false, localBackend = true } = {}) {
  const mod = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        return {
          ...React,
          useState: (initial) => [forcePopoverOpen ? true : (typeof initial === 'function' ? initial() : initial), () => {}],
        }
      }
      if (name === '@/components/ui/Popover') {
        return {
          Popover: ({ open, trigger, children }) =>
            React.createElement(React.Fragment, null, trigger, open ? React.createElement('div', { 'data-popover': 'open' }, children) : null),
        }
      }
      if (name === '@/lib/clientRuntime') {
        return {
          clientRuntime: { capabilities: { localBackend } },
        }
      }
      if (name === '@/lib/cloudSession') {
        return {
          useCloudSession: () => cloudState,
          useHasCloud: () => cloudState.active,
        }
      }
      if (name === '@/lib/connectionDisplay') {
        return {
          describeBackend: (url) => ({ local: false, title: url ? new URL(url).host : '', url }),
        }
      }
      return require(name)
    },
    mod,
    mod.exports,
  )
  return mod.exports.ExecutionLocationPicker
}

const acmeWorkspace = {
  id: 'ws_acme',
  url: 'https://cloud.acme.example',
  org_id: 'o1',
  org_name: 'Acme Org',
  email: 'dev@acme.example',
  display_name: 'Dev',
  role: 'admin',
}

test('ExecutionLocationPicker handles pure Local, Local workspace (with connected Cloud), Cloud workspace, and locked Cloud-only mode', () => {
  // 1. Pure Local (0 Cloud workspaces): Local + Cloud (Sign in…)
  const pureLocal = {
    active: false,
    source: null,
    status: 'disconnected',
    activeWorkspaceId: 'local',
    workspaces: [],
    url: '',
    user: null,
    error: null,
    oidcLoginUrl: '',
    capabilities: {},
    lastSyncAt: '',
  }
  const PureLocalPicker = loadPicker(pureLocal, { forcePopoverOpen: true, localBackend: true })
  const pureLocalHtml = renderToStaticMarkup(
    React.createElement(PureLocalPicker, {
      location: 'local',
      lockedCloud: false,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(pureLocalHtml).toContain('Local')
  expect(pureLocalHtml).toContain('Work in')
  expect(pureLocalHtml).toContain('Sign in…')

  // 2. Local workspace while Cloud workspace(s) exist: strictly Local (no Cloud leakage)
  const localWithCloudConnected = {
    ...pureLocal,
    source: 'attached',
    workspaces: [acmeWorkspace],
  }
  const LocalWorkspacePicker = loadPicker(localWithCloudConnected, { forcePopoverOpen: true, localBackend: true })
  const localWorkspaceHtml = renderToStaticMarkup(
    React.createElement(LocalWorkspacePicker, {
      location: 'local',
      lockedCloud: false,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(localWorkspaceHtml).toContain('Local')
  expect(localWorkspaceHtml).not.toContain('Cloud')
  expect(localWorkspaceHtml).not.toContain('aria-haspopup')

  // 3. Active Cloud workspace on Desktop: strictly Local vs Cloud
  const connectedCloud = {
    active: true,
    source: 'attached',
    status: 'connected',
    activeWorkspaceId: 'ws_acme',
    workspaces: [acmeWorkspace],
    url: 'https://cloud.acme.example',
    user: { id: 'u1', org_id: 'o1', org_name: 'Acme Org', email: 'dev@acme.example', display_name: 'Dev', role: 'admin' },
    error: null,
    oidcLoginUrl: '/v1/auth/login',
    capabilities: { spaces: true },
    lastSyncAt: '',
  }
  const ConnectedPicker = loadPicker(connectedCloud, { forcePopoverOpen: true, localBackend: true })
  const connectedHtml = renderToStaticMarkup(
    React.createElement(ConnectedPicker, {
      location: 'cloud',
      lockedCloud: false,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(connectedHtml).toContain('aria-label="Work in: Cloud"')
  expect(connectedHtml).toContain('Local')
  expect(connectedHtml).toContain('Runs on this machine')
  expect(connectedHtml).toContain('Acme Org')
  expect(connectedHtml).not.toContain('Connected machine')
  expect(connectedHtml).not.toContain('Sign in…')

  // 4. Locked cloud-only mode (via lockedCloud OR localBackend=false): strictly Cloud, zero Local option
  const lockedHtml = renderToStaticMarkup(
    React.createElement(ConnectedPicker, {
      location: 'cloud',
      lockedCloud: true,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(lockedHtml).toContain('Cloud')
  expect(lockedHtml).not.toContain('Local')
  expect(lockedHtml).not.toContain('aria-haspopup')

  const CloudOnlyRuntimePicker = loadPicker(connectedCloud, { forcePopoverOpen: true, localBackend: false })
  const cloudOnlyRuntimeHtml = renderToStaticMarkup(
    React.createElement(CloudOnlyRuntimePicker, {
      location: 'local',
      lockedCloud: false,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(cloudOnlyRuntimeHtml).toContain('Cloud')
  expect(cloudOnlyRuntimeHtml).not.toContain('Local')
  expect(cloudOnlyRuntimeHtml).not.toContain('aria-haspopup')
})
