import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { Composer } from '@/components/session/Composer'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'

export async function exerciseComposerSkills(): Promise<void> {
  const skills = [
    { name: 'tenders', description: 'Standalone prefix', path: '/fixture/skills/tenders/SKILL.md' },
    { name: 'query-sheet', description: 'Standalone query', path: '/fixture/skills/query-sheet/SKILL.md' },
    { name: 'tenders:query-sheet', description: 'Tender query', path: '/fixture/plugins/tenders/skills/query-sheet/SKILL.md' },
    { name: 'tenders.extra:query-sheet', description: 'Extra query', path: '/fixture/plugins/tenders.extra/skills/query-sheet/SKILL.md' },
  ]
  const originalFetch = window.fetch
  window.fetch = (input, init) => new URL(String(input), location.href).pathname === '/v1/skills'
    ? Promise.resolve(Response.json({ skills }))
    : originalFetch(input, init)
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1000;padding:48px;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const sent: string[] = []
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) throw new Error('Plugin skill mentions: ' + check.toString())
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const field = () => element.querySelector<HTMLTextAreaElement>('textarea')!
  const type = async (text: string) => {
    field().focus()
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(field(), text)
    field().dispatchEvent(new Event('input', { bubbles: true }))
    await until(() => field().value === text)
  }
  const select = async (name: string) => {
    await type('$' + name)
    const skill = skills.find((skill) => skill.name === name)!
    const option = () => [...document.querySelectorAll<HTMLElement>('[role="option"]')].find((option) => option.textContent === name + skill.description)
    await until(() => Boolean(option()))
    option()!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await until(() => field().value === '$' + name + ' ')
  }
  const send = async () => {
    const count = sent.length
    element.querySelector<HTMLButtonElement>('button[aria-label="Send message"]')!.click()
    await until(() => sent.length === count + 1 && field().value === '')
  }
  try {
    root.render(<QueryClientProvider client={client}>
      <div data-skill-transcript><MessageMarkdown text="$tenders:query-sheet $tenders.extra:query-sheet $query-sheet $tenders.extra:unknown $tenders $tenders:query-sheet. $tenders.md $tenders.extra:query-sheet.md" /></div>
      <Composer draftStorageKey="composer-plugin-skills-smoke" onSend={(text) => { sent.push(text) }} />
    </QueryClientProvider>)
    await until(() => Boolean(field()) && element.querySelectorAll('[data-skill-transcript] button').length === 5)
    for (const skill of skills) {
      const pill = [...element.querySelectorAll<HTMLButtonElement>('[data-skill-transcript] button')].find((button) => button.textContent === '$' + skill.name)
      if (pill?.title !== skill.path) throw new Error('Rendered skill did not retain its exact name and original path: ' + skill.name)
    }
    if (!element.querySelector('[data-skill-transcript]')!.textContent?.includes('$tenders:query-sheet. $tenders.md $tenders.extra:query-sheet.md')) throw new Error('Sentence punctuation or filename skill boundaries changed')
    for (const literal of ['$tenders:query-sheet', '$tenders.extra:query-sheet']) {
      await select('tenders')
      await type(literal)
      await send()
      if (sent.at(-1) !== literal) throw new Error('A plain skill half-matched a qualified name: ' + sent.at(-1))
    }
    await select('tenders:query-sheet')
    await send()
    if (sent.at(-1) !== '[$tenders:query-sheet](/fixture/plugins/tenders/skills/query-sheet/SKILL.md)') throw new Error('Qualified completion sent the wrong skill path: ' + sent.at(-1))
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    localStorage.removeItem('composer-plugin-skills-smoke')
  }
}
