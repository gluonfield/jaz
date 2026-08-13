import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

mock.module('@/lib/api/skills', () => ({
  skillsQuery: () => ({ queryKey: ['skills'], queryFn: async () => [] }),
}))
mock.module('./mentions', () => ({
  MentionPill: ({ mention }) => createElement('span', null, `${mention.sigil}${mention.name}`),
}))
mock.module('./CodeBlock', () => ({
  CodeBlock: ({ children }) => createElement('pre', null, children),
}))
mock.module('./MessageAttachments', () => ({ MessageAttachments: () => null }))
mock.module('./MessageContexts', () => ({ MessageContexts: () => null }))
mock.module('./ThinkingBlock', () => ({ ThinkingBlock: () => null }))
mock.module('./ToolCalls', () => ({ ToolCalls: () => null }))

test('user bubbles render Markdown, LaTeX, and mentions', async () => {
  const { UserBubble } = await import('./Bubble')
  const html = renderToStaticMarkup(
    createElement(UserBubble, {
      text: '**Bold** with \\(x^2\\) and [$paper](/tmp/SKILL.md).\n\n- one\n- two',
    }),
  )

  expect(html).toContain('<strong>Bold</strong>')
  expect(html).toContain('class="katex"')
  expect(html).toContain('<span>$paper</span>')
  expect(html).toContain('<ul>')
  expect(html).toContain('<li>one</li>')
})

test('user bubbles preserve typed line breaks', async () => {
  const { UserBubble } = await import('./Bubble')
  const html = renderToStaticMarkup(
    createElement(UserBubble, {
      text: '(int(d) for d in "1234")\n[int(d) for d in "1234"]\n{int(d) for d in "1234"}',
    }),
  )

  expect(html).toContain('class="chat-prose whitespace-pre-wrap"')
  expect(html).toContain('(int(d) for d in &quot;1234&quot;)\n[int(d) for d in &quot;1234&quot;]\n{int(d) for d in &quot;1234&quot;}')
})
