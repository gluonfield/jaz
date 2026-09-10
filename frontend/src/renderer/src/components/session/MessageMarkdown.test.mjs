import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

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

test('inline links use website favicons and keep local file icons', async () => {
  const { RenderedMarkdown, UserMessageMarkdown } = await import('./MessageMarkdown')
  for (const component of [RenderedMarkdown, UserMessageMarkdown]) {
    const html = renderToStaticMarkup(createElement(component, {
      text: 'Opened [jaz.chat](https://jaz.chat/docs?section=links) and [app.tsx](/tmp/app.tsx:12).',
    }))

    expect(html).toContain('href="https://jaz.chat/docs?section=links"')
    expect(html).toContain('src="https://www.google.com/s2/favicons?domain=jaz.chat&amp;sz=64"')
    expect(html).toContain('alt=""')
    expect(html).toContain('lucide-file-text')
    expect(html.match(/<img\b/g)).toHaveLength(1)
  }
})

test.each(['user', 'assistant'])('saved %s messages show their timestamp beside copy and omit unknown dates', async (role) => {
  const { Bubble } = await import('./Bubble')
  const created = '2026-09-10T08:26:13Z'
  for (const created_at of [created, undefined, 'invalid', '0001-01-01T00:00:00Z']) {
    const html = renderToStaticMarkup(createElement(QueryClientProvider, { client: new QueryClient() },
      createElement(Bubble, {
        message: { seq: 1, role, content: 'Saved reply', blocks: [], created_at },
      }),
    ))

    expect(html).toContain('Copy message as Markdown')
    if (created_at === created) {
      expect(html).toContain(`dateTime="${created}"`)
      expect(html.indexOf('Copy message as Markdown')).toBeLessThan(html.indexOf('<time'))
    } else {
      expect(html).not.toContain('<time')
    }
  }
})
