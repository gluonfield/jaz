import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRef, Profiler } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BotChat } from '@/components/bots/BotChat'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, ChatMessage, SessionEvent } from '@/lib/api/types'

// A bot's chat opens with its hello. A just-created bot types it first; any
// other bot has said it already. Starters sit under it until the person says
// something, and the hello stays once they have.
export async function exerciseBotWelcome(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;overflow:auto;background:var(--color-bg);padding:24px'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(mcpEntrypointsQuery.queryKey, [])
  const now = new Date().toISOString()
  const bot: Bot = { id: 'welcome', kind: 'bot', name: 'Welcome Bot', avatar: { shape: 'circle', color: 'blue' }, unread: false, status: 'idle', created_at: now, updated_at: now }
  const sent: string[] = []
  let splitHello = false
  const render = (newBot: boolean, messages: ChatMessage[] = [], events: SessionEvent[] = []) => flushSync(() => root.render(
    <QueryClientProvider client={client}>
      <Profiler id="welcome" onRender={() => {
        splitHello ||= starters().length > 0 && !said()
      }}>
        <BotChat
          key={String(newBot)}
          bot={bot}
          bots={[bot]}
          newBot={newBot}
          messages={messages}
          events={events}
          scrollRef={createRef()}
          paging={{ hasEarlier: false, loading: false, loadEarlier: async () => false }}
          working={false}
          threads={[]}
          send={(text) => sent.push(text)}
          showDetails={() => {}}
        />
      </Profiler>
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
    while ((typing() || !said() || starters().length !== 5) && Date.now() < deadline) {
      await new Promise(requestAnimationFrame)
    }
    if (typing() || !said() || starters().length !== 5) {
      throw new Error('A new bot never finished typing its hello: ' + JSON.stringify({ typing: typing(), said: said(), starters: starters().map((button) => button.textContent) }))
    }
    if (splitHello) {
      throw new Error('The hello and its starters committed in separate renders')
    }
    starters()[2].click()
    if (sent.join() !== 'Keep my CRM up to date from my email and chats') {
      throw new Error('Picking a starter sent ' + JSON.stringify(sent))
    }
    render(false, [{ seq: 1, role: 'user', from_user: true, content: sent[0], blocks: [], created_at: new Date().toISOString() }])
    if (!said() || starters().length) {
      throw new Error('After the first message the hello is gone or the starters remain')
    }
    const text = 'Coordination status: keep  two spaces and a line break.\nNext action is ready.'
    const human: ChatMessage = { seq: 1, role: 'user', from_user: true, content: text, blocks: [{ type: 'text', text }], created_at: now }
    const internal: ChatMessage = { ...human, seq: 2, from_user: false }
    const events: SessionEvent[] = [
      { session_id: bot.id, seq: 1, at: now, type: 'bot_activity', bot_activity: { kind: 'message_received', label: 'Worker', text: 'Private internal update' } },
      { session_id: bot.id, seq: 2, at: now, type: 'room_message', room_message: { speaker: 'bot', name: bot.name, text: 'Your requested delivery is ready.' } },
    ]
    const bubbles = () => [...element.querySelectorAll('[class*="group/message"]')]
    const check = () => {
      const visible = bubbles().map((bubble) => bubble.textContent)
      if (visible.length !== 3 || visible.filter((value) => value?.includes('Coordination status:')).length !== 1 || !visible.some((value) => value?.includes('Your requested delivery is ready.')) || element.textContent?.includes('Message from Worker')) {
        throw new Error('Bot chat exposed internal coordination or lost a human/public delivery: ' + JSON.stringify(visible))
      }
      const paragraph = element.querySelector('[class*="bg-bubble-sent"] p')
      const bounds = paragraph?.getBoundingClientRect()
      if (!bounds || bounds.top < 0 || bounds.bottom > innerHeight || bounds.width < 20) {
        throw new Error('The human message is outside the captured viewport')
      }
      if (paragraph?.textContent !== text) {
        throw new Error('Bot chat altered source whitespace: ' + JSON.stringify(paragraph?.textContent))
      }
    }
    render(false, [human], events)
    render(false, [human, internal, { ...internal, seq: 3, from_user: undefined }], events)
    check()
    await new Promise(requestAnimationFrame)
    await window.smoke.capture('bot-message-visibility-desktop')
    await window.smoke.resize(390, 844)
    check()
    await new Promise(requestAnimationFrame)
    await window.smoke.capture('bot-message-visibility-mobile')
    render(false, [human, internal], events)
    check()
    await window.smoke.resize(1200, 850)
  } finally {
    root.unmount()
    element.remove()
  }
}
