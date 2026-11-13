import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterContextProvider } from '@tanstack/react-router'
import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import { FileReaderLinkProvider, MessageMarkdown } from '@/components/session/MessageMarkdown'
import { SidePanelCloseGuard } from '@/components/session/SidePanelState'
import { keys } from '@/lib/query/keys'
import type { SessionFileRead } from '@/lib/api/types'
import type { FileReference } from '@shared/fileReader'

export async function exerciseMarkdownDocuments() {
  const size = { width: innerWidth, height: innerHeight }
  const path = '/workspace/reports/route-comparison.md'
  const filler = 'A short paragraph explaining the manufacturing options.\n\n'.repeat(12)
  const content = '# Route comparison\n\n## Contents\n\n- [Résumé and costs](#résumé--costs)\n- [Next steps](#next-steps)\n- [More steps](#next-steps-1)\n\n' + filler +
    '## Résumé & **costs**\n\nUse the available presses.\n\nA part costs \\(c = m + t\\) to make.\n\n| Route | Cost |\n| --- | ---: |\n| Press | £1 |\n\n- [ ] Confirm the press\n- [x] Price the blank\n\n```sh\nmake check\n```\n\n' + filler +
    '## Next steps\n\nConfirm the route.\n\n' + filler + '## Next steps\n\nReserve the presses.\n\n' + filler + '## Next steps-1\n\nKeep the deadline.\n\n' + filler
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  // The file as the backend holds it: a save is refused unless it is made over
  // this version.
  let stored: SessionFileRead = { path, content, size: content.length, version: 'v1' }
  const saves: { over: string | null; content: string }[] = []
  const created: string[] = []
  let refuseWrites = false
  let closeGuard: (() => Promise<boolean>) | undefined
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = new URL(String(input), location.href)
    if (url.pathname !== '/v1/sessions/documents/file') return originalFetch(input, init)
    if (init?.method !== 'PUT') return Response.json(stored)
    if (new Headers(init.headers).get('If-None-Match')) {
      created.push(url.searchParams.get('path')!)
      return new Response(null, { status: 201 })
    }
    const over = new Headers(init.headers).get('If-Match')
    saves.push({ over, content: String(init.body) })
    if (refuseWrites) return new Response(null, { status: 500 })
    if (over !== stored.version) return new Response(null, { status: 412, headers: { ETag: stored.version! } })
    stored = { ...stored, content: String(init.body), version: stored.version + '+' }
    return new Response(null, { status: 204, headers: { ETag: stored.version! } })
  }
  client.setQueryData(keys.sessionFile('documents', path), stored)
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
          <SidePanelCloseGuard value={(guard) => {
            closeGuard = guard
            return () => { closeGuard = undefined }
          }}>
            <FileReaderPanel sessionId="documents" fileRef={file} visible onOpenFile={setFile} onClose={() => setFile(null)} />
          </SidePanelCloseGuard>
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
  const type = async (letters: string) => {
    for (const letter of letters) await window.smoke.key(letter)
  }
  // The reader is the editor: typing saves only the edited line over the
  // version read, the agent's writes show, a refused save asks, and a block
  // kept as Markdown is edited as its source.
  const edit = async (reader: HTMLElement) => {
    const text = () => reader.textContent ?? ''
    const paragraph = (start: string) => [...reader.querySelectorAll('p')].find((node) => node.textContent?.startsWith(start)) ?? null
    const source = (selector: string) => reader.querySelector(selector)!.closest<HTMLElement>('.document-source')!
    const editSource = async (block: HTMLElement) => {
      block.scrollIntoView({ block: 'center' })
      await new Promise(requestAnimationFrame)
      await click(block)
      await click(block.querySelector('[aria-label="Edit as Markdown"]'))
      await until(() => document.activeElement?.tagName === 'TEXTAREA')
    }
    if (!reader.querySelector('.document-editor table')?.textContent?.includes('£1') || !source('pre').textContent?.includes('make check') || !source('.katex') || reader.querySelectorAll('ul[data-type="taskList"] input[type="checkbox"]').length !== 2) throw new Error('The editable document lost what the reader draws')
    const typeAfterPresses = async (letters: string) => {
      paragraph('Use the available')!.scrollIntoView({ block: 'center' })
      await new Promise(requestAnimationFrame)
      await click(paragraph('Use the available'))
      await window.smoke.key('End')
      await type(letters)
    }
    const refuseDiscard = async () => {
      const confirm = window.confirm
      let asked = 0
      window.confirm = () => {
        asked += 1
        return false
      }
      try {
        if (!closeGuard || await closeGuard()) throw new Error('Closing the tab would discard unsaved edits')
        await click(reader.querySelector('[aria-label="Close document"]'))
        await until(() => asked === 2)
        if (!reader.isConnected) throw new Error('Closing the document discarded unsaved edits')
      } finally {
        window.confirm = confirm
      }
    }
    await typeAfterPresses('Owl')
    await until(() => saves.length === 1 && reader.querySelector('[role="status"]')?.textContent === '')
    if (saves[0].over !== 'v1' || saves[0].content !== content.replace('presses.', 'presses.Owl')) throw new Error('The edit saved as ' + JSON.stringify(saves[0]))
    if (client.getQueryData<SessionFileRead>(keys.sessionFile('documents', path))?.version !== 'v1+') throw new Error('The saved version was not kept')

    stored = { ...stored, content: stored.content + '\nThe agent added this.\n', version: 'agent' }
    client.setQueryData(keys.sessionFile('documents', path), stored)
    await until(() => text().includes('The agent added this.') && text().includes('presses.Owl'))

    stored = { ...stored, content: stored.content + '\nThe agent wrote again.\n', version: 'agent-2' }
    await typeAfterPresses('Bee')
    await until(() => text().includes('changed while you were editing'))
    await window.smoke.capture('markdown-document-conflict')
    await refuseDiscard()
    if (!text().includes('presses.OwlBee')) throw new Error('Refusing to discard lost the conflicting edit')
    await click([...reader.querySelectorAll('button')].find((node) => node.textContent === 'Keep mine') ?? null)
    await until(() => stored.version === 'agent-2+' && !text().includes('changed while you were editing'))
    if (!stored.content.includes('presses.OwlBee')) throw new Error('Keep mine lost the edit: ' + stored.content)

    const before = saves.length
    await editSource(source('pre'))
    if ((document.activeElement as HTMLTextAreaElement).value !== '```sh\nmake check\n```') throw new Error('The code block did not open as its Markdown')
    await type('x')
    await window.smoke.key('Escape')
    await until(() => document.activeElement?.tagName !== 'TEXTAREA' && source('pre').textContent?.includes('make check') === true)
    await editSource(source('.katex'))
    await window.smoke.key('Left')
    await window.smoke.key('Left')
    await type('2')
    await window.smoke.key('Enter')
    await until(() => saves.length === before + 1 && Boolean(reader.querySelector('.katex')))
    if (!stored.content.includes('costs \\(c = m + t2\\) to make.') || !stored.content.includes('| Press | £1 |')) throw new Error('Editing a formula saved as ' + stored.content)
    await click(reader.querySelector('ul[data-type="taskList"] input[type="checkbox"]'))
    await until(() => stored.content.includes('- [x] Confirm the press'))
    const ticked = getComputedStyle(reader.querySelector('ul[data-type="taskList"] input[type="checkbox"]')!)
    if (ticked.backgroundColor !== ticked.borderTopColor || ticked.backgroundColor === 'rgba(0, 0, 0, 0)') throw new Error('A box ticked under the pointer does not show as ticked')
    const row = (label: string) => [...document.querySelectorAll('[role="menuitem"]')].find((node) => node.textContent === label) ?? null
    await click([...reader.querySelectorAll('td')].find((node) => node.textContent === '£1') ?? null)
    await window.smoke.key('End')
    await type('0')
    await until(() => stored.content.includes('| Press | £10  |'))
    await click(reader.querySelector('[aria-label="Edit table"]'))
    await until(() => Boolean(row('Insert row below')))
    await click(row('Insert row below'))
    await until(() => stored.content.includes('| Press | £10  |\n|       |      |'))
    reader.querySelector('table')!.scrollIntoView({ block: 'center' })
    await new Promise(requestAnimationFrame)
    await window.smoke.capture('markdown-document-table')
    await click(paragraph('A part costs'))
    await window.smoke.key('Home')
    await window.smoke.key('Right', ['shift', 'alt'])
    await until(() => Boolean(reader.querySelector('[aria-label="Bold"]')))
    await click(reader.querySelector('[aria-label="Bold"]'))
    await until(() => stored.content.includes('**A** part costs'))
    await window.smoke.key('End')
    const picked = new DataTransfer()
    picked.items.add(new File(['png'], 'first sketch.png', { type: 'image/png' }))
    const picker = reader.querySelector<HTMLInputElement>('input[type="file"]')!
    picker.files = picked.files
    picker.dispatchEvent(new Event('change', { bubbles: true }))
    await until(() => created.length === 1 && stored.content.includes(`![first sketch](${created[0].slice(created[0].indexOf('images/'))})`))
    if (!/\/images\/\w{8}-first-sketch\.png$/.test(created[0])) throw new Error('The picture was saved as ' + created[0])
    await click(paragraph('A part costs'))
    await window.smoke.key('End')
    await window.smoke.key('Enter')
    const pasted = new DataTransfer()
    pasted.setData('text/plain', '### Pasted\n\n- one\n- two')
    reader.querySelector('.ProseMirror')!.dispatchEvent(new ClipboardEvent('paste', { clipboardData: pasted, bubbles: true, cancelable: true }))
    await until(() => stored.content.includes('.png)\n\n### Pasted\n\n- one\n- two\n'))
    await click(paragraph('Use the available'))
    await window.smoke.key('Home')
    await window.smoke.key('Right', ['shift', 'alt'])
    await until(() => Boolean(reader.querySelector('[aria-label="Link"]')))
    await click(reader.querySelector('[aria-label="Link"]'))
    await until(() => document.activeElement?.getAttribute('name') === 'href')
    await type('notes.md')
    await window.smoke.key('Enter')
    await until(() => stored.content.includes('[Use](notes.md) the available'))
    await click(reader.querySelector('[aria-label="Outline"]'))
    await until(() => Boolean(row('Next steps-1')))
    await click(row('Next steps-1'))
    const last = [...reader.querySelectorAll('h2')].at(-1)!
    await until(() => !row('Next steps-1') && Math.abs(last.getBoundingClientRect().top - reader.querySelector('.scrollbar-quiet')!.getBoundingClientRect().top) < 48)
    await typeAfterPresses('')
    await window.smoke.key('Enter')
    await type('``` ')
    await until(() => (document.activeElement as HTMLTextAreaElement).value === '```\n\n```')
    await type('ls')
    await click(paragraph('Use the available'))
    await until(() => stored.content.includes('presses.OwlBee\n\n```\nls\n```\n\n**A** part costs'))
    if ([...reader.querySelectorAll('.document-source pre')].every((node) => node.textContent !== 'ls')) throw new Error('A typed fence did not become a code block')
    await click(paragraph('Use the available'))
    await window.smoke.key('End')
    await window.smoke.key('Enter')
    await type('/')
    await until(() => Boolean(row('Quote')))
    await new Promise((resolve) => setTimeout(resolve, 250))
    await window.smoke.capture('markdown-document-slash')
    await type('che')
    await until(() => Boolean(row('Checklist')) && !row('Quote'))
    await window.smoke.key('Enter')
    await type('Call')
    await until(() => stored.content.includes('presses.OwlBee\n\n- [ ] Call\n\n```\nls'))
    await window.smoke.capture('markdown-document-edited')
    refuseWrites = true
    await typeAfterPresses('Fox')
    await until(() => reader.querySelector('[role="status"]')?.textContent === "Couldn't save")
    await refuseDiscard()
    refuseWrites = false
    await click([...reader.querySelectorAll('button')].find((node) => node.textContent === 'Retry') ?? null)
    await until(() => reader.querySelector('[role="status"]')?.textContent === '' && stored.content.includes('Fox'))
  }
  const router = createRouter({ routeTree: createRootRoute(), history: createMemoryHistory() })
  try {
    root.render(<RouterContextProvider router={router}><View /></RouterContextProvider>)
    for (const width of [1200, 390]) {
      await window.smoke.resize(width, 844)
      await until(() => Boolean(host.querySelector('.markdown-document-card > button')))
      await click(host.querySelector('.markdown-document-card > button'))
      await until(() => Boolean(host.querySelector('[data-document-reader] h1')))
      const reader = host.querySelector<HTMLElement>('[data-document-reader]')!
      const scroll = reader.querySelector<HTMLElement>('.scrollbar-quiet')!
      const url = location.href
      const queries = client.getQueryCache().getAll().length
      const opened = opens
      const saved = saves.length
      if (host.textContent?.includes('/workspace/') || [...host.querySelectorAll('[title]')].some((node) => node.getAttribute('title')?.includes('/workspace/'))) throw new Error('Markdown chrome exposes its filesystem path')
      const headings = [...reader.querySelectorAll('h2')]
      for (const [href, heading] of [['#résumé--costs', headings[1]], ['#next-steps', headings[2]], ['#next-steps-1', headings[3]]] as const) {
        scroll.scrollTop = 0
        await new Promise(requestAnimationFrame)
        await click([...reader.querySelectorAll('a[href]')].find((node) => decodeURIComponent(node.getAttribute('href') ?? '') === href) ?? null)
        await until(() => Math.abs(heading.getBoundingClientRect().top - scroll.getBoundingClientRect().top) < 48)
        if (location.href !== url || client.getQueryCache().getAll().length !== queries || opens !== opened) throw new Error('Contents navigation changed the document or thread')
      }
      if (saves.length !== saved) throw new Error('Reading a document saved it')
      if (width === 1200) await edit(reader)
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
    window.fetch = originalFetch
    await window.smoke.resize(size.width, size.height)
  }
}
