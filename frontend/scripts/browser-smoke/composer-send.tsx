import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Composer } from '@/components/session/Composer'
import { useLiveSessionSend } from '@/components/session/useLiveSessionSend'

export async function exerciseComposerSend(): Promise<void> {
  const key = (id: string) => `composer-send-smoke:${id}`
  const requests: {
    message: string
    signal: AbortSignal
    persisted: boolean
    headers: () => void
    accept: () => void
    reject: () => void
  }[] = []
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    if (!String(input).endsWith('/messages:stream')) {
      return Response.json([])
    }
    const response = Promise.withResolvers<Response>()
    let body!: ReadableStreamDefaultController<Uint8Array>
    const stream = new ReadableStream<Uint8Array>({
      start: (controller) => {
        body = controller
      },
    })
    const signal = init!.signal!
    const frame = (type: string, error?: string) => {
      body.enqueue(new TextEncoder().encode(`data: ${JSON.stringify({ type, error })}\n\n`))
    }
    const request = {
      message: JSON.parse(String(init!.body)).message as string,
      signal,
      persisted: false,
      headers: () => response.resolve(new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } })),
      accept: () => {
        request.persisted = true
        frame('accepted')
      },
      reject: () => {
        frame('error', 'Message rejected')
        body.close()
      },
    }
    signal.addEventListener('abort', () => {
      const error = new DOMException('Observation cancelled', 'AbortError')
      response.reject(error)
      body.error(error)
    }, { once: true })
    requests.push(request)
    return response.promise
  }
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Composer send: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  function Chat({ id }: { id: string }) {
    const live = useLiveSessionSend({ sessionId: id, onCriticalError: () => {} })
    return <div data-chat={id}><Composer streaming={live.streaming} onSend={live.send} onStop={live.abort} draftStorageKey={key(id)} /></div>
  }
  const render = async (id: string) => {
    root.render(<StrictMode><QueryClientProvider client={client}><Chat key={id} id={id} /></QueryClientProvider></StrictMode>)
    await until(() => element.querySelector('[data-chat]')?.getAttribute('data-chat') === id)
  }
  const textarea = () => element.querySelector<HTMLTextAreaElement>('textarea')!
  const type = async (text: string) => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea(), text)
    textarea().dispatchEvent(new Event('input', { bubbles: true }))
    await until(() => JSON.parse(localStorage.getItem(key('a')) ?? '{}').text === text)
  }
  const send = async () => {
    const before = requests.length
    element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.click()
    await until(() => requests.length === before + 1 && textarea().value === '')
    return requests.at(-1)!
  }
  try {
    await render('a')
    await type('Unsent draft')
    await render('b')
    await render('a')
    if (textarea().value !== 'Unsent draft') {
      throw new Error('Navigation lost an unsent draft')
    }

    for (const headersFirst of [false, true]) {
      await type(`Sent before navigating: headers=${headersFirst}`)
      const request = await send()
      if (headersFirst) {
        request.headers()
      }
      request.accept()
      await render('b')
      if (!headersFirst) {
        request.headers()
      }
      await until(() => request.signal.aborted)
      await render('a')
      if (!request.persisted || textarea().value || localStorage.getItem(key('a'))) {
        throw new Error('An accepted message reappeared in the composer after navigation: ' + textarea().value)
      }
    }

    await type('Navigate before server acceptance')
    const pending = await send()
    pending.headers()
    await render('b')
    if (pending.signal.aborted) {
      throw new Error('Navigation cancelled submission before acceptance')
    }
    pending.accept()
    await until(() => pending.signal.aborted)
    await render('a')
    if (textarea().value) {
      throw new Error('Detached accepted message restored its draft')
    }

    await type('Stay through acceptance')
    const accepted = await send()
    accepted.headers()
    accepted.accept()
    await new Promise((resolve) => setTimeout(resolve, 30))
    if (accepted.signal.aborted) {
      throw new Error('Active chat detached its live observation')
    }
    await render('b')
    await until(() => accepted.signal.aborted)
    await render('a')

    await type('Rejected message')
    const rejected = await send()
    rejected.headers()
    rejected.reject()
    await until(() => textarea().value === rejected.message)
    await render('b')
    await render('a')
    if (textarea().value !== rejected.message) {
      throw new Error('Navigation lost a rejected message')
    }

    const retry = await send()
    retry.headers()
    retry.accept()
    await type('Next draft')
    await render('b')
    await until(() => retry.signal.aborted)
    await render('a')
    if (textarea().value !== 'Next draft') {
      throw new Error('Previous send overwrote the next draft')
    }

    const stopped = await send()
    await until(() => Boolean(element.querySelector('button[aria-label="Stop response"]')))
    element.querySelector<HTMLButtonElement>('button[aria-label="Stop response"]')!.click()
    await until(() => stopped.signal.aborted && textarea().value === stopped.message)
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    for (const id of ['a', 'b']) {
      localStorage.removeItem(key(id))
    }
  }
}
