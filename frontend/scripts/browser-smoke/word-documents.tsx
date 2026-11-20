import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import { FileReaderLinkProvider, MessageMarkdown } from '@/components/session/MessageMarkdown'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import type { FileReference } from '@shared/fileReader'
import { LOCAL_BACKEND_URL } from '@shared/localBackend'

export async function exerciseWordDocuments() {
  const path = 'word-reader.docx'
  const opening = 'first opening clause applies'
  const table = 'EMERGENCY power   when required by code'
  const missing = 'absent quotation starts at the top'
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:9999'
  document.body.append(host)
  const root = createRoot(host)
  const client = new QueryClient()
  const previousBackend = apiBaseUrl()
  setApiBaseUrl(LOCAL_BACKEND_URL)
  function View() {
    const [file, setFile] = useState<FileReference | null>(null)
    return <>
      <FileReaderLinkProvider sessionId="tabs" onOpen={setFile}>
        <MessageMarkdown text={[opening, table, missing].map((quote) => `[${quote}](${path}#text=${encodeURIComponent(quote)})`).join('\n\n')} />
      </FileReaderLinkProvider>
      {file && <div className="absolute inset-y-0 right-0 w-[60%] bg-bg">
        <FileReaderPanel sessionId="tabs" fileRef={file} visible onOpenFile={setFile} onClose={() => setFile(null)} />
      </div>}
    </>
  }
  const until = async (description: string, check: () => boolean) => {
    const deadline = performance.now() + 5000
    while (!check()) {
      if (performance.now() > deadline) throw new Error(description)
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const frame = () => host.querySelector<HTMLIFrameElement>('iframe[title="Word document"]')
  const highlight = () => frame()?.contentWindow?.CSS.highlights.get('file-location')?.values().next().value as Range | undefined
  const click = (quote: string) => {
    host.querySelectorAll<HTMLElement>('[role="link"]').forEach((link) => {
      if (link.textContent === quote) link.click()
    })
  }
  try {
    root.render(<StrictMode><QueryClientProvider client={client}><View /></QueryClientProvider></StrictMode>)
    await until('The Word links did not render', () => host.querySelectorAll('[role="link"]').length === 3)
    click(table)
    await until('The table quote did not highlight and scroll into view', () => {
      const range = highlight()
      const rect = range?.getBoundingClientRect()
      return !!range?.startContainer.parentElement?.closest('td')
        && range.toString().replace(/\s+/g, ' ') === 'emergency powerwhen required by code'
        && !!rect && rect.top >= 0 && rect.bottom <= frame()!.clientHeight
        && (frame()!.contentWindow?.scrollY ?? 0) > 0
    })
    const document = frame()!.contentDocument!
    const page = document.querySelector('section.docx')
    const link = document.querySelector('a')
    if (!link || link.hasAttribute('href') || document.querySelector('iframe, script') || document.body.isContentEditable) {
      throw new Error('The Word document kept active content or links')
    }
    link.click()
    click(opening)
    await until('The second quote did not replace the first', () => highlight()?.toString() === opening)
    if (frame()?.contentDocument !== document || document.querySelector('section.docx') !== page) throw new Error('The reader reloaded the same document')
    click(table)
    await until('The reader did not return to the table', () => (frame()?.contentWindow?.scrollY ?? 0) > 0)
    click(missing)
    await until('An absent quote did not clear the highlight and return to the top', () => !highlight() && frame()?.contentWindow?.scrollY === 0)
    const requests = await fetch('/resource-identities').then((response) => response.json())
    if (requests['/word-remote-probe.svg']) throw new Error('A Word document requested a remote resource')
    host.querySelector<HTMLButtonElement>('[aria-label="File actions"]')!.click()
    const download = () => [...window.document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find((item) => item.textContent === 'Download file')
    await until('The Word reader lost its download action', () => Boolean(download()))
    download()!.click()
    if (await window.smoke.fileDownload() !== path) throw new Error('The Word download lost its filename or bytes')
  } finally {
    root.unmount()
    host.remove()
    client.clear()
    setApiBaseUrl(previousBackend)
  }
}
