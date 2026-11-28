import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./ConnectedMachinesSettings.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    'lucide-react',
    '@tanstack/react-query',
    '@/components/ui/Select',
    '@/components/ui/toast',
    '@/lib/api/machineBridge',
    '@/lib/clientRuntime',
    '@/lib/cloudSession',
    '@/lib/hooks/useCloudCatalogQueries',
  ],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

const thisMachine = {
  device_id: 'jaz-7369041990e9bef4',
  display_name: 'Work MacBook',
  platform: 'darwin',
  online: true,
  capabilities: { file_staging: true },
  folder_grants: [{ id: 'repo', root_path: '/Users/dev/acme', mode: 'ro' }],
  last_seen_at: '2026-11-28T10:00:00Z',
}

const otherMachine = {
  device_id: 'desktop-3f9a1c20',
  display_name: 'Studio PC',
  platform: 'windows',
  online: true,
  capabilities: { file_staging: true, local_exec: true },
  folder_grants: [{ id: 'notes', root_path: 'C:\\Users\\dev\\notes', mode: 'rw' }],
  last_seen_at: '2026-11-28T10:00:00Z',
}

const bridge = {
  device_id: thisMachine.device_id,
  connected: true,
  config: {
    enabled: true,
    display_name: thisMachine.display_name,
    grants: thisMachine.folder_grants,
    allow_exec: false,
  },
}

function renderPage({ machines, localBackend }) {
  const mod = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        return React
      }
      if (name === '@tanstack/react-query') {
        return {
          useMutation: () => ({ mutate: () => {}, isPending: false }),
          useQueryClient: () => ({}),
          useQuery: ({ queryKey, enabled }) =>
            enabled
              ? { data: queryKey[0] === 'bridge' ? bridge : machines, isPending: false, isLoading: false, isError: false, error: null }
              : { data: undefined, isPending: true, isLoading: false, isError: false, error: null },
        }
      }
      if (name === '@/components/ui/Select') {
        return {
          Select: ({ value, options }) => options.find((option) => option.value === value).label,
        }
      }
      if (name === '@/components/ui/toast') {
        return { useToast: () => () => {} }
      }
      if (name === '@/lib/api/machineBridge') {
        return {
          machineBridgeQuery: { queryKey: ['bridge'] },
          machineBridgeQueryKey: ['bridge'],
          isMachineBridgeUnavailable: () => false,
          updateMachineBridge: async (config) => ({ ...bridge, config }),
        }
      }
      if (name === '@/lib/clientRuntime') {
        return { clientRuntime: { capabilities: { localBackend }, pickDirectory: localBackend ? async () => null : undefined } }
      }
      if (name === '@/lib/cloudSession') {
        return {
          useCloudSession: () => ({ url: 'https://workspaces.example.com', active: true }),
          useIsCloudOnly: () => !localBackend,
        }
      }
      if (name === '@/lib/hooks/useCloudCatalogQueries') {
        return { useCloudMachinesQuery: () => ({ queryKey: ['machines'] }) }
      }
      return require(name)
    },
    mod,
    mod.exports,
  )
  return renderToStaticMarkup(React.createElement(mod.exports.ConnectedMachinesSettings))
}

test('this machine appears once, as its own card, even though Cloud also lists it', () => {
  const html = renderPage({ machines: [thisMachine], localBackend: true })

  expect(html.split('Work MacBook')).toHaveLength(2)
  expect(html).toContain('Read-only')
  expect(html).toContain('Add folder')
  expect(html).not.toContain('Other machines')
  expect(html).not.toContain(thisMachine.device_id)
  expect(html).not.toContain('jaz connect-cloud')
})

test('other machines are listed under their own heading with plain access labels', () => {
  const html = renderPage({ machines: [thisMachine, otherMachine], localBackend: true })

  expect(html.split('Work MacBook')).toHaveLength(2)
  expect(html).toContain('Other machines')
  expect(html).toContain('Studio PC')
  expect(html).toContain('Windows · Runs commands')
  expect(html).toContain('Read &amp; write')
  expect(html).not.toContain(otherMachine.device_id)
})

test('without a local backend the page lists machines and opens setup only when there are none', () => {
  const listed = renderPage({ machines: [otherMachine], localBackend: false })
  expect(listed).toContain('Studio PC')
  expect(listed).not.toContain('Share this machine')
  expect(listed).not.toContain('Other machines')
  expect(listed).not.toContain('jaz connect-cloud')

  const empty = renderPage({ machines: [], localBackend: false })
  expect(empty).toContain('jaz connect-cloud')
  expect(empty).toContain('placeholder="Folder path"')
  expect(empty).not.toContain('Add folder')
})
