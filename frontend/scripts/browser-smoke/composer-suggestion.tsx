import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { Composer } from '@/components/session/Composer'
import { useLiveSessionSend } from '@/components/session/useLiveSessionSend'

export async function exerciseComposerSuggestion(): Promise<void> {
  const prediction = 'Run the tests and give me a short summary of any failures you find.'
  const finishes: (() => void)[] = []
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    if (new URL(String(input), window.location.href).pathname !== '/v1/sessions/suggestion/messages:stream') {
      return originalFetch(input, init)
    }
    let body!: ReadableStreamDefaultController<Uint8Array>
    const stream = new ReadableStream<Uint8Array>({
      start: (controller) => {
        body = controller
        controller.enqueue(new TextEncoder().encode('data: {"type":"accepted"}\n\n'))
      },
    })
    finishes.push(() => body.close())
    return new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1000;padding:48px;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  function Thread({ chat, suggestedPrompt }: { chat: boolean; suggestedPrompt?: string }) {
    const live = useLiveSessionSend({ sessionId: 'suggestion', onCriticalError: () => {} })
    return (
      <div style={{ width: chat ? 320 : 640 }}>
        <Composer chat={chat} streaming={live.streaming} suggestedPrompt={suggestedPrompt} onSend={live.send} onStop={live.abort}
          placeholder="Message" draftStorageKey={`composer-suggestion-smoke:${chat}`} />
      </div>
    )
  }
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Composer suggestion: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const field = () => element.querySelector<HTMLTextAreaElement>('textarea')!
  const hint = () => element.querySelector('kbd')
  const setDraft = (text: string) => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(field(), text)
    field().dispatchEvent(new Event('input', { bubbles: true }))
  }
  try {
    for (const chat of [false, true]) {
      const layout = chat ? 'bot chat' : 'thread'
      root.render(<QueryClientProvider client={client}><Thread chat={chat} /></QueryClientProvider>)
      await until(() => Boolean(element.querySelector('textarea')))
      const placeholder = field().placeholder
      const emptyHeight = field().getBoundingClientRect().height
      root.render(<QueryClientProvider client={client}><Thread chat={chat} suggestedPrompt={prediction} /></QueryClientProvider>)
      await until(() => hint()?.textContent?.trim() === 'Tab' && hint()?.previousElementSibling?.textContent === prediction)
      const keycap = hint()!
      const bounds = keycap.getBoundingClientRect()
      if (bounds.left <= keycap.previousElementSibling!.getBoundingClientRect().right || bounds.right > field().getBoundingClientRect().right) {
        throw new Error(`The ${layout} Tab keycap does not fit after the suggestion`)
      }
      await window.smoke.capture(chat ? 'suggested-prompt-bot-chat' : 'suggested-prompt-thread')
      field().focus()
      await window.smoke.key('Tab')
      await until(() => field().value === prediction && !hint())
      if (document.activeElement !== field() || field().selectionStart !== prediction.length) {
        throw new Error(`Tab in a ${layout} left the composer or the caret short of the accepted prompt`)
      }
      setDraft('')
      await until(() => Boolean(hint()))
      if (field().getBoundingClientRect().height !== emptyHeight) {
        throw new Error(`A long prediction changes the empty ${layout} composer's height`)
      }
      await window.smoke.key('x')
      if (field().placeholder !== placeholder || hint()) {
        throw new Error(`A ${layout} draft still carries the prediction`)
      }
      await window.smoke.key('Tab')
      if (field().value !== 'x') {
        throw new Error(`Tab after typing in a ${layout} wrote ${JSON.stringify(field().value)}`)
      }
      field().focus()
      setDraft('Ship it')
      await until(() => !element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.disabled)
      element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.click()
      await until(() => finishes.length > 0 && field().value === '' && Boolean(element.querySelector('button[aria-label="Stop response"]')))
      if (field().placeholder !== placeholder || hint()) {
        throw new Error(`A running ${layout} turn still offers the old prediction`)
      }
      field().focus()
      await window.smoke.key('Tab')
      if (field().value !== '') {
        throw new Error(`Tab during a running ${layout} turn wrote ${JSON.stringify(field().value)}`)
      }
      finishes.shift()!()
      await until(() => Boolean(element.querySelector('button[aria-label="Send message"], button[aria-label="Voice mode"]')) && !element.querySelector('button[aria-label="Stop response"]'))
      root.render(<QueryClientProvider client={client}><Thread chat={chat} /></QueryClientProvider>)
      await until(() => field().placeholder === placeholder)
    }
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    for (const chat of [false, true]) {
      localStorage.removeItem(`composer-suggestion-smoke:${chat}`)
    }
  }
}
