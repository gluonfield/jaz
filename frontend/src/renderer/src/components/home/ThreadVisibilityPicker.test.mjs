import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./ThreadVisibilityPicker.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    'lucide-react',
    '@/components/projects/SpaceModal',
    '@/components/ui/Popover',
  ],
})
if (!bundle.success) throw new AggregateError(bundle.logs)

const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

// Mounts the picker with state that survives between renders, so a click can
// be followed by a second render.
function mountPicker(props) {
  const states = []
  let stateCall = 0
  let rows = []
  let trigger
  let modal
  const mod = { exports: {} }
  evaluate((name) => {
    if (name === 'react') {
      return {
        ...React,
        useState: (initial) => {
          const idx = stateCall++
          if (!(idx in states)) states[idx] = initial
          return [states[idx], (next) => {
            states[idx] = typeof next === 'function' ? next(states[idx]) : next
          }]
        },
      }
    }
    if (name === '@/components/projects/SpaceModal') {
      return {
        SpaceModal: (modalProps) => {
          modal = modalProps
          return React.createElement('div', { 'data-space-modal': '' })
        },
      }
    }
    if (name === '@/components/ui/Popover') {
      return {
        MenuRow: ({ role, selected, onClick, children }) => {
          rows.push({ label: renderToStaticMarkup(children), onClick })
          return React.createElement('button', { role, 'aria-checked': selected }, children)
        },
        Popover: ({ open, trigger: button, children }) => {
          trigger = button
          return React.createElement(React.Fragment, null, button, open ? children : null)
        },
      }
    }
    return require(name)
  }, mod, mod.exports)

  const render = () => {
    stateCall = 0
    rows = []
    modal = undefined
    return renderToStaticMarkup(React.createElement(mod.exports.ThreadVisibilityPicker, props))
  }
  render()
  return {
    render,
    modal: () => modal,
    openMenu: () => {
      trigger.props.onClick()
      return render()
    },
    click: (label) => {
      rows.find((row) => row.label.includes(label)).onClick()
      return render()
    },
  }
}

test('a project created from the menu becomes the thread project', () => {
  const changes = []
  const picker = mountPicker({
    spaceId: '',
    spaces: [{ id: 'space-1', name: 'jaz' }],
    onChange: (spaceId) => changes.push(spaceId),
  })
  picker.openMenu()
  const html = picker.click('New project')
  expect(html).toContain('data-space-modal')
  expect(html).not.toContain('role="menu"')
  expect(picker.modal().space).toBeUndefined()

  picker.modal().onCreated({ id: 'space-2', name: 'Launch' })
  picker.modal().onClose()
  expect(changes).toEqual(['space-2'])
  expect(picker.render()).not.toContain('data-space-modal')
})
