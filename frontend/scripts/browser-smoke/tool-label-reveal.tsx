import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { Transcript } from '@/components/session/Transcript'
import { getAppearance, setAppearance } from '@/lib/appearance'
import type { ChatMessage, SessionEvent } from '@/lib/api/types'

export async function exerciseToolLabelReveal(): Promise<void> {
  const host = document.createElement('div')
  host.style.cssText = 'position: absolute; inset: 0; z-index: 1000; padding: 40px; background: var(--color-bg)'
  document.body.append(host)
  const root = createRoot(host)
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } })
  const scrollRef = { current: host }
  const appearance = getAppearance()
  const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches
  const time = (seq: number) => new Date(1000 * seq).toISOString()
  const message: ChatMessage = { seq: 1, role: 'user', created_at: time(1), content: 'Check the workspace', blocks: [] }
  const event = (seq: number, type: string, acp: Partial<NonNullable<SessionEvent['acp']>>): SessionEvent => ({
    session_id: 'reveal', seq, at: time(seq), type, acp: { id: 'reveal', agent: 'codex', state: 'running', ...acp },
  })
  const command = (seq: number, id: string, status = 'running') => event(seq, 'acp_tool', {
    tool_calls: [{ id, tool_name: 'exec_command', status, raw_input: { cmd: 'git status --short' } }],
  })
  const thought = (seq: number) => event(seq, 'acp_thought', { thought: 'Checking the workspace' })
  const render = (events: SessionEvent[], key = 'initial') => {
    flushSync(() => root.render(
      <StrictMode>
        <QueryClientProvider client={client}>
          <Transcript key={key} sessionId="reveal" messages={[message]} events={events} scrollRef={scrollRef} groupTurns working />
        </QueryClientProvider>
      </StrictMode>,
    ))
  }
  const sweeping = () => [...host.querySelectorAll<HTMLElement>('.tool-label-revealing')]
  const expectStill = (description: string) => {
    if (sweeping().length) throw new Error(`A tool label swept during ${description}`)
  }
  const nextFrame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))

  try {
    setAppearance({ effects: true, inlineShellCommands: false })
    await window.smoke.reducedMotion(false)
    const saved = command(3, 'saved', 'completed')
    render([saved])
    expectStill('initial history in a running thread')
    render([command(2, 'paged', 'completed'), saved])
    expectStill('pagination')

    const first = command(5, 'first')
    const second = command(6, 'second')
    const history = [saved, thought(4), first, second]
    render(history)
    const [label] = sweeping()
    const animations = label?.getAnimations({ subtree: true }) ?? []
    const sweep = animations.find((animation) => !(animation instanceof CSSAnimation))
    const caret = animations.find((animation) => animation instanceof CSSAnimation)
    if (sweeping().length !== 1 || label?.textContent !== 'Ran commands' || !sweep || !caret) {
      throw new Error('A grouped arrival did not start exactly one heading sweep')
    }
    sweep.pause()
    sweep.currentTime = 70
    const start = parseFloat(getComputedStyle(label, '::after').left)
    const row = label.closest('button')!.getBoundingClientRect()
    sweep.currentTime = 205
    const middle = parseFloat(getComputedStyle(label, '::after').left)
    sweep.currentTime = 340
    const late = parseFloat(getComputedStyle(label, '::after').left)
    if (!(middle - start > late - middle && late > middle)) {
      throw new Error('The caret did not ease toward the end of the label')
    }
    sweep.currentTime = 205
    caret.pause()
    caret.currentTime = 205
    if (Number(getComputedStyle(label, '::after').opacity) < 0.5 || getComputedStyle(label.firstElementChild!).maskImage === 'none') {
      throw new Error('The moving caret and text mask were not both visible')
    }
    await nextFrame()
    await nextFrame()
    await window.smoke.capture('tool-label-mid-sweep')
    const updated = [saved, thought(4), first, command(7, 'second', 'completed'), command(8, 'third')]
    render(updated)
    if (sweeping().length !== 1 || !label.getAnimations().includes(sweep)) {
      throw new Error('An update to a tool group restarted its sweep')
    }
    sweep.finish()
    await nextFrame()
    await nextFrame()
    expectStill('the settled label')
    const settled = label.closest('button')!.getBoundingClientRect()
    if (getComputedStyle(label.firstElementChild!).maskImage !== 'none' || row.x !== settled.x || row.y !== settled.y || row.width !== settled.width || row.height !== settled.height) {
      throw new Error('The settled label was still masked or the sweep moved its row')
    }
    await window.smoke.capture('tool-label-settled')

    const refreshed = [saved, thought(4), command(9, 'fourth'), command(10, 'first'), command(11, 'second'), command(12, 'third')]
    render(refreshed)
    expectStill('a coalesced update that places a new call before existing calls')
    render([saved])
    render(refreshed)
    expectStill('a history refresh that temporarily replaces the transcript')
    render(updated, 'reopened')
    expectStill('reopening a running thread')
    await window.smoke.reducedMotion(true)
    const reduced = [...updated, thought(9), command(10, 'reduced')]
    render(reduced, 'reopened')
    expectStill('Reduce Motion')
    await window.smoke.reducedMotion(false)
    setAppearance({ effects: false })
    const quiet = [...reduced, thought(11), command(12, 'quiet')]
    render(quiet, 'reopened')
    expectStill('Effects off')

    setAppearance({ effects: true, inlineShellCommands: true })
    const agent = event(14, 'acp_tool', {
      tool_calls: [{ id: 'agent', tool_name: 'codex.spawnAgent', status: 'running' }],
    })
    render([...quiet, command(13, 'inline'), agent], 'reopened')
    if (sweeping().map((element) => element.textContent).join() !== 'git status --short,Creating an agent') {
      throw new Error('New inline command and agent rows did not each sweep their label once')
    }
  } finally {
    root.unmount()
    client.clear()
    host.remove()
    setAppearance(appearance)
    await window.smoke.reducedMotion(reducedMotion)
  }
}
