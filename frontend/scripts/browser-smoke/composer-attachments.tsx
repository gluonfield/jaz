import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { Composer } from '@/components/session/Composer'
import type { Attachment } from '@/lib/api/types'

// Many attachments keep the composer in view: four previews and a +N control
// whose menu lists, flags and removes the rest, and every file still sends.
export async function exerciseComposerAttachments(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;left:24px;bottom:24px;width:760px;z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const uploads = new Map<string, PromiseWithResolvers<Attachment>>()
  const sent: string[][] = []
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Composer attachments: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const canvas = document.createElement('canvas')
  canvas.width = 8
  canvas.height = 8
  canvas.getContext('2d')!.fillRect(0, 0, 8, 8)
  const png = await new Promise<Blob>((resolve) => canvas.toBlob((blob) => resolve(blob!), 'image/png'))
  const add = (names: string[]) => {
    const input = element.querySelector<HTMLInputElement>('input[type="file"]')!
    const transfer = new DataTransfer()
    for (const name of names) {
      transfer.items.add(name.endsWith('.png') ? new File([png], name, { type: 'image/png' }) : new File([name], name, { type: 'text/plain' }))
    }
    input.files = transfer.files
    input.dispatchEvent(new Event('change', { bubbles: true }))
  }
  const upload = (name: string) => uploads.get(name)!.resolve({ id: 'id-' + name, name })
  const click = async (target: Element) => {
    const rect = target.getBoundingClientRect()
    const x = Math.round(rect.x + rect.width / 2)
    const y = Math.round(rect.y + rect.height / 2)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const previews = () => [...element.querySelectorAll('[aria-label^="Remove "]')].map((button) => button.getAttribute('aria-label')!.slice(7))
  const overflow = () => element.querySelector<HTMLButtonElement>('button[aria-label$=" more attachment"], button[aria-label$=" more attachments"]')
  const more = () => overflow()?.getAttribute('aria-label') ?? ''
  const listed = () => [...document.body.querySelectorAll(':scope > [data-escape-surface] [aria-label^="Remove "]')]
  const listedNames = () => listed().map((button) => button.getAttribute('aria-label')!.slice(7)).join()
  const card = () => element.firstElementChild!.getBoundingClientRect()
  try {
    root.render(<QueryClientProvider client={client}><Composer streaming={false}
      onSend={(_text, options) => {
        sent.push((options?.attachments ?? []).map((attachment) => attachment.id))
      }}
      onUploadAttachment={(file) => {
        const pending = Promise.withResolvers<Attachment>()
        uploads.set(file.name, pending)
        return pending.promise
      }} /></QueryClientProvider>)
    await until(() => Boolean(element.querySelector('textarea')))

    add(['a.png', 'b.png', 'c.png', 'd.png', 'e.txt'])
    await until(() => more() === '1 more attachment')
    const fiveTall = card().height
    const first = element.querySelector('[aria-label="Remove a.png"]')!.parentElement!
    if (Math.abs(overflow()!.getBoundingClientRect().top - first.getBoundingClientRect().top) > 1) {
      throw new Error('The +N control wrapped below four image previews in a thread-width composer')
    }
    add(['f.txt', 'g.txt', 'h.png', 'i.txt'])
    await until(() => more() === '5 more attachments')
    if (previews().join() !== 'a.png,b.png,c.png,d.png' || !overflow()!.textContent!.startsWith('+5')) {
      throw new Error('Nine attachments did not show four previews and +5: ' + previews().join())
    }
    if (card().height !== fiveTall || card().bottom > window.innerHeight || card().top < 0) {
      throw new Error(`Nine attachments grew the composer from ${fiveTall}px to ${card().height}px`)
    }

    for (const name of ['a.png', 'b.png', 'c.png', 'd.png', 'e.txt', 'h.png', 'i.txt']) {
      upload(name)
    }
    uploads.get('f.txt')!.reject(new Error('Upload refused'))
    await until(() => overflow()!.querySelector('[role="status"]')?.textContent === 'Failed')

    await click(overflow()!)
    await until(() => listedNames() === 'e.txt,f.txt,g.txt,h.png,i.txt' && getComputedStyle(document.body.querySelector(':scope > [data-escape-surface]')!).opacity === '1')
    await window.smoke.capture('composer-attachments')
    const statuses = [...document.body.querySelectorAll(':scope > [data-escape-surface] [role="status"]')].map((status) => status.textContent).join()
    if (statuses !== 'Failed,Uploading' || overflow()!.getAttribute('aria-expanded') !== 'true') {
      throw new Error('The +N menu did not flag the failed and uploading attachments: ' + statuses)
    }
    if (document.activeElement !== listed()[0]) {
      throw new Error('The +N menu opened without focus on its first remove button')
    }
    await click(listed()[1])
    await until(() => listedNames() === 'e.txt,g.txt,h.png,i.txt' && more() === '4 more attachments')
    if (overflow()!.querySelector('[role="status"]')?.textContent !== 'Uploading') {
      throw new Error('Removing the failed attachment left the +N control flagged as failed')
    }
    upload('g.txt')
    await until(() => !overflow()!.querySelector('[role="status"]') && !document.body.querySelector(':scope > [data-escape-surface] [role="status"]'))

    await click(overflow()!)
    await until(() => listed().length === 0 && overflow()!.getAttribute('aria-expanded') === 'false')
    element.querySelector<HTMLButtonElement>('[aria-label="Remove b.png"]')!.click()
    await until(() => previews().join() === 'a.png,c.png,d.png,e.txt' && overflow()!.textContent === '+3')
    element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.click()
    await until(() => sent.length === 1)
    if (sent[0].join() !== 'id-a.png,id-c.png,id-d.png,id-e.txt,id-g.txt,id-h.png,id-i.txt') {
      throw new Error('The sent message lost attachments behind the +N control: ' + sent[0].join())
    }

    await until(() => previews().length === 0)
    add(['j.txt', 'k.txt', 'l.txt', 'm.txt', 'n.txt'])
    await until(() => more() === '1 more attachment')
    await click(overflow()!)
    await until(() => listedNames() === 'n.txt')
    await click(listed()[0])
    await until(() => !overflow() && listed().length === 0)
    add(['o.txt'])
    await until(() => more() === '1 more attachment')
    await new Promise(requestAnimationFrame)
    if (listed().length !== 0) {
      throw new Error('A new overflow reopened the menu its emptied predecessor left open')
    }
  } finally {
    root.unmount()
    client.clear()
    element.remove()
  }
}
