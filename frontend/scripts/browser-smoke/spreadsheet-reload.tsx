import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { LOCAL_BACKEND_URL } from '@shared/localBackend'

export async function exerciseSpreadsheetReload() {
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:9999'
  document.body.append(host)
  const root = createRoot(host)
  const client = new QueryClient()
  const previousBackend = apiBaseUrl()
  setApiBaseUrl(LOCAL_BACKEND_URL)
  const cell = (text: string) => host.querySelector(`[role="gridcell"][aria-label="A2: ${text}"]`)
  const until = async (description: string, check: () => boolean) => {
    const deadline = performance.now() + 8000
    while (!check()) {
      if (performance.now() > deadline) throw new Error(description)
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  try {
    root.render(<QueryClientProvider client={client}>
      <FileReaderPanel sessionId="tabs" fileRef={{ path: 'workbook.xlsx' }} visible working onOpenFile={() => {}} onClose={() => {}} />
    </QueryClientProvider>)
    await until('The workbook did not open', () => Boolean(cell('Motor')))
    const grid = host.querySelector('[role="grid"]')
    const rewritten = await fetch('/workbook-fixture', { method: 'POST' })
    if (!rewritten.ok) throw new Error('The workbook fixture was not rewritten')
    await until('The open workbook did not follow the rewritten file', () => Boolean(cell('Gearbox')))
    if (host.querySelector('[role="grid"]') !== grid) throw new Error('The workbook was reopened from scratch')
  } finally {
    root.unmount()
    host.remove()
    client.clear()
    setApiBaseUrl(previousBackend)
  }
}
