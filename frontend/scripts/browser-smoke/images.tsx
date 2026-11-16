import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { FileReaderLinkProvider, MessageMarkdown } from '@/components/session/MessageMarkdown'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import type { FileReference } from '@shared/fileReader'
import { LOCAL_BACKEND_TRANSPORT_URL } from '@shared/localBackend'

export async function exerciseImages() {
  const remoteURL = new URL('/remote-chart.svg', location.origin)
  remoteURL.hostname = 'localhost'
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:9999'
  document.body.append(host)
  const root = createRoot(host)
  const client = new QueryClient()
  const previousBackend = apiBaseUrl()
  setApiBaseUrl(await window.smoke.backend())
  function View({ count, suffix }: { count: number; suffix: string }) {
    const [file, setFile] = useState<FileReference | null>(null)
    return <FileReaderLinkProvider sessionId="tabs" onOpen={setFile}>
      <MessageMarkdown text={`![Remote chart](${remoteURL.href})\n\n[Remote page](${remoteURL.origin}/)\n\n[PNG file](icon-1024.png)\n\n[SVG file](cache-chart.svg)`} />
      {Array.from({ length: count }, (_, index) => (
        <MessageMarkdown key={index} text={`![Chart](cache-chart.svg)${suffix}`} />
      ))}
      {file ? <div className="absolute inset-y-0 right-0 w-[55%] bg-bg" data-image-reader>
        <FileReaderPanel sessionId="tabs" fileRef={file} visible onOpenFile={setFile} onClose={() => setFile(null)} />
      </div> : null}
    </FileReaderLinkProvider>
  }
  const render = (count: number, suffix = '') => root.render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <View count={count} suffix={suffix} />
      </QueryClientProvider>
    </StrictMode>,
  )
  const until = async (check: () => boolean) => {
    const deadline = performance.now() + 5000
    while (!check()) {
      if (performance.now() > deadline) {
        throw new Error('Inline image did not load its current file contents')
      }
      await new Promise(resolve => setTimeout(resolve, 20))
    }
  }
  const images = () => [...host.querySelectorAll<HTMLImageElement>('img[alt="Chart"]')]
  try {
    render(1)
    await until(() => images()[0]?.naturalWidth === 12)
    await until(() => [...host.querySelectorAll<HTMLImageElement>('img')].every(image => image.complete && image.naturalWidth > 0))
    const requests = await fetch('/resource-identities').then(response => response.json()) as Record<string, Record<string, string>>
    for (const path of ['/remote-chart.svg', '/favicon.ico']) {
      if (!requests[path] || requests[path].referer || /jaz|electron/i.test(requests[path]['user-agent'])) {
        throw new Error('A remote image or favicon received app identity or referrer metadata: ' + JSON.stringify(requests))
      }
    }
    const originalSource = images()[0].src
    const response = await fetch('/image-fixture', { method: 'POST' })
    if (!response.ok) {
      throw new Error('Could not overwrite the image fixture')
    }
    render(2)
    await until(() => images()[1]?.naturalWidth === 24)
    const updatedSource = images()[1].src
    render(2, '\n\nStreamed text\n\n![Latest](cache-chart.svg)')
    await until(() => host.textContent?.includes('Streamed text') === true)
    await until(() => {
      const latest = [...host.querySelectorAll<HTMLImageElement>('img[alt="Latest"]')]
      return latest.length === 2 && latest.every(image => image.naturalWidth === 24)
    })
    if (images()[0].src !== originalSource || images()[1].src !== updatedSource) {
      throw new Error('Text streaming changed the image request identity')
    }
    for (const [label, width] of [['PNG file', 1024], ['SVG file', 24]] as const) {
      const link = [...host.querySelectorAll<HTMLElement>('[role="link"]')].find((node) => node.textContent === label)
      link?.click()
      await until(() => host.querySelector<HTMLImageElement>('[data-image-reader] img')?.naturalWidth === width)
      const reader = host.querySelector<HTMLElement>('[data-image-reader]')!
      const image = reader.querySelector('img')!.getBoundingClientRect()
      if (image.height > reader.clientHeight || image.width > reader.clientWidth || reader.textContent?.includes('Binary file')) throw new Error('Image file did not fit its side tab')
      const folder = reader.querySelector<HTMLButtonElement>('[aria-label="Open folder"]')!
      if (!folder.disabled || !folder.title.includes('this machine')) throw new Error('A browser without native file access offers Open folder')
      if (label === 'PNG file') {
        await window.smoke.capture('image-file-tab')
        const download = reader.querySelector<HTMLAnchorElement>('[aria-label="Download file"]')!
        const url = new URL(download.href)
        download.href = LOCAL_BACKEND_TRANSPORT_URL + url.pathname + url.search
        download.click()
        if (await window.smoke.fileDownload() !== 'icon-1024.png') throw new Error('Download did not retain the image filename and contents')
      }
    }
    host.querySelectorAll<HTMLButtonElement>('button[aria-label="Expand image: Chart"]')[1].click()
    await until(() => document.querySelector<HTMLImageElement>('[role="dialog"] img')?.naturalWidth === 24)
  } finally {
    root.unmount()
    client.clear()
    host.remove()
    setApiBaseUrl(previousBackend)
  }
}
