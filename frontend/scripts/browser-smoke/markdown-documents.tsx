import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import { FileReaderLinkProvider, MessageMarkdown } from '@/components/session/MessageMarkdown'
import { keys } from '@/lib/query/keys'
import type { FileReference } from '@shared/fileReader'

export async function exerciseMarkdownDocuments() {
  const path = '/workspace/reports/route-comparison.md'
  const content = '# Route comparison\n\n## Contents\n\n- [Résumé and costs](#résumé--costs)\n- [Next steps](#next-steps)\n- [More steps](#next-steps-1)\n\n' +
    'A short paragraph explaining the manufacturing options.\n\n'.repeat(12) +
    '## Résumé & **costs**\n\nUse the available presses.\n\n## Next steps\n\nConfirm the route.\n\n## Next steps\n\nReserve the presses.\n\n## Next steps-1\n\nKeep the deadline.'
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  client.setQueryData(keys.sessionFile('documents', path), { path, content, size: content.length })
  client.setQueryData(keys.health, { capabilities: { session_file_read: true } })
  client.setQueryData(keys.skills(), [])
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;z-index:10;background:var(--color-bg);overflow:hidden'
  document.body.append(host)
  const root = createRoot(host)
  let opens = 0
  function View() {
    const [file, setFile] = useState<FileReference | null>(null)
    return <QueryClientProvider client={client}>
      <div className="flex h-full flex-col gap-4 p-4">
        <div className="min-h-0 flex-1 overflow-auto" data-document-chat>
          <FileReaderLinkProvider sessionId="documents" onOpen={(next) => {
            opens += 1
            setFile(next)
          }}><MessageMarkdown text={path} /></FileReaderLinkProvider>
        </div>
        {file ? <div className="h-[55%] min-h-0 border border-border" data-document-reader>
          <FileReaderPanel sessionId="documents" fileRef={file} visible onOpenFile={setFile} onClose={() => setFile(null)} />
        </div> : null}
      </div>
    </QueryClientProvider>
  }
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 5000
    while (!check()) {
      if (Date.now() > deadline) throw new Error('Markdown document check: ' + check.toString())
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const click = async (target: Element | null) => {
    if (!target) throw new Error('Missing document control')
    const rect = target.getBoundingClientRect()
    const x = Math.round(rect.x + rect.width / 2)
    const y = Math.round(rect.y + rect.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  try {
    root.render(<View />)
    for (const width of [1200, 390]) {
      await window.smoke.resize(width, 844)
      await until(() => Boolean(host.querySelector('.markdown-document-card button')))
      await click(host.querySelector('.markdown-document-card button'))
      await until(() => Boolean(host.querySelector('[data-document-reader] h1')))
      const reader = host.querySelector<HTMLElement>('[data-document-reader]')!
      const scroll = reader.querySelector<HTMLElement>('.scrollbar-quiet')!
      const url = location.href
      const queries = client.getQueryCache().getAll().length
      const opened = opens
      if (host.textContent?.includes('/workspace/') || [...host.querySelectorAll('[title]')].some((node) => node.getAttribute('title')?.includes('/workspace/'))) throw new Error('Markdown chrome exposes its filesystem path')
      if (reader.querySelectorAll('#next-steps-1').length !== 1 || !reader.querySelector('#next-steps-1-1')) throw new Error('Duplicate heading anchors collide')
      for (const href of ['#résumé--costs', '#next-steps', '#next-steps-1']) {
        scroll.scrollTop = 0
        await new Promise(requestAnimationFrame)
        await click([...reader.querySelectorAll('a[href]')].find((node) => decodeURIComponent(node.getAttribute('href') ?? '') === href) ?? null)
        await until(() => scroll.scrollTop > 0)
        if (location.href !== url || client.getQueryCache().getAll().length !== queries || opens !== opened) throw new Error('Contents navigation changed the document or thread')
      }
      if (reader.scrollWidth > reader.clientWidth || document.documentElement.scrollWidth > innerWidth) throw new Error('Document reader overflows its viewport')
      scroll.scrollTop = 0
      await window.smoke.capture(width === 1200 ? 'markdown-document-desktop' : 'markdown-document-phone')
      await click(reader.querySelector('[aria-label="Close document"]'))
      await until(() => !host.querySelector('[data-document-reader]'))
      if (!host.querySelector('[data-document-chat] .markdown-document-card')) throw new Error('Closing the document removed the chat')
    }
  } finally {
    root.unmount()
    host.remove()
    client.clear()
  }
}
