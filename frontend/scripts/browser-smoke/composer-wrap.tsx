import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { ComposerCard } from '@/components/session/Composer'

// A bot chat composer keeps attach, draft and controls on one row until the
// draft wraps; then the draft spans the full width over the buttons, and goes
// back to one row once it fits again, in the same focused field.
export async function exerciseComposerWrap(): Promise<void> {
  const element = document.createElement('div')
  element.style.width = '560px'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  root.render(
    <QueryClientProvider client={client}>
      <ComposerCard chat streaming={false} onSend={async () => {}} />
    </QueryClientProvider>,
  )
  const draft = 'The composer keeps its buttons beside a short draft and moves them under a draft that wraps.'
  try {
    const deadline = Date.now() + 3000
    while (!element.querySelector('textarea') && Date.now() < deadline) {
      await new Promise(requestAnimationFrame)
    }
    const field = element.querySelector('textarea')!
    const attach = element.querySelector('[aria-label="Attach files"]')!
    field.focus()
    let below = false
    let flips = 0
    const typing = [...Array(draft.length + 1).keys()]
    for (const length of [...typing, ...typing.toReversed()]) {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(field, draft.slice(0, length))
      field.dispatchEvent(new Event('input', { bubbles: true }))
      await new Promise(requestAnimationFrame)
      const text = field.getBoundingClientRect()
      const style = getComputedStyle(field)
      const lines = Math.round((field.scrollHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom)) / parseFloat(style.lineHeight))
      const now = attach.getBoundingClientRect().top >= text.bottom - 1
      if (!now && lines > 1) {
        throw new Error(`A ${length}-character draft wraps beside the composer buttons`)
      }
      if (element.querySelector('textarea') !== field || document.activeElement !== field) {
        throw new Error('The draft field lost focus moving between one and two rows')
      }
      flips += Number(now !== below)
      below = now
    }
    if (flips !== 2) {
      throw new Error(`The buttons moved ${flips} times typing and deleting one draft, not twice`)
    }
  } finally {
    root.unmount()
    element.remove()
  }
}
