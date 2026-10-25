import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { VoiceSettings } from '@/components/settings/VoiceSettings'
import { setApiBaseUrl } from '@/lib/api/client'
import type { VoiceSettings as Settings } from '@/lib/api/liveVoice'

export async function exerciseClaudeVoice(): Promise<void> {
  const native = window.jaz!.claudeVoice
  const status = await native.status()
  if (!status.available || status.models.length !== 1 || status.models[0].id !== 'fixture-opus') {
    throw new Error('Claude catalog did not use the signed-in browser session or remove retired models')
  }
  const events: { type: string; data?: { sender?: string } }[] = []
  const unsubscribe = native.onEvent((_id, data) => events.push(JSON.parse(data)))
  const options = { sdp: 'v=0\r\nfixture-offer', context: 'Thread context', model: 'fixture-opus', voice: 'buttery' }
  try {
    const answer = await native.start('voice-fixture', options)
    if (answer !== 'v=0\r\nfixture-answer') {
      throw new Error('Claude signaling did not return the media answer')
    }
    const windows = await window.smoke.claudeWindows()
    if (windows.length !== 1 || windows.some((window) => window.visible || window.focused || window.privilegedPage)) {
      throw new Error('Claude voice exposed, focused or granted privileges to its authentication broker')
    }
    if (!await window.smoke.claudeOwner()) {
      throw new Error('Another Jaz window could control this voice call')
    }
    await native.result('voice-fixture', 'tool-1', 'Agent accepted', false)
    if ((await window.smoke.claudeFixture()).results !== 1) {
      throw new Error('Claude client tool result did not reach the authenticated conversation endpoint')
    }
    await native.send('voice-fixture', JSON.stringify({ type: 'interrupt' }))
    await until(() => events.some((event) => event.type === 'server_interrupt'))
    if (events.some((event) => event.type === 'message_complete' && event.data?.sender === 'human')) {
      throw new Error('Host context leaked into the spoken-user transcript')
    }
    await native.close('voice-fixture')
    await until(async () => (await window.smoke.claudeFixture()).active === 0)
    if (!await window.smoke.claudeNavigation()) {
      throw new Error('Reloading the owning window left its Claude broker active')
    }
    await until(async () => (await window.smoke.claudeFixture()).active === 0)
    const opened = (await window.smoke.claudeFixture()).opened
    await window.smoke.claudeFixture(true)
    const pending = native.start('cancelled-start', options).then(() => false, () => true)
    await new Promise((resolve) => setTimeout(resolve, 20))
    await native.close('cancelled-start')
    if (!await pending) {
      throw new Error('A cancelled Claude connection completed successfully')
    }
    await new Promise((resolve) => setTimeout(resolve, 350))
    if ((await window.smoke.claudeFixture()).opened !== opened) {
      throw new Error('Cancelling Claude startup leaked a voice socket')
    }
  } finally {
    await native.close('voice-fixture')
    await native.close('cancelled-start')
    unsubscribe()
  }

  const originalFetch = window.fetch
  const originalBase = localStorage.getItem('jaz.backendUrl')
  const panel = document.createElement('div')
  panel.className = 'fixed inset-0 z-10 overflow-auto bg-bg px-4'
  document.body.append(panel)
  const root = createRoot(panel)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let settings: Settings = {
    agent: 'openai', provider: 'openai', voice: 'cove', voices: ['cove'],
    providers: [{ id: 'openai', label: 'OpenAI OAuth', available: true }, { id: 'claude', label: 'Claude', available: false, reason: 'Claude voice requires Jaz Desktop.' }],
  }
  window.fetch = async (input, init) => {
    const path = new URL(String(input), location.origin).pathname
    if (path === '/v1/auth/check') {
      return Response.json({ ok: true })
    }
    if (path === '/v1/settings/voice') {
      if (init?.method === 'PUT') {
        const update = JSON.parse(String(init.body))
        settings = { ...settings, ...update, voice: update.voice || (update.provider === 'claude' ? 'buttery' : 'cove'), voices: update.provider === 'claude' ? ['buttery', 'airy'] : ['cove'] }
      }
      return Response.json(settings)
    }
    return originalFetch(input, init)
  }
  setApiBaseUrl('https://voice-settings.fixture')
  const button = (text: string) => [...panel.querySelectorAll('button')].find((button) => button.textContent?.trim() === text)
  try {
    root.render(<QueryClientProvider client={client}><VoiceSettings /></QueryClientProvider>)
    await until(() => Boolean(panel.querySelector('[aria-label="Voice provider"]')))
    await click(panel.querySelector('[aria-label="Voice provider"]')!)
    await until(() => Boolean([...document.querySelectorAll('button')].find((button) => button.textContent?.trim().startsWith('Claude'))))
    await click([...document.querySelectorAll('button')].find((button) => button.textContent?.trim().startsWith('Claude'))!)
    await until(() => Boolean(panel.querySelector('[aria-label="Claude voice model"]'))).catch((error: Error) => {
      throw new Error(error.message + ' UI: ' + panel.textContent + ' saved: ' + JSON.stringify(settings))
    })
    await click(panel.querySelector('[aria-label="Claude voice model"]')!)
    await until(() => Boolean([...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === 'Opus')))
    await click([...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === 'Opus')!)
    await until(() => settings.model === 'fixture-opus' && Boolean(button('Opus')))
    if (settings.agent !== 'claude' || settings.voice !== 'buttery') {
      throw new Error('Claude selection did not preserve its provider, native model and voice')
    }
    for (const [width, name] of [[1280, 'desktop'], [390, 'phone']] as const) {
      await window.smoke.resize(width, 900)
      if (panel.scrollWidth > panel.clientWidth) {
        throw new Error('Claude voice settings overflow the ' + name + ' viewport')
      }
      await window.smoke.capture('claude-voice-' + name)
    }
  } finally {
    root.unmount()
    panel.remove()
    client.clear()
    window.fetch = originalFetch
    setApiBaseUrl(originalBase ?? location.origin)
  }
}

async function until(check: () => boolean | Promise<boolean>): Promise<void> {
  const deadline = Date.now() + 10000
  while (!await check()) {
    if (Date.now() > deadline) {
      throw new Error('Claude voice check timed out: ' + check.toString())
    }
    await new Promise((resolve) => setTimeout(resolve, 20))
  }
}

async function click(element: Element): Promise<void> {
  element.scrollIntoView({ block: 'nearest' })
  await new Promise(requestAnimationFrame)
  await Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished))
  const bounds = element.getBoundingClientRect()
  const x = Math.round(bounds.x + bounds.width / 2)
  const y = Math.round(bounds.y + bounds.height / 2)
  await window.smoke.pointer('mouseMove', x, y)
  await window.smoke.pointer('mouseDown', x, y)
  await window.smoke.pointer('mouseUp', x, y)
}
