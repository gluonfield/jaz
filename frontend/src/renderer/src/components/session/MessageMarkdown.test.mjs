import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

mock.module('@/lib/clientRuntime', () => ({
  DEFAULT_API_BASE_URL: 'http://127.0.0.1:5299',
  clientRuntime: { platform: 'browser', defaultApiBaseUrl: () => 'https://jaz.example' },
}))
const storage = new Map([['jaz.backendAuth.https://jaz.example', 'test-image-key']])
globalThis.localStorage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, value),
  removeItem: (key) => storage.delete(key),
}

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

test.each([
  ['/tmp/chart.png', '/tmp/chart.png'],
  ['<C:/My Charts/chart.png>', 'C:/My Charts/chart.png'],
  ['C:%5CCharts%5Cchart.png', 'C:\\Charts\\chart.png'],
  ['</tmp/a #1 & 100%.png>', '/tmp/a #1 & 100%.png'],
  ['file:///tmp/a%20b.png', 'file:///tmp/a%20b.png'],
  ['plots/chart.svg', 'plots/chart.svg'],
  ['../chart.webp', '../chart.webp'],
])('Markdown image %s loads from the authenticated backend', async (source, path) => {
  const { FileReaderLinkProvider, RenderedMarkdown, UserMessageMarkdown, MessageMarkdown } = await import('./MessageMarkdown')
  for (const component of [RenderedMarkdown, UserMessageMarkdown, MessageMarkdown]) {
    const html = renderToStaticMarkup(
      createElement(QueryClientProvider, { client: new QueryClient() },
        createElement(FileReaderLinkProvider, { sessionId: 'session-1', onOpen: () => {} },
          createElement(component, { text: '![Chart](' + source + ')' }),
        ),
      ),
    )
    const url = new URL(html.match(/<img[^>]+src="([^"]+)"/)[1].replaceAll('&amp;', '&'))
    expect(url.origin).toBe('https://jaz.example')
    expect(url.pathname).toBe('/v1/sessions/session-1/file')
    expect(url.searchParams.get('path')).toBe(path)
    expect(url.searchParams.get('raw')).toBe('1')
    expect(url.searchParams.get('key')).toBe('test-image-key')
    expect(html).toContain('Expand image: Chart')
    expect(html).toContain('referrerPolicy="no-referrer"')
  }
})

test('Markdown file images resolve from the document directory', async () => {
  const { FileReaderLinkProvider, RenderedMarkdown } = await import('./MessageMarkdown')
  for (const documentPath of ['/work/docs/readme.md', 'C:\\work\\docs\\readme.md']) {
    const html = renderToStaticMarkup(
      createElement(FileReaderLinkProvider, { sessionId: 'session-2', documentPath, onOpen: () => {} },
        createElement(RenderedMarkdown, { text: '![Chart][plot]\n\n[plot]: ../assets/chart.svg' }),
      ),
    )
    const url = new URL(html.match(/<img[^>]+src="([^"]+)"/)[1].replaceAll('&amp;', '&'))
    expect(url.searchParams.get('path')).toBe(documentPath.replace('readme.md', '../assets/chart.svg'))
  }
})

test('remote and embedded images stay independent of backend credentials', async () => {
  const { markdownImageSource } = await import('@/lib/markdownImages')
  for (const source of ['https://images.example/chart.png?version=2', 'data:image/png;base64,aGVsbG8=']) {
    expect(markdownImageSource(source, 'session-1')).toBe(source)
  }
  for (const source of ['', '#', 'javascript:alert(1)', 'data:text/html;base64,aGVsbG8=', 'ftp://example.com/chart.png', '%6Aavascript:alert(1)']) {
    expect(markdownImageSource(source, 'session-1')).toBe('')
  }
  expect(markdownImageSource('/tmp/chart.png')).toBe('')
})

test('protocol-relative images remain web URLs in the packaged desktop app', async () => {
  const { RenderedMarkdown } = await import('./MessageMarkdown')
  const html = renderToStaticMarkup(createElement(RenderedMarkdown, {
    text: '![Chart](//images.example/chart.png)',
  }))
  const source = html.match(/<img[^>]+src="([^"]+)"/)[1]
  const desktopURL = new URL(source, 'file:///Applications/Jaz.app/Contents/renderer/index.html')
  expect(desktopURL.href).toBe('https://images.example/chart.png')
})

test('the Markdown pipeline rejects unsupported image schemes and unsafe links', async () => {
  const { FileReaderLinkProvider, RenderedMarkdown } = await import('./MessageMarkdown')
  for (const source of ['javascript:alert%281%29', 'data:text/html;base64,aGVsbG8=', 'ftp://example.com/chart.png']) {
    const html = renderToStaticMarkup(
      createElement(FileReaderLinkProvider, { sessionId: 'session-1', onOpen: () => {} },
        createElement(RenderedMarkdown, { text: '![Blocked](' + source + ') [unsafe](javascript:alert%281%29)' }),
      ),
    )
    expect(html).toContain('Blocked — image unavailable')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('href=')
  }
})

test('linked images keep their link without nesting interactive controls', async () => {
  const { RenderedMarkdown } = await import('./MessageMarkdown')
  const html = renderToStaticMarkup(createElement(RenderedMarkdown, {
    text: '[![Chart](https://images.example/chart.png)](https://example.com/report)',
  }))
  expect(html).toContain('href="https://example.com/report"')
  expect(html).toContain('target="_blank"')
  expect(html).toContain('src="https://images.example/chart.png"')
  expect(html).not.toContain('<button')
})
