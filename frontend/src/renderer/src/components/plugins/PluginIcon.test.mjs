import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { PluginIcon } from './PluginIcon'

test('plugin icons use the shipped SVG as a current-colour mask and a fixed fallback when absent', () => {
  const icon = 'data:image/svg+xml;base64,PHN2Zy8+'
  const rendered = renderToStaticMarkup(createElement(PluginIcon, { icon }))
  expect(rendered).toContain('bg-current')
  expect(rendered).toContain(`mask-image:url(&quot;${icon}&quot;)`)
  expect(rendered).not.toContain('<svg')
  expect(renderToStaticMarkup(createElement(PluginIcon, { icon: '' }))).toContain('lucide-panels-top-left')
})
