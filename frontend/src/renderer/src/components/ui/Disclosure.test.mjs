import { expect, test } from 'bun:test'
import { parseHTML } from 'linkedom'
import { act, createElement, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { Collapse } from './Collapse'
import { DisclosureTrigger } from './DisclosureTrigger'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
const { document, window } = parseHTML('<html><body></body></html>')
globalThis.document = document
globalThis.window = window

test('mount-on-open collapse skips initially hidden content', () => {
  let renders = 0
  function DeferredContent() {
    renders++
    return createElement('span', null, 'expensive content')
  }

  const closed = renderToStaticMarkup(
    createElement(Collapse, { open: false, mountOnOpen: true }, createElement(DeferredContent)),
  )
  expect(renders).toBe(0)

  const open = renderToStaticMarkup(
    createElement(Collapse, { open: true, mountOnOpen: true }, createElement(DeferredContent)),
  )
  expect(renders).toBe(1)

  const eager = renderToStaticMarkup(createElement(Collapse, { open: false }, createElement(DeferredContent)))

  expect(closed).not.toContain('expensive content')
  expect(open).toContain('expensive content')
  expect(eager).toContain('expensive content')
})

test('mount-on-open collapse retains its child after closing', () => {
  let mounts = 0
  let unmounts = 0
  function TrackedContent() {
    useEffect(() => {
      mounts++
      return () => {
        unmounts++
      }
    }, [])
    return createElement('span', null, 'tracked content')
  }
  const tree = (open) =>
    createElement(Collapse, { open, mountOnOpen: true }, createElement(TrackedContent))

  const container = document.createElement('div')
  const root = createRoot(container)
  act(() => {
    root.render(tree(false))
  })
  expect(mounts).toBe(0)
  expect(container.textContent).toBe('')

  act(() => root.render(tree(true)))
  expect(mounts).toBe(1)
  expect(container.textContent).toBe('tracked content')

  act(() => root.render(tree(false)))
  expect(unmounts).toBe(0)
  expect(container.textContent).toBe('tracked content')

  act(() => root.unmount())
  expect(unmounts).toBe(1)
})

test('disclosure trigger keeps its caret after the label', () => {
  const html = renderToStaticMarkup(
    createElement(DisclosureTrigger, {
      label: 'Worked for 12s',
      open: false,
      onClick: () => {},
    }),
  )

  expect(html.indexOf('Worked for 12s')).toBeLessThan(html.indexOf('lucide-chevron-right'))
  expect(html).toContain('aria-expanded="false"')
})
