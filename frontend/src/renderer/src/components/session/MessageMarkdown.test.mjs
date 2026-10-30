import { expect, mock, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

mock.module('@/lib/clientRuntime', () => ({
  DEFAULT_API_BASE_URL: 'http://127.0.0.1:5299',
  clientRuntime: { platform: 'browser', defaultApiBaseUrl: () => 'https://jaz.example' },
  openExternal: () => {},
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

test('user bubbles keep typed line breaks without preserving block whitespace', async () => {
  const { UserBubble } = await import('./Bubble')
  const html = renderToStaticMarkup(
    createElement(UserBubble, {
      text: '(int(d) for d in "1234")\n[int(d) for d in "1234"]\n{int(d) for d in "1234"}',
    }),
  )

  expect(html).toContain('(int(d) for d in &quot;1234&quot;)<br/>\n[int(d) for d in &quot;1234&quot;]<br/>\n{int(d) for d in &quot;1234&quot;}')
  // Markdown separates blocks with newline text nodes; preserved whitespace paints them as blank lines.
  expect(html).not.toContain('pre-wrap')
})

test('inline links use website favicons and keep local file icons', async () => {
  const { FileReaderLinkProvider, RenderedMarkdown, UserMessageMarkdown } = await import('./MessageMarkdown')
  for (const component of [RenderedMarkdown, UserMessageMarkdown]) {
    const html = renderToStaticMarkup(createElement(FileReaderLinkProvider, { sessionId: 'session-1', onOpen: () => {} },
      createElement(component, {
        text: 'Opened [jaz.chat](https://jaz.chat/docs?section=links) and [app.tsx](/tmp/app.tsx:12).',
      }),
    ))

    expect(html).toContain('href="https://jaz.chat/docs?section=links"')
    expect(html).toContain('src="https://jaz.chat/favicon.ico"')
    expect(html).toContain('alt=""')
    expect(html).toContain('lucide-file-text')
    expect(html.match(/<img\b/g)).toHaveLength(1)
  }
})

test('bot mentions keep their bot: target through link sanitizing', async () => {
  const { MessageMarkdown, UserMessageMarkdown } = await import('./MessageMarkdown')
  for (const component of [MessageMarkdown, UserMessageMarkdown]) {
    const html = renderToStaticMarkup(createElement(QueryClientProvider, { client: new QueryClient() },
      createElement(component, { text: 'Ask [@Inbox bot](bot:20260930T101500-abcdef12) first.' }),
    ))

    expect(html).toContain('<span>@Inbox bot</span>')
  }
})

test('connect links on their own line render live connect cards in assistant and bot chats only', async () => {
  const { MessageMarkdown, RenderedMarkdown, UserMessageMarkdown } = await import('./MessageMarkdown')
  const plugin = (id, name, extra = {}) => ({
    id,
    name,
    description: `${name} tools`,
    provider: { id, name },
    icon: { kind: 'initials', value: name.slice(0, 2) },
    auth: [{ kind: 'oauth' }],
    capabilities: [],
    multi_account: false,
    implementation: { status: 'available', owner: 'jaz' },
    connection: { status: 'not_connected' },
    ...extra,
  })
  const client = new QueryClient()
  client.setQueryData(['connections', 'plugins'], [
    plugin('slack', 'Slack'),
    plugin('mail', 'Mail', { multi_account: true, connection: { status: 'connected', accounts: [{ id: 'mail:work', account_id: 'august@work.example' }] } }),
    plugin('tasks', 'Tasks', { connection: { status: 'connected', accounts: [{ id: 'tasks', account_name: 'Team tasks' }] } }),
  ])
  for (const component of [MessageMarkdown, UserMessageMarkdown]) {
    const html = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(component, {
      text: 'Slack is not connected.\n\njaz://connect/slack\n\n[Connect Mail](jaz://connect/mail)\njaz://connect/tasks\n\nCopy `jaz://connect/slack` or open jaz://connect/slack later.\n\n`jaz://connect/mail`\n\njaz://connect/unknown',
    })))

    expect(html).toContain('Slack tools')
    expect(html).toContain('>Connect</button>')
    expect(html).toContain('august@work.example')
    expect(html).toContain('>Add account</button>')
    expect(html).toContain('Team tasks')
    expect(html).toContain('Connected</span>')
    expect(html).toContain('<code>jaz://connect/slack</code>')
    expect(html).toContain('open jaz://connect/slack later.')
    expect(html).toContain('<p><code>jaz://connect/mail</code></p>')
    expect(html).toContain('<p>jaz://connect/unknown</p>')
    expect(html.match(/jaz:\/\/connect\//g)).toHaveLength(4)
  }

  const loading = renderToStaticMarkup(createElement(QueryClientProvider, { client: new QueryClient() },
    createElement(MessageMarkdown, { text: 'jaz://connect/slack' }),
  ))
  expect(loading).toContain('animate-pulse')
  expect(loading).not.toContain('jaz://connect/slack')

  expect(renderToStaticMarkup(createElement(RenderedMarkdown, { text: 'jaz://connect/slack' }))).toContain('<p>jaz://connect/slack</p>')
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
  const { setResourceTicket } = await import('@/lib/api/client')
  setResourceTicket('https://jaz.example', { token: 'test-image-ticket', expires_at: '2030-01-01T00:00:00Z' })
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
    expect(url.searchParams.get('key')).toBeNull()
    expect(url.searchParams.get('ticket')).toBe('test-image-ticket')
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
    expect(markdownImageSource(source, 'image-1', 'session-1')).toBe(source)
  }
  for (const source of ['', '#', 'javascript:alert(1)', 'data:text/html;base64,aGVsbG8=', 'ftp://example.com/chart.png', '%6Aavascript:alert(1)']) {
    expect(markdownImageSource(source, 'image-1', 'session-1')).toBe('')
  }
  expect(markdownImageSource('/tmp/chart.png', 'image-1')).toBe('')
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

test('file references stay plain text where no file reader can open them', async () => {
  const { RenderedMarkdown } = await import('./MessageMarkdown')
  const html = renderToStaticMarkup(createElement(RenderedMarkdown, { text: 'Read [the notes](/NOTES.md).' }))
  expect(html).toContain('Read the notes.')
  expect(html).not.toContain('role="link"')
  expect(html).not.toContain('lucide-file-text')
})

test('standalone Markdown paths and links render document cards with formatted content in chat', async () => {
  const { FileReaderLinkProvider, UserMessageMarkdown, MessageMarkdown } = await import('./MessageMarkdown')
  const { sessionFileQuery } = await import('@/lib/api/sessions')
  const client = new QueryClient()
  const path = '/work/docs/My report.md'
  client.setQueryData(sessionFileQuery('documents', path).queryKey, {
    path, size: 100,
    content: '# Manufacturing report\n\nA **short** answer.\n\n## Routes\n\n- Press\n- Laser\n\n| Route | Cost |\n| --- | --- |\n| Press | £1 |\n\n![Drawing](drawing.svg)',
  })
  for (const component of [MessageMarkdown, UserMessageMarkdown]) {
    for (const text of [path, '[Report](</work/docs/My report.md>)', '[Report](<docs/My report.md>)', 'docs/My report.md', 'docs/My%20report.md']) {
      const html = renderToStaticMarkup(createElement(QueryClientProvider, { client },
        createElement(FileReaderLinkProvider, { sessionId: 'documents', documentPath: '/work/', onOpen: () => {} },
          createElement(component, { text }),
        ),
      ))
      expect(html).toContain('class="markdown-document-card"')
      expect(html).toContain('<h1 id="manufacturing-report">Manufacturing report</h1>')
      expect(html).toContain('<strong>short</strong>')
      expect(html).toContain('<li>Press</li>')
      expect(html).toContain('<td>£1</td>')
      expect(html).toContain('aria-label="Open Manufacturing report"')
      expect(html.match(/Manufacturing report/g)).toHaveLength(2)
      expect(html).not.toContain('Markdown ·')
      expect(html).not.toContain('lucide-arrow-up-right')
      const source = new URL(html.match(/<img[^>]+src="([^"]+)"/)[1].replaceAll('&amp;', '&'))
      expect(source.pathname).toBe('/v1/sessions/documents/file')
      expect(source.searchParams.get('path')).toBe('/work/docs/drawing.svg')
    }
  }
  client.clear()
})

test('bare filenames show cards and unavailable previews keep an open action', async () => {
  const { FileReaderLinkProvider, UserMessageMarkdown } = await import('./MessageMarkdown')
  const { sessionFileQuery } = await import('@/lib/api/sessions')
  for (const state of ['loading', 'binary', 'empty', 'error']) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false } } })
    const options = sessionFileQuery('documents', 'report.md')
    if (state === 'binary' || state === 'empty') client.setQueryData(options.queryKey, { path: '/work/report.md', binary: state === 'binary', content: '', size: 0 })
    if (state === 'error') client.getQueryCache().build(client, options).setState({ status: 'error', error: new Error('File missing') })
    const html = renderToStaticMarkup(createElement(QueryClientProvider, { client },
      createElement(FileReaderLinkProvider, { sessionId: 'documents', onOpen: () => {} },
        createElement(UserMessageMarkdown, { text: 'report.md' }),
      ),
    ))
    expect(html).toContain('aria-label="Open report.md"')
    expect(html).toContain(state === 'loading' ? 'Loading document' : state === 'empty' ? 'Empty document' : 'Preview unavailable')
    client.clear()
  }
  const noContext = renderToStaticMarkup(createElement(UserMessageMarkdown, { text: 'report.md' }))
  expect(noContext).toContain('<p>report.md</p>')
  expect(noContext).not.toContain('markdown-document-card')
})

test('Markdown cards leave prose, code, mentions and document-body links unchanged', async () => {
  const { FileReaderLinkProvider, UserMessageMarkdown, RenderedMarkdown } = await import('./MessageMarkdown')
  const client = new QueryClient()
  for (const text of [
    'Read [the report](/work/report.md).',
    'Read report.md',
    '`/work/report.md`',
    '```text\n/work/report.md\n```',
    '[$notes](/work/SKILL.md)',
    '[Website](https://example.com/report.md)',
    '/work/report.md:12',
    '/work/app.tsx',
  ]) {
    const html = renderToStaticMarkup(createElement(QueryClientProvider, { client },
      createElement(FileReaderLinkProvider, { sessionId: 'documents', onOpen: () => {} },
        createElement(UserMessageMarkdown, { text }),
      ),
    ))
    expect(html).not.toContain('markdown-document-card')
  }
  const body = renderToStaticMarkup(createElement(FileReaderLinkProvider, { sessionId: 'documents', documentPath: '/work/docs/report.md', onOpen: () => {} },
    createElement(RenderedMarkdown, { text: '[Appendix](appendix.md)' }),
  ))
  expect(body).toContain('role="link"')
  expect(body).not.toContain('markdown-document-card')
  client.clear()
})
