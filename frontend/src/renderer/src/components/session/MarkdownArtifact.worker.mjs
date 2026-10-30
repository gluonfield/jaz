import { mock } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

mock.module('@/lib/clientRuntime', () => ({
  DEFAULT_API_BASE_URL: 'http://127.0.0.1:5299',
  clientRuntime: { platform: 'browser', defaultApiBaseUrl: () => 'https://jaz.example' },
  openExternal: () => {},
}))
mock.module('@/lib/theme', () => ({ useTheme: () => ({ resolved: 'light' }) }))
mock.module('@/lib/appearance', () => ({ useEffectsEnabled: () => false, useInlineDiffs: () => false, useInlineShellCommands: () => false }))
mock.module('@/lib/hooks/useArtifactThemeCSS', () => ({ useArtifactThemeCSS: () => ':root{--color-ink:#222}' }))
globalThis.localStorage = { getItem: () => null }
const { FileReaderLinkProvider, RenderedMarkdown, MessageMarkdown } = await import('./MessageMarkdown')
const { FileReaderPanel } = await import('./FileReaderPanel')
const { sessionFileQuery } = await import('@/lib/api/sessions')
const client = new QueryClient()
const code = String.raw`<script>
const pattern = /\(x\)/;
const literal = '\[x\]';
</script><button>Run</button>`
const fence = (marker, source = code, title = 'Counter') => `${marker}jaz-artifact ${title}\n${source}\n${marker}\n\nMath: \\(x^2\\)`
const render = (text, component = RenderedMarkdown, props = {}, documentPath = '/work/report.md') => renderToStaticMarkup(
  createElement(QueryClientProvider, { client },
    createElement(FileReaderLinkProvider, { sessionId: 'artifact-doc', documentPath, onOpen: () => {} },
      createElement(component, { text, ...props }),
    ),
  ),
)
const full = '<!doctype html><html><head><title>Full</title></head><body>' + code + '</body></html>'
client.setQueryData(sessionFileQuery('artifact-doc', '/work/report.md').queryKey, { path: '/work/report.md', content: '# Report\n\n' + fence('~~~'), size: 100 })
const result = {
  code,
  live: [
    ...['```', '````', '~~~', '~~~~'].map((marker) => render(fence(marker))),
    render(fence('~~~').split('\n').map((line) => '> ' + line).join('\n')),
    render('- Item\n\n' + fence('~~~').split('\n').map((line) => '    ' + line).join('\n') + '\n\nMath: \\(x^2\\)'),
  ],
  full: render(fence('~~~', full)),
  svg: render(fence('```', '<svg viewBox="0 0 100 40"><text x="4" y="20">Diagram</text></svg>')),
  plain: ['html', 'svg', 'text'].map((language) => render('```' + language + '\n' + code + '\n```')),
  outside: render(fence('```'), MessageMarkdown),
  contextless: render(fence('```'), RenderedMarkdown, {}, ''),
  preview: render(fence('~~~'), RenderedMarkdown, { artifactPreview: true }),
  card: render('/work/report.md', MessageMarkdown),
  hidden: renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(FileReaderPanel, {
    sessionId: 'artifact-doc', fileRef: { path: '/work/report.md' }, visible: false, onOpenFile: () => {}, onClose: () => {},
  }))),
}
client.clear()
globalThis.postMessage(result)
