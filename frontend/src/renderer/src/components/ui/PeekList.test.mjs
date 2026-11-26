import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { PeekList } from './PeekList'

const items = ['Alpha', 'Beta', 'Gamma', 'Delta', 'Epsilon'].map((name) => createElement('button', { key: name }, name))

// Overflow is painted under the fade, so it must stay out of the tab order and
// accessibility tree while the toggle announces what it hides.
test('items past the limit peek inert behind a toggle that names the hidden count', () => {
  const html = renderToStaticMarkup(createElement(PeekList, { items, limit: 3, className: 'grid' }))
  const region = html.match(/<div[^>]*\bid="([^"]+)"[^>]*>/)

  expect(region?.[0]).toContain('inert')
  expect(html.indexOf('Gamma')).toBeLessThan(region?.index ?? -1)
  expect(html.indexOf('Delta')).toBeGreaterThan(region?.index ?? Infinity)
  expect(html).toContain(`aria-controls="${region?.[1]}"`)
  expect(html).toContain('aria-expanded="false"')
  expect(html).toContain('Show 2 more')
})

test('a list within its limit renders every item without a toggle', () => {
  const html = renderToStaticMarkup(createElement(PeekList, { items, limit: Infinity, className: 'grid' }))

  expect(html).toContain('Epsilon')
  expect(html).not.toContain('inert')
  expect(html).not.toContain('aria-expanded')
})
