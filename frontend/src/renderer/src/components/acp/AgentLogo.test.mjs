import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'

describe('Kimi agent logo', () => {
  test('uses the adapted Kimi mark through the shared monochrome path', () => {
    expect(hasAgentLogo('kimi')).toBe(true)
    expect(hasAgentLogo('KIMI')).toBe(true)

    const html = renderToStaticMarkup(createElement(AgentLogo, { agent: 'kimi' }))
    expect(html).toContain('<svg')
    expect(html).toContain('fill="currentColor"')
    expect(html).not.toContain('<img')
  })
})
