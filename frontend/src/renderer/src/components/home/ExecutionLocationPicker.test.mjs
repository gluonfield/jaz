import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./ExecutionLocationPicker.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: ['react', 'react/jsx-runtime', 'react/jsx-dev-runtime', 'lucide-react', '@/components/ui/Popover', '@/lib/cloudSession', '@/lib/connectionDisplay'],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function loadPicker(cloudState, { forcePopoverOpen = false } = {}) {
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

test('ExecutionLocationPicker renders This computer, Cloud (connected org vs Sign in…), and locked Cloud-only mode', () => {
  const disconnectedCloud = {
    active: false,
    source: null,
    status: 'disconnected',
    url: '',
    user: null,
    error: null,
    oidcLoginUrl: '',
    capabilities: {},
  }
  const DisconnectedPicker = loadPicker(disconnectedCloud, { forcePopoverOpen: true })
  const disconnectedHtml = renderToStaticMarkup(
    React.createElement(DisconnectedPicker, {
      location: 'local',
      lockedCloud: false,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(disconnectedHtml).toContain('This computer')
  expect(disconnectedHtml).toContain('Work in')
  expect(disconnectedHtml).toContain('Sign in…')

  const connectedCloud = {
    active: true,
    source: 'attached',
    status: 'connected',
    url: 'https://cloud.acme.example',
    user: { id: 'u1', org_id: 'o1', org_name: 'Acme Org', email: 'dev@acme.example', display_name: 'Dev', role: 'admin' },
    error: null,
    oidcLoginUrl: '/v1/auth/login',
    capabilities: { spaces: true },
  }
  const ConnectedPicker = loadPicker(connectedCloud, { forcePopoverOpen: true })
  const connectedHtml = renderToStaticMarkup(
    React.createElement(ConnectedPicker, {
      location: 'cloud',
      lockedCloud: false,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(connectedHtml).toContain('aria-label="Work in: Cloud"')
  expect(connectedHtml).toContain('Acme Org')
  expect(connectedHtml).not.toContain('Sign in…')

  const lockedHtml = renderToStaticMarkup(
    React.createElement(ConnectedPicker, {
      location: 'cloud',
      lockedCloud: true,
      onChange: () => {},
      onConnectCloud: () => {},
    }),
  )
  expect(lockedHtml).toContain('Cloud')
  expect(lockedHtml).not.toContain('aria-haspopup')
})
