import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRef } from 'react'
import { createRoot } from 'react-dom/client'
import { ChatLog } from '@/components/bots/ChatLog'
import type { ChatEntry } from '@/lib/bots'

export async function exerciseChatHistory(): Promise<void> {
  const bot = { id: 'bot-1', name: 'Sync Bot', avatar: { shape: 'circle', color: 'blue' }, kind: 'bot' } as never
  const entries: ChatEntry[] = Array.from({ length: 150 }, (_, i) => ({
    kind: 'bot', key: `entry-${i}`, at: new Date(Date.UTC(2026, 9, 1) + i * 60_000).toISOString(), botId: 'bot-1', name: 'Sync Bot', text: `Reply ${i}`,
  }))
  let earlierLoads = 0
  const paging = { hasEarlier: true, loading: false, loadEarlier: async () => {
    earlierLoads += 1
    return false
  } }
  function Chat() {
    const scrollRef = useRef<HTMLDivElement>(null)
    return <div ref={scrollRef} style={{ height: 600, overflowY: 'auto' }}>
      <ChatLog entries={entries} bots={[bot]} named={false} working={[]} scrollRef={scrollRef} paging={paging} />
    </div>
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Chat history: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const shown = () => [...element.querySelectorAll('p')].filter((paragraph) => paragraph.textContent?.startsWith('Reply ')).map((paragraph) => paragraph.textContent)
  const viewport = () => element.firstElementChild as HTMLDivElement
  try {
    root.render(<QueryClientProvider client={new QueryClient()}><Chat /></QueryClientProvider>)
    await until(() => shown().length > 0)
    viewport().scrollTop = viewport().scrollHeight
    await new Promise((resolve) => setTimeout(resolve, 100))
    if (shown().length !== 60 || shown()[0] !== 'Reply 90' || shown().at(-1) !== 'Reply 149') {
      throw new Error(`A bot chat rendered ${shown().length} entries instead of the newest 60`)
    }
    viewport().scrollTop = 0
    await until(() => shown().length === 120)
    viewport().scrollTop = 0
    await until(() => shown().length === 150)
    if (earlierLoads !== 0) {
      throw new Error('Earlier history was fetched while loaded entries were still hidden')
    }
    viewport().scrollTop = 0
    await until(() => earlierLoads === 1)
  } finally {
    root.unmount()
    element.remove()
  }
}
