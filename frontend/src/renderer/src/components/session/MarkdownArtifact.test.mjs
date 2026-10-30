import { expect, test } from 'bun:test'

test('Markdown documents reuse sandboxed Jaz artifacts only for explicit live fences', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./MarkdownArtifact.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
    const decode = (text) => text.replaceAll('&quot;', '"').replaceAll('&#x27;', "'").replaceAll('&lt;', '<').replaceAll('&gt;', '>').replaceAll('&amp;', '&')
    for (const html of [...result.live, result.full, result.svg]) {
      expect(html).toContain('<iframe')
      expect(html).toContain('title="Counter"')
      expect(html).toContain('sandbox="allow-scripts allow-forms"')
      expect(html).toContain('referrerPolicy="no-referrer"')
      const document = decode(html.match(/srcDoc="([^"]+)"/)[1])
      expect(document).toContain('Content-Security-Policy')
      expect(document).toContain('jaz:artifact-height')
      expect(document).toContain('--color-ink:#222')
      expect(document).not.toContain('ticket=')
      if (html !== result.svg) expect(document).toContain(result.code)
    }
    for (const html of result.live) expect(html).toContain('class="katex"')
    for (const html of [...result.plain, result.outside, result.contextless, result.preview, result.card, result.hidden]) expect(html).not.toContain('<iframe')
    for (const html of [result.preview, result.card, result.hidden]) {
      expect(html).toContain('Counter · Interactive artifact')
      expect(html).not.toContain('const pattern')
    }
    for (const html of result.plain) expect(decode(html)).toContain('const pattern')
  } finally {
    worker.terminate()
  }
})
