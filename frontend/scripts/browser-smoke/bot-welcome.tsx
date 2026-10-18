import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BotWelcome } from '@/components/bots/BotWelcome'
import type { Bot } from '@/lib/api/types'

// A just-created bot types for a moment before its first message arrives; the
// same bot opened later shows the message at once. Picking a starter sends it.
export async function exerciseBotWelcome(): Promise<void> {
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const now = new Date().toISOString()
  const bot: Bot = { id: 'welcome', kind: 'bot', name: 'Welcome Bot', avatar: { shape: 'circle', color: 'blue' }, unread: false, status: 'idle', created_at: now, updated_at: now }
  const picked: string[] = []
  const render = (newBot: boolean) => flushSync(() => root.render(
    <QueryClientProvider client={client}>
      <BotWelcome key={String(newBot)} bot={bot} newBot={newBot} onPick={(text) => picked.push(text)} />
    </QueryClientProvider>,
  ))
  const starters = () => [...element.querySelectorAll('button')].filter((button) => /^[A-E]/.test(button.textContent ?? ''))
  try {
    render(false)
    if (element.querySelector('[role="status"]') || starters().length !== 5) {
      throw new Error('A reopened bot does not show its first message at once')
    }
    render(true)
    if (!element.querySelector('[role="status"]') || starters().length) {
      throw new Error('A new bot shows its first message without typing first')
    }
    const deadline = Date.now() + 3000
    while (!starters().length && Date.now() < deadline) {
      await new Promise(requestAnimationFrame)
    }
    if (element.querySelector('[role="status"]') || element.textContent?.includes("I'm Welcome Bot") !== true) {
      throw new Error('A new bot never finished typing its first message')
    }
    starters()[2].click()
    if (picked.join() !== 'Keep my CRM up to date from my email and chats') {
      throw new Error('Picking a starter sent ' + JSON.stringify(picked))
    }
  } finally {
    root.unmount()
    element.remove()
  }
}
