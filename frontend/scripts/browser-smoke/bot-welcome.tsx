import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRef } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BotChat } from '@/components/bots/BotChat'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, ChatMessage } from '@/lib/api/types'

// A bot's chat opens with its hello. A just-created bot types it first; any
// other bot has said it already. Starters sit under it until the person says
// something, and the hello stays once they have.
export async function exerciseBotWelcome(): Promise<void> {
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  const now = new Date().toISOString()
  const bot: Bot = { id: 'welcome', kind: 'bot', name: 'Welcome Bot', avatar: { shape: 'circle', color: 'blue' }, unread: false, status: 'idle', created_at: now, updated_at: now }
  const sent: string[] = []
  const render = (newBot: boolean, messages: ChatMessage[] = []) => flushSync(() => root.render(
    <QueryClientProvider client={client}>
      <BotChat
        key={String(newBot)}
        bot={bot}
        bots={[bot]}
        newBot={newBot}
        messages={messages}
        events={[]}
        scrollRef={createRef()}
        paging={{ hasEarlier: false, loading: false, loadEarlier: async () => false }}
        working={false}
        threads={[]}
        send={(text) => sent.push(text)}
        showDetails={() => {}}
      />
    </QueryClientProvider>,
  ))
  const said = () => element.textContent?.includes("Hi, I'm Welcome Bot") === true
  const typing = () => element.textContent?.includes('Welcome Bot is typing') === true
  const starters = () => [...element.querySelectorAll('button')].filter((button) => /^[A-E]/.test(button.textContent ?? ''))
  try {
    render(false)
    if (typing() || !said() || starters().length !== 5) {
      throw new Error('A bot opened later does not show its hello and starters at once')
    }
    render(true)
    if (!typing() || said() || starters().length) {
      throw new Error('A new bot says hello without typing first')
    }
    const deadline = Date.now() + 3000
    while (!starters().length && Date.now() < deadline) {
      await new Promise(requestAnimationFrame)
    }
    if (typing() || !said()) {
      throw new Error('A new bot never finished typing its hello')
    }
    starters()[2].click()
    if (sent.join() !== 'Keep my CRM up to date from my email and chats') {
      throw new Error('Picking a starter sent ' + JSON.stringify(sent))
    }
    render(false, [{ seq: 1, role: 'user', content: sent[0], blocks: [], created_at: new Date().toISOString() }])
    if (!said() || starters().length) {
      throw new Error('After the first message the hello is gone or the starters remain')
    }
  } finally {
    root.unmount()
    element.remove()
  }
}
