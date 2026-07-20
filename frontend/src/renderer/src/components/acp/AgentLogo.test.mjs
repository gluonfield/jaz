import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'

describe('Kimi agent logo', () => {
  test('uses the shared monochrome brand-mark path', () => {
    expect(hasAgentLogo('kimi')).toBe(true)
    expect(hasAgentLogo('KIMI')).toBe(true)

    const html = renderToStaticMarkup(createElement(AgentLogo, { agent: 'kimi' }))
    expect(html).toContain('viewBox="0 0 24 24"')
    expect(html).toContain('stroke="currentColor"')
  })
})
