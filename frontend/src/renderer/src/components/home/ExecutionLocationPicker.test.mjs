import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./ExecutionLocationPicker.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: ['react', 'react/jsx-runtime', 'react/jsx-dev-runtime', 'lucide-react', '@/components/ui/Popover'],
})
if (!bundle.success) throw new AggregateError(bundle.logs)

const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)
const mod = { exports: {} }
evaluate((name) => name === '@/components/ui/Popover' ? {
  MenuRow: ({ role, selected, onClick, children }) => React.createElement('button', { role, 'aria-checked': selected, onClick }, children),
  Popover: ({ trigger, children }) => React.createElement(React.Fragment, null, trigger, children),
} : require(name), mod, mod.exports)

const { ExecutionLocationPicker } = mod.exports

function renderPicker(props) {
  return renderToStaticMarkup(React.createElement(ExecutionLocationPicker, {
    location: 'cloud',
    localAvailable: true,
    machines: [],
    onChange: () => {},
    ...props,
  }))
}

test('the Cloud workspace picker presents exactly two execution targets', () => {
  const html = renderPicker({ location: 'local' })
  expect(html).toContain('aria-label="Work in: This machine"')
  expect(html).toContain('<button role="menuitemradio" aria-checked="true">This machine</button>')
  expect(html).toContain('<button role="menuitemradio" aria-checked="false">Cloud</button>')
})

test('peer tools remain visible when the Cloud workspace supports only Cloud execution', () => {
  const html = renderPicker({
    localAvailable: false,
    machines: [{ device_id: 'workstation', display_name: 'Workstation' }],
  })
  expect(html).toContain('<button role="menuitemradio" aria-checked="true">Cloud</button>')
  expect(html).not.toContain('This machine')
  expect(html).toContain('aria-label="Machines available to Cloud tools"')
  expect(html).toMatch(/<li[^>]*>Workstation<\/li>/)
  expect(html).not.toMatch(/<button[^>]*>Workstation<\/button>/)
  expect(renderPicker({ localAvailable: false })).not.toContain('aria-haspopup')
})
