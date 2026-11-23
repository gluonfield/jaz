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
    '@/components/connection/AddCloudWorkspaceModal',
    '@/components/ui/Popover',
    '@/lib/clientRuntime',
    '@/lib/cloudSession',
  ],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function loadPicker(cloudState, { forcePopoverOpen = false, localBackend = true, onSetAdding = () => {} } = {}) {
  const mod = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        let stateCall = 0
        return {
          ...React,
          useState: (initial) => (stateCall++ === 0 ? [forcePopoverOpen, () => {}] : [initial, onSetAdding]),
        }
      }
      if (name === '@/components/connection/AddCloudWorkspaceModal') {
        return { AddCloudWorkspaceModal: () => null }
      }
      if (name === '@/components/ui/Popover') {
        return {
          MenuRow: ({ role, selected, onClick, children }) =>
            React.createElement('button', { role, 'aria-checked': selected, onClick }, children),
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
  // 1. Pure Local (0 Cloud workspaces): single-line Local + Cloud rows
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
    }),
  )
  expect(pureLocalHtml).toContain('aria-label="Work in: Local"')
  expect(pureLocalHtml).toContain('<button role="menuitemradio" aria-checked="true">Local</button>')
  expect(pureLocalHtml).toContain('<button role="menuitemradio" aria-checked="false">Cloud</button>')
  expect(pureLocalHtml).not.toContain('Sign in')

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
    }),
  )
  expect(connectedHtml).toContain('aria-label="Work in: Cloud"')
  expect(connectedHtml).toContain('<button role="menuitemradio" aria-checked="false">Local</button>')
  expect(connectedHtml).toContain('<button role="menuitemradio" aria-checked="true">Cloud</button>')
  expect(connectedHtml).not.toContain('Runs on this machine')
  expect(connectedHtml).not.toContain('Acme Org')

  // 4. Locked cloud-only mode (via lockedCloud OR localBackend=false): strictly Cloud, zero Local option
  const lockedHtml = renderToStaticMarkup(
    React.createElement(ConnectedPicker, {
      location: 'cloud',
      lockedCloud: true,
      onChange: () => {},
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
    }),
  )
  expect(cloudOnlyRuntimeHtml).toContain('Cloud')
  expect(cloudOnlyRuntimeHtml).not.toContain('Local')
  expect(cloudOnlyRuntimeHtml).not.toContain('aria-haspopup')
})

test('Choosing Cloud with no Cloud workspace opens the Add Cloud Workspace modal; with one it only switches', () => {
  const cloudRow = (Picker, onChange) => {
    let row
    const walk = (node) => {
      if (!node || typeof node !== 'object') return
      if (Array.isArray(node)) return node.forEach(walk)
      if (node.props?.role === 'menuitemradio' && node.props.children === 'Cloud') row = node
      walk(node.props?.children)
    }
    walk(Picker({ location: 'local', onChange }))
    return row
  }
  const disconnected = { active: false, status: 'disconnected', workspaces: [], url: '' }
  const connected = { active: true, status: 'connected', workspaces: [acmeWorkspace], url: acmeWorkspace.url }

  for (const [cloudState, expectedAdding] of [[disconnected, [true]], [connected, []]]) {
    const changes = []
    const addingCalls = []
    const Picker = loadPicker(cloudState, { onSetAdding: (value) => addingCalls.push(value) })
    cloudRow(Picker, (next) => changes.push(next)).props.onClick()
    expect(changes).toEqual(['cloud'])
    expect(addingCalls).toEqual(expectedAdding)
  }
})
