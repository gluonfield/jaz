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
  const sweeping = () => [...host.querySelectorAll<HTMLElement>('.tool-row-revealing')]
  const sweepOn = (row: HTMLElement) => row.getAnimations().find((animation) => !(animation instanceof CSSAnimation))
  const caretOn = (row: HTMLElement) => row.getAnimations({ subtree: true }).find((animation) =>
    animation.effect instanceof KeyframeEffect && animation.effect.pseudoElement === '::after',
  )
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
    const sweep = label && sweepOn(label)
    const caret = label && caretOn(label)
    const content = label?.firstElementChild
    const spinner = content?.querySelector('.animate-spin')
    if (sweeping().length !== 1 || label?.textContent !== 'Ran commands' || !sweep || !caret ||
        !content?.querySelector('.lucide-square-terminal') || !content.querySelector('.lucide-chevron-right') || !spinner?.getAnimations().length) {
      throw new Error('A grouped arrival did not start exactly one heading sweep')
    }
    sweep.pause()
    sweep.currentTime = 140
    const start = parseFloat(getComputedStyle(label, '::after').left)
    const row = label.closest('button')!.getBoundingClientRect()
    sweep.currentTime = 280
    const middle = parseFloat(getComputedStyle(label, '::after').left)
    sweep.currentTime = 420
    const late = parseFloat(getComputedStyle(label, '::after').left)
    if (!(middle - start > late - middle && late > middle)) {
      throw new Error('The caret did not ease toward the end of the label')
    }
    sweep.currentTime = 280
    caret.pause()
    caret.currentTime = 280
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
    const inline = command(13, 'inline')
    render([...quiet, inline], 'reopened')
    const [shell] = sweeping()
    const shellSweep = shell && sweepOn(shell)
    const shellCaret = shell && caretOn(shell)
    const shellSpinner = shell?.querySelector('.animate-spin')
    if (!shellSweep || !shellCaret || !shellSpinner || !shell.textContent?.startsWith('$git status --short')) {
      throw new Error('The inline command did not reveal its prompt and status as one line')
    }
    const duration = Number(shellSweep.effect?.getComputedTiming().duration)
    shellSweep.pause()
    shellCaret.pause()
    shellSweep.currentTime = duration * .76
    shellCaret.currentTime = duration * .76
    const shellOrigin = shell.getBoundingClientRect().left
    const tail = shellSpinner.getBoundingClientRect()
    const crossing = getComputedStyle(shell, '::after')
    const crossingX = parseFloat(crossing.left)
    if (shellOrigin + crossingX >= tail.left || Number(crossing.opacity) >= .4) {
      throw new Error(`The caret did not dim before reaching the far-right spinner: ${crossing.opacity} at ${crossingX}px of ${tail.left - shellOrigin}px over ${duration}ms`)
    }
    shellSweep.currentTime = duration * .98
    if (shellOrigin + parseFloat(getComputedStyle(shell, '::after').left) < tail.right - 2) {
      throw new Error('The sweep stopped before the far-right spinner')
    }
    shellSweep.finish()
    await nextFrame()
    await nextFrame()
    const agent = event(14, 'acp_tool', {
      tool_calls: [{ id: 'agent', tool_name: 'codex.spawnAgent', status: 'running' }],
    })
    render([...quiet, inline, agent], 'reopened')
    const [agentRow] = sweeping()
    if (sweeping().length !== 1 || agentRow?.textContent !== 'Creating an agent' ||
        !agentRow.querySelector('.lucide-bot') || !agentRow.querySelector('.animate-spin')) {
      throw new Error('The standalone agent did not reveal its icon, label and spinner together')
    }
  } finally {
    root.unmount()
    client.clear()
    host.remove()
    setAppearance(appearance)
    await window.smoke.reducedMotion(reducedMotion)
  }
}
