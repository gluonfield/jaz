import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { ComposerCard } from '@/components/session/Composer'

// While a turn runs, hovering send offers Send and Queue; the menu survives
// the gap between button and menu, and each row reaches its own handler.
export async function exerciseComposerSendMenu(): Promise<void> {
  const element = document.createElement('div')
  element.style.width = '560px'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const calls: string[] = []
  const render = (streaming: boolean) => root.render(
    <QueryClientProvider client={client}>
      <ComposerCard
        streaming={streaming}
        sendWhileStreaming
        clearTiming="immediate"
        onSend={async (text) => {
          calls.push(`send:${text}`)
        }}
        onQueuePrompt={async (text) => {
          calls.push(`queue:${text}`)
        }}
      />
    </QueryClientProvider>,
  )
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Composer send menu: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const textarea = () => element.querySelector<HTMLTextAreaElement>('textarea')!
  const sendButton = () => element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!
  const row = (label: string) => [...document.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent?.trim().startsWith(label) && !element.contains(button))
  const type = async (text: string) => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea(), text)
    textarea().dispatchEvent(new Event('input', { bubbles: true }))
    await until(() => !sendButton().disabled)
  }
  const pointer = (type: 'pointerover' | 'pointerout', target: Element, relatedTarget: Element | null) => {
    target.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerType: 'mouse', relatedTarget }))
  }
  try {
    render(false)
    await until(() => Boolean(element.querySelector('textarea')))
    await type('Idle draft')
    pointer('pointerover', sendButton(), null)
    await new Promise((resolve) => setTimeout(resolve, 50))
    if (row('Queue')) {
      throw new Error('An idle composer offered Queue')
    }
    pointer('pointerout', sendButton(), document.body)

    render(true)
    for (const [label, text] of [['Queue', 'Queued follow-up'], ['Send', 'Steer now']]) {
      await type(text)
      pointer('pointerover', sendButton(), document.body)
      await until(() => Boolean(row('Queue')))
      pointer('pointerout', sendButton(), document.body)
      pointer('pointerover', row(label)!, document.body)
      await new Promise((resolve) => setTimeout(resolve, 400))
      const choice = row(label)
      if (!choice) {
        throw new Error('The send menu closed while the pointer was on it')
      }
      choice.click()
      await until(() => calls.length > 0 && textarea().value === '')
      const expected = `${label === 'Queue' ? 'queue' : 'send'}:${text}`
      if (calls.join() !== expected) {
        throw new Error(`${label} from the send menu called ${calls.join()}, not ${expected}`)
      }
      calls.length = 0
      await until(() => !row('Queue'))
    }

    // Touch has no hover: pressing and holding opens the menu, lifting the
    // finger neither closes it nor sends.
    await type('Held follow-up')
    const touch = (type: 'touchstart' | 'touchend') => {
      const point = new Touch({ identifier: 1, target: sendButton(), clientX: 1, clientY: 1 })
      sendButton().dispatchEvent(new TouchEvent(type, { bubbles: true, cancelable: true, touches: type === 'touchstart' ? [point] : [], changedTouches: [point] }))
    }
    touch('touchstart')
    await until(() => Boolean(row('Queue')))
    touch('touchend')
    sendButton().dispatchEvent(new PointerEvent('pointerout', { bubbles: true, pointerType: 'touch', relatedTarget: document.body }))
    sendButton().click()
    await new Promise((resolve) => setTimeout(resolve, 400))
    if (!row('Queue') || calls.length > 0) {
      throw new Error(`Lifting a held send ${calls.length > 0 ? 'sent the draft' : 'closed the menu'}`)
    }
    row('Queue')!.click()
    await until(() => calls.join() === 'queue:Held follow-up')
  } finally {
    root.unmount()
    element.remove()
  }
}
