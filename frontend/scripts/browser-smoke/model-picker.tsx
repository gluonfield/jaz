import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { AgentModelControls, useNewThreadControls } from '@/components/session/useNewThreadControls'
import type { AgentSettings, ModelCatalogEntry } from '@/lib/api/types'
import { NEW_SESSION_AGENT_KEY, NEW_SESSION_MODELS_KEY } from '@/lib/newSessionConfig'
import { ACPAgentsSettings } from '@/components/settings/ACPAgentsSettings'
import { ModelSelect } from '@/components/session/ModelSelect'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { useModelReasoningState } from '@/lib/modelReasoning'
import { modelCatalogQuery } from '@/lib/models'
import { keys } from '@/lib/query/keys'

export async function exerciseModelPicker(): Promise<void> {
  const model = (value: string, label: string, efforts: string[]): ModelCatalogEntry => ({
    value, label, reasoning: { status: 'ready', efforts, default_effort: 'medium' },
  })
  const efforts = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']
  const codex = [
    model('gpt-6-astra', 'GPT-6 Astra', [...efforts, 'ultra']),
    model('gpt-5.6-sol', 'GPT-5.6 Sol', [...efforts, 'ultra']),
    model('gpt-5.6-terra', 'GPT-5.6 Terra', efforts),
  ]
  const claude = [
    model('opus[1m]', 'Opus 5.5', [...efforts, 'ultracode']),
    model('fable[1m]', 'Fable 5.2', efforts),
    model('fable-5', 'Fable 5', efforts),
    model('sonnet', 'Sonnet 5', efforts),
    model('sonnet[1m]', 'Sonnet 5 (1M context)', efforts),
    model('haiku', 'Haiku 4.5', []),
  ]
  const providers = ['openai', 'openrouter'].map((id) => ({ id, label: id, base_url: '', implemented: true }))
  const settings: AgentSettings = {
    providers, agents: ['codex', 'claude'],
    acp_auth: Object.fromEntries(['codex', 'claude'].map((agent) => [agent, { authenticated: true, ready: true, api_key_configured: false, login_command_available: false }])),
    acp: {
      codex: { enabled: true, model_provider: 'openai', model: 'gpt-6-astra', reasoning_effort: 'medium' },
      claude: { enabled: true, model: 'opus[1m]', reasoning_effort: 'xhigh' },
    },
    acp_options: {
      codex: { local: true, supports_auth: false, reasoning_efforts: [], provider_mode: 'agent_defaults', model_providers: providers, models: codex, default_model_provider: 'openai', fast_mode_config_id: 'fast-mode', fast_mode_models: ['gpt-6-astra', 'gpt-5.6-sol'] },
      claude: { local: true, supports_auth: false, reasoning_efforts: [], models: claude },
    },
  }
  const originallyDark = document.documentElement.classList.contains('dark')
  document.documentElement.classList.add('dark')
  const originalFetch = window.fetch
  const originalAgent = localStorage.getItem(NEW_SESSION_AGENT_KEY)
  const originalModels = localStorage.getItem(NEW_SESSION_MODELS_KEY)
  localStorage.setItem(NEW_SESSION_AGENT_KEY, 'codex')
  localStorage.removeItem(NEW_SESSION_MODELS_KEY)
  let currentSettings = settings
  let holdProvider: Promise<void> | undefined
  let nativeCatalogue = [model('reported-model', 'Reported model', [])]
  let nativeDiscoveryFails = false
  window.fetch = async (input, init) => {
    const url = String(input)
    if (url.includes('/v1/settings/agents')) {
      if (init?.method === 'PUT') currentSettings = { ...currentSettings, ...JSON.parse(String(init.body)) }
      return Response.json(currentSettings)
    }
    if (url.includes('/v1/acp/agents/claude/models')) return Response.json({ models: claude })
    if (url.includes('/v1/acp/agents/antigravity/models')) return nativeDiscoveryFails ? Response.json({ error: 'Discovery failed' }, { status: 500 }) : Response.json({ models: nativeCatalogue })
    if (url.includes('/v1/model-providers/')) {
      await holdProvider
      return Response.json({ models: url.includes('/openai/') ? codex : [model('qwen/qwen3', 'Qwen 3', ['low', 'high'])] })
    }
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);padding:280px 24px 24px;z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let controls: ReturnType<typeof useNewThreadControls>
  const until = async (check: () => boolean) => {
    const end = Date.now() + 5000
    while (!check()) {
      if (Date.now() > end) {
        throw new Error('Model picker: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const button = (name: string) => [...document.querySelectorAll('button')].find((item) => item.getAttribute('aria-label') === name || item.textContent?.trim() === name)
  const slider = () => document.querySelector<HTMLInputElement>('input[type="range"]')!
  const frame = () => new Promise(requestAnimationFrame)
  const click = async (target: Element | undefined) => {
    if (!target) {
      throw new Error('Missing model picker control')
    }
    await frame()
    const rect = target.getBoundingClientRect()
    const x = Math.round(rect.x + rect.width / 2)
    const y = Math.round(rect.y + rect.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await frame()
    await window.smoke.pointer('mouseUp', x, y)
    await frame()
  }
  const configIs = (agent: string, model: string, effort: string) => {
    const config = controls.sessionConfig({ directory: '', worktree: false })
    return config.agent === agent && config.model === model && config.reasoning_effort === effort
  }
  function Composer() {
    controls = useNewThreadControls()
    return <div className="flex flex-wrap gap-2 rounded-card bg-surface p-3"><AgentModelControls controls={controls} /></div>
  }
  const render = (key: number) => root.render(<StrictMode><QueryClientProvider client={client}><Composer key={key} /></QueryClientProvider></StrictMode>)
  const openPicker = async () => {
    await click([...element.querySelectorAll('button')].find((item) => item.getAttribute('aria-label')?.startsWith('Model:')))
    await until(() => Boolean(slider()))
    await until(() => document.activeElement?.getAttribute('aria-checked') === 'true')
    await until(() => getComputedStyle(document.querySelector('[role="dialog"]')!.parentElement!).opacity === '1')
    assertNoFocusBorder(document.activeElement!)
  }
  const assertNoFocusBorder = (element: Element) => {
    const style = getComputedStyle(element)
    if (style.outlineStyle !== 'none' || style.boxShadow !== 'none') {
      throw new Error('Picker shows a focus border: ' + style.outline + ' / ' + style.boxShadow)
    }
  }
  const switchAgent = async (name: string, config: () => boolean) => {
    const panel = document.querySelector('[role="dialog"]')!
    const heights: number[] = []
    const untilTime = performance.now() + 500
    const measure = async () => {
      while (performance.now() < untilTime) {
        heights.push(panel.getBoundingClientRect().height)
        await frame()
      }
    }
    const measuring = measure()
    await click(button(name))
    await until(config)
    await measuring
    const first = heights[0]
    const last = heights.at(-1)!
    if (Math.abs(last - first) > 10 && !heights.some((height) => height > Math.min(first, last) + 2 && height < Math.max(first, last) - 2)) {
      throw new Error('Model list height jumped instead of animating')
    }
    if (document.activeElement?.getAttribute('aria-selected') !== 'true' || document.activeElement.getAttribute('aria-label') !== name) {
      throw new Error('Switching agent moved focus off its tab')
    }
    if (!element.querySelector(`button[aria-label*=", agent: ${name}"]`)) {
      throw new Error('The closed picker does not name the agent')
    }
    assertNoFocusBorder(document.activeElement)
  }
  const close = async () => {
    await window.smoke.key('Escape')
    await until(() => !document.querySelector('[role="dialog"]'))
    if (!document.activeElement?.getAttribute('aria-label')?.startsWith('Model:')) {
      throw new Error('Picker lost focus on Escape')
    }
  }

  try {
    render(0)
    await until(() => controls?.modelSuggestions.length === 3)
    await openPicker()
    await click(button('Fast Mode'))
    await until(() => controls.sessionConfig({ directory: '', worktree: false }).config_options?.['fast-mode'] === 'on')
    await close()
    if (!document.activeElement?.getAttribute('aria-label')?.endsWith(', Fast Mode')) {
      throw new Error('The closed picker hides that Fast Mode is on')
    }
    await openPicker()
    await window.smoke.key('Tab')
    if (document.activeElement !== button('Fast Mode')) {
      throw new Error('Natural Tab sequence missed Fast Mode')
    }
    assertNoFocusBorder(document.activeElement!)
    await window.smoke.key('Tab')
    if (document.activeElement !== slider()) {
      throw new Error('Natural Tab sequence missed the slider')
    }
    if (slider().max !== '4') {
      throw new Error('Codex must have five efforts')
    }
    assertNoFocusBorder(slider())
    await window.smoke.key('Home')
    await until(() => configIs('codex', 'gpt-6-astra', 'low'))
    for (const effort of ['medium', 'high', 'xhigh', 'ultra']) {
      await window.smoke.key('Right')
      await until(() => configIs('codex', 'gpt-6-astra', effort))
    }
    await click(button('GPT-5.6 Terra'))
    await until(() => configIs('codex', 'gpt-5.6-terra', 'medium'))
    await until(() => !button('Fast Mode'))
    if (controls.sessionConfig({ directory: '', worktree: false }).config_options) {
      throw new Error('Fast Mode was offered for a model without native support')
    }
    if (slider().max !== '3') {
      throw new Error('Terra shows unavailable Ultracode')
    }
    await click(button('GPT-6 Astra'))
    await until(() => configIs('codex', 'gpt-6-astra', 'medium') && Boolean(button('Fast Mode')))
    await window.smoke.key('Tab')
    await window.smoke.key('Tab')
    if (document.activeElement !== slider()) {
      throw new Error('Model selection lost keyboard focus: ' + document.activeElement?.outerHTML)
    }
    await window.smoke.key('End')
    await until(() => configIs('codex', 'gpt-6-astra', 'ultra'))
    await window.smoke.capture('codex-picker')
    await switchAgent('Claude', () => configIs('claude', 'opus[1m]', 'xhigh'))
    await window.smoke.capture('claude-models')
    await window.smoke.key('Tab')
    await until(() => document.activeElement?.textContent === 'Opus 5.5')
    await window.smoke.key('Down')
    await until(() => document.activeElement?.textContent === 'Fable 5.2')
    await window.smoke.key('Up')
    await until(() => document.activeElement?.textContent === 'Opus 5.5')
    await click(button('Opus 5.5'))
    await until(() => Boolean(slider()))
    await window.smoke.key('Tab')
    if (slider().max !== '4') {
      throw new Error('Claude must have five efforts')
    }
    assertNoFocusBorder(slider())
    await window.smoke.key('Home')
    await until(() => configIs('claude', 'opus[1m]', 'low'))
    for (const effort of ['medium', 'high', 'xhigh', 'ultracode']) {
      await window.smoke.key('Right')
      await until(() => configIs('claude', 'opus[1m]', effort))
    }
    const rect = slider().getBoundingClientRect()
    const y = Math.round(rect.y + rect.height / 2)
    const thumb = slider().parentElement!.querySelector<HTMLElement>('[data-effort-thumb]')!
    const fill = slider().parentElement!.querySelector<HTMLElement>('[data-effort-fill]')!
    const center = () => {
      const bounds = thumb.getBoundingClientRect()
      return bounds.x + bounds.width / 2
    }
    await until(() => Math.abs(center() - (rect.right - 14)) < 1)
    await window.smoke.pointer('mouseDown', Math.round(rect.right - 16), y)
    await frame()
    const positions: number[] = []
    const fillOffsets: number[] = []
    const end = performance.now() + 300
    const measuring = (async () => {
      while (performance.now() < end) {
        positions.push(center())
        fillOffsets.push(Math.abs(fill.getBoundingClientRect().right - center()))
        await frame()
      }
    })()
    await window.smoke.pointer('mouseMove', Math.round(rect.left + 16), y)
    await measuring
    const left = Math.min(...positions)
    const right = Math.max(...positions)
    const intermediate = positions.filter((x) => x > left + (right - left) * 0.1 && x < right - (right - left) * 0.1)
    if (right - left < 100 || intermediate.length < 2 || Math.max(...fillOffsets) > 1) {
      throw new Error('Slider motion did not ease the thumb and fill together: ' + JSON.stringify({ positions, fillOffsets }))
    }
    console.log('Effort slider motion:', JSON.stringify({ frames: positions.length, intermediate: intermediate.length, maxFillOffset: Math.max(...fillOffsets) }))
    await window.smoke.pointer('mouseMove', Math.round(rect.right - 16), y)
    await window.smoke.pointer('mouseUp', Math.round(rect.right - 16), y)
    await until(() => configIs('claude', 'opus[1m]', 'ultracode'))
    await new Promise((resolve) => setTimeout(resolve, 1000))
    await window.smoke.capture('claude-picker')
    await close()
    const previousControls = controls
    render(1)
    await until(() => controls !== previousControls && configIs('claude', 'opus[1m]', 'ultracode'))
    await openPicker()
    await switchAgent('Codex', () => configIs('codex', 'gpt-6-astra', 'ultra'))
    await close()
    settings.acp.codex = { ...settings.acp.codex, model_provider: 'openrouter', model: 'qwen/qwen3', reasoning_effort: 'high' }
    client.setQueryData(keys.agentSettings, { ...settings })
    await until(() => configIs('codex', 'qwen/qwen3', 'high'))
    if (controls.fastMode || controls.sessionConfig({ directory: '', worktree: false }).config_options) {
      throw new Error('Fast Mode leaked to a different provider')
    }
    settings.acp.codex = { ...settings.acp.codex, model_provider: 'openai', model: 'gpt-6-astra' }
    client.setQueryData(keys.agentSettings, { ...settings })
    await until(() => configIs('codex', 'gpt-6-astra', 'ultra'))
    await openPicker()
    if (button('Fast Mode')?.getAttribute('aria-checked') !== 'true') {
      throw new Error('Fast Mode was lost after changing model, agent, provider or remounting')
    }
    await click(button('GPT-6 Astra'))
    await close()
    await window.smoke.resize(390, 760)
    await openPicker()
    const bounds = document.querySelector('[role="dialog"]')!.getBoundingClientRect()
    if (bounds.left < 0 || bounds.right > innerWidth) {
      throw new Error('Picker overflows mobile viewport')
    }
    await window.smoke.capture('model-picker-mobile')
    await close()
    controls.setSelection({ model: 'gpt-6-astra', effort: 'max' })
    await until(() => configIs('codex', 'gpt-6-astra', 'max'))
    await openPicker()
    if (slider().max !== '4') {
      throw new Error('An inherited Max effort added an extra stop')
    }
    const firstStop = slider().getBoundingClientRect()
    await window.smoke.pointer('mouseDown', Math.round(firstStop.left + 16), Math.round(firstStop.y + firstStop.height / 2))
    await window.smoke.pointer('mouseUp', Math.round(firstStop.left + 16), Math.round(firstStop.y + firstStop.height / 2))
    await until(() => configIs('codex', 'gpt-6-astra', 'low'))
    await close()

    await window.smoke.resize(1050, 850)
    let availability: ReturnType<typeof useModelReasoningState>
    let picked = 'legacy-model'
    const unavailable: AgentSettings = {
      agents: ['antigravity'], providers: [],
      acp: { antigravity: { model: picked } },
      acp_options: { antigravity: { supports_auth: true, reasoning_efforts: [], models: claude } },
      acp_auth: { antigravity: { authenticated: false, ready: false, api_key_configured: false, login_command_available: false } },
    }
    function SettingsModel({ settings }: { settings: AgentSettings }) {
      availability = useModelReasoningState({ settings, agent: 'antigravity', model: picked, reasoningEffort: '', provider: '' })
      return <ModelCombobox value={picked} suggestions={availability.modelSuggestions} disabled={!availability.modelsReady} onChange={(value) => { picked = value }} aria-label="Antigravity model" />
    }
    const showSettings = (settings: AgentSettings) => root.render(<QueryClientProvider client={client}><SettingsModel settings={settings} /></QueryClientProvider>)
    const input = () => element.querySelector<HTMLInputElement>('input')!
    const nativeKey = modelCatalogQuery('antigravity').queryKey
    client.setQueryData(nativeKey, { models: [{ value: 'reported-model', label: 'Reported model', reasoning: { status: 'unavailable' } }], defaultModel: 'reported-model' })
    showSettings(unavailable)
    await until(() => input()?.disabled === true)
    if (availability!.modelSuggestions.length || input().value !== picked) throw new Error('Signed-out agent exposes cached or settings models, or loses saved model')
    const readySettings = { ...unavailable, acp_auth: { antigravity: { ...unavailable.acp_auth!.antigravity, authenticated: true, ready: true } } }
    showSettings(readySettings)
    await until(() => input()?.disabled === false)
    await click(input())
    await until(() => Boolean(document.querySelector('[role="listbox"]')))
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input(), 'invented-model')
    input().dispatchEvent(new Event('input', { bubbles: true }))
    await frame()
    if (picked !== 'legacy-model') throw new Error('Typing an unreported model saved it')
    for (const state of ['missing', 'downloading', 'failed', 'unsupported'] as const) {
      showSettings({ ...readySettings, acp_auth: { antigravity: { ...readySettings.acp_auth.antigravity, managed_adapter: { adapter: 'antigravity', state } } } })
      await until(() => input()?.disabled === true && !document.querySelector('[role="listbox"]'))
      if (availability!.modelSuggestions.length) throw new Error('Unavailable adapter exposes cached models')
    }
    showSettings(readySettings)
    await until(() => input()?.disabled === false)
    if (input().getAttribute('aria-expanded') !== 'false') throw new Error('Restoring readiness reopens the previous catalogue')
    await click(input())
    await click(button('Reported model'))
    await until(() => picked === 'reported-model')
    showSettings(readySettings)
    await click(input())
    await click(button('Default'))
    await until(() => picked === '')
    await click(input())
    showSettings(unavailable)
    await until(() => input()?.disabled === true && !document.querySelector('[role="listbox"]'))
    if (availability!.modelSuggestions.length) throw new Error('Sign-out exposes cached models')
    nativeCatalogue = [model('new-account-model', 'New account model', [])]
    showSettings(readySettings)
    await until(() => input()?.disabled === false)
    if (availability!.modelSuggestions[0]?.value !== 'new-account-model') throw new Error('Reconnect reuses the old account catalogue')
    client.setQueryData(nativeKey, { models: [] })
    await until(() => input()?.disabled === true)
    if (availability!.reasoningOptions.length) throw new Error('Empty discovery exposes reasoning controls')
    nativeDiscoveryFails = true
    await client.refetchQueries({ queryKey: nativeKey })
    await until(() => Boolean(availability!.modelsError) && input().disabled)
    if (availability!.modelSuggestions.length) throw new Error('Failed discovery exposes cached models')
    await window.smoke.capture('model-picker-unavailable')

    root.render(<ModelSelect value="runtime-default" effort="low" suggestions={[]} effortOptions={[{ value: 'low', label: 'Low' }, { value: 'high', label: 'High' }]} onChange={(selection) => { picked = selection.effort }} />)
    await until(() => Boolean(element.querySelector('button')))
    await click(element.querySelector('button')!)
    await until(() => Boolean(slider()))
    if (document.querySelector('[role="menuitemradio"], [role="tab"]')) throw new Error('Reasoning-only control offers a model or agent')
    slider().focus()
    await window.smoke.key('End')
    await until(() => picked === 'high')
    await window.smoke.key('Escape')
    const exitingSlider = document.querySelector<HTMLInputElement>('input[type="range"]')
    if (exitingSlider && !exitingSlider.closest('[inert]')) throw new Error('Closing picker leaves interactive exit content')

    currentSettings = { ...settings, acp: { ...settings.acp, codex: { ...settings.acp.codex, model_provider: 'openai', model: 'gpt-6-astra', reasoning_effort: 'medium' } }, acp_options: { ...settings.acp_options, codex: { ...settings.acp_options!.codex, supports_auth: true, auth_provider_id: 'openai', model_providers: providers.map((item) => item.id === 'openrouter' ? { ...item, default_model: 'qwen/qwen3' } : item) } } }
    client.setQueryData(keys.agentSettings, currentSettings)
    root.render(<QueryClientProvider client={client}><ACPAgentsSettings onOpenProviders={() => {}} /></QueryClientProvider>)
    const settingsInput = () => element.querySelector<HTMLInputElement>('[aria-label="Codex model"]')
    await until(() => Boolean(button('Expand Codex')))
    await click(button('Expand Codex'))
    await until(() => settingsInput()?.disabled === false && Boolean(settingsInput()?.closest('.overflow-visible')))
    await click(settingsInput()!)
    await click(button('Default'))
    await until(() => button('Save changes')?.disabled === false)
    await click(button('Save changes'))
    await until(() => currentSettings.acp.codex.model === '' && button('Save changes')?.disabled === true)
    if (currentSettings.acp.codex.reasoning_effort !== 'medium') throw new Error('Native Default clears the saved reasoning effort')
    await until(() => settingsInput()?.disabled === false)
    let releaseProvider: () => void
    holdProvider = new Promise<void>((resolve) => { releaseProvider = resolve })
    const refresh = client.invalidateQueries({ queryKey: keys.acpAgents })
    await until(() => settingsInput()?.disabled === true && element.textContent?.includes('Loading models…') === true)
    releaseProvider!()
    await refresh
    holdProvider = undefined
    await until(() => settingsInput()?.disabled === false)
    await click(element.querySelector('[aria-label="Codex provider"]')!)
    await click(button('openrouter'))
    await until(() => settingsInput()?.disabled === false)
    await click(settingsInput()!)
    if (button('Default')) throw new Error('API provider advertises an unsavable Default choice')
    await click(button('Qwen 3'))
    await until(() => button('Save changes')?.disabled === false)
    await click(button('Save changes'))
    await until(() => currentSettings.acp.codex.model_provider === 'openrouter' && button('Save changes')?.disabled === true)
    await click(element.querySelector('[aria-label="Codex provider"]')!)
    await click(button('openai'))
    await until(() => settingsInput()?.disabled === true)
    if (element.textContent?.includes('Loading models…')) throw new Error('Unsaved native auth route starts discovery with API-route readiness')


  } finally {
    document.documentElement.classList.toggle('dark', originallyDark)
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    for (const [key, value] of [[NEW_SESSION_AGENT_KEY, originalAgent], [NEW_SESSION_MODELS_KEY, originalModels]] as const) {
      if (value == null) {
        localStorage.removeItem(key)
      } else {
        localStorage.setItem(key, value)
      }
    }
    await window.smoke.resize(1100, 780)
  }
}
