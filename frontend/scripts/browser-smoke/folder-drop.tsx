import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { Composer } from '@/components/session/Composer'
import { ToastProvider } from '@/components/ui/toast'
import { apiBaseUrl, setApiBaseUrl } from '@/lib/api/client'
import { DEFAULT_API_BASE_URL } from '@/lib/clientRuntime'

export async function exerciseFolderDrop(): Promise<void> {
  const sent: string[] = []
  const originalFetch = window.fetch
  const originalBase = apiBaseUrl()
  window.fetch = async () => new Response(null, { status: 404 })
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:auto 24px 24px;z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Folder drop: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const textarea = () => element.querySelector<HTMLTextAreaElement>('textarea')!
  const drop = () => {
    const rect = textarea().getBoundingClientRect()
    return window.smoke.dropFolder(Math.round(rect.x + rect.width / 2), Math.round(rect.y + rect.height / 2))
  }
  try {
    root.render(<QueryClientProvider client={client}><ToastProvider><Composer streaming={false} onSend={(text) => {
      sent.push(text)
    }} /></ToastProvider></QueryClientProvider>)
    await until(() => Boolean(element.querySelector('textarea')))

    setApiBaseUrl(DEFAULT_API_BASE_URL)
    const folder = await drop()
    await until(() => textarea().value === '@Folder fixture ')
    element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.click()
    await until(() => sent.length === 1)
    if (sent[0] !== `[@Folder fixture](<${folder}>)`) {
      throw new Error('A dropped folder did not send its absolute path: ' + sent[0])
    }

    setApiBaseUrl(originalBase)
    await drop()
    await until(() => element.textContent!.includes('Folders attach only in the desktop app with a local backend'))
    if (textarea().value || element.querySelector('button[aria-label^="Remove "]')) {
      throw new Error('A remote backend accepted a client folder')
    }
  } finally {
    setApiBaseUrl(originalBase)
    window.fetch = originalFetch
    root.unmount()
    client.clear()
    element.remove()
  }
}
