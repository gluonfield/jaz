import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { ChatLog } from '@/components/bots/ChatLog'
import type { SessionEvent } from '@/lib/api/types'
import { botChat } from '@/lib/bots'
import { coalesceSessionEvents } from '@/lib/sessionEvents'

export async function exerciseBotArtifactDelivery() {
  const client = new QueryClient()
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const event: SessionEvent = {
    session_id: 'artifact-bot', seq: 316, type: 'artifact', at: '2026-10-30T13:17:10Z',
    artifact: {
      presented: true, title: 'Supply chain map',
      widget_code: `<h1>Supply chain map</h1>
<svg viewBox="0 0 680 100"><text x="20" y="50">Mill → Service centre → Shop → Buyer</text></svg>
<button onclick="sendPrompt('Explain service centres')">Explain this layer</button>
<script>
setTimeout(() => {
  document.querySelector('button').click()
  parent.postMessage({type: 'map-ready'}, '*')
}, 50)
</script>`,
    },
  }
  let update: (events: SessionEvent[]) => void = () => {}
  let ready = 0
  const prompts: string[] = []
  const receive = (message: MessageEvent) => {
    if (message.source === element.querySelector('iframe')?.contentWindow && message.data?.type === 'map-ready') ready += 1
  }
  window.addEventListener('message', receive)
  function Chat({ initial = [] }: { initial?: SessionEvent[] }) {
    const [events, setEvents] = useState(initial)
    update = setEvents
    const scrollRef = useRef<HTMLDivElement>(null)
    const { entries } = botChat([], coalesceSessionEvents(events), { id: event.session_id, name: 'Manufacturing Bot' }, [])
    return <div ref={scrollRef}>
      <ChatLog entries={entries} bots={[]} named={false} working={[]} scrollRef={scrollRef}
        paging={{ hasEarlier: false, loading: false, loadEarlier: async () => false }}
        onSendPrompt={(text) => prompts.push(text)} />
    </div>
  }
  const until = async (check: () => boolean) => {
    const deadline = performance.now() + 4000
    while (!check()) {
      if (performance.now() > deadline) throw new Error('Artifact delivery: ' + check.toString())
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const render = (key: string, initial?: SessionEvent[]) => root.render(
    <QueryClientProvider client={client}><Chat key={key} initial={initial} /></QueryClientProvider>,
  )
  try {
    render('live')
    await until(() => Boolean(element.firstElementChild))
    update([event])
    await until(() => ready === 1 && prompts.length === 1)
    const frame = element.querySelector('iframe')!
    if (frame.title !== 'Supply chain map' || frame.sandbox.value !== 'allow-scripts allow-forms') {
      throw new Error('Bot artifact did not reuse the sandboxed artifact renderer')
    }
    update([JSON.parse(JSON.stringify(event)), event])
    await new Promise((resolve) => setTimeout(resolve, 100))
    if (element.querySelectorAll('iframe').length !== 1 || element.querySelector('iframe') !== frame || ready !== 1) {
      throw new Error('History replay duplicated or remounted the live artifact')
    }
    render('reloaded', [JSON.parse(JSON.stringify(event))])
    await until(() => ready === 2 && prompts.length === 2)
    if (prompts.some((text) => text !== 'Explain service centres')) throw new Error('Artifact prompt bridge lost its message')
    const reloaded = element.querySelector('iframe')!
    const rect = reloaded.getBoundingClientRect()
    if (rect.width < 100 || rect.height < 100 || document.elementFromPoint(rect.left + 20, rect.top + 20) !== reloaded) {
      throw new Error(`Artifact is not visible: ${JSON.stringify(rect)}`)
    }
    await new Promise(requestAnimationFrame)
    await new Promise(requestAnimationFrame)
    await window.smoke.capture('bot-artifact-reloaded')
    update([{ ...event, seq: 317, artifact: { presented: true, title: 'Broken map', widget_code: '<script>throw new Error("Map rendering failed")</script>' } }])
    await until(() => Boolean(element.querySelector('pre')?.textContent?.includes('Map rendering failed')))
  } finally {
    root.unmount()
    element.remove()
    client.clear()
    window.removeEventListener('message', receive)
  }
}
