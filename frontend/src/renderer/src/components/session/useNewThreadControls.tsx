import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { ModelSelect, RuntimeSelect } from '@/components/session/NewThreadControls'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import type { CreateSessionInput } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { composerConfig } from '@/lib/appearanceConfig'
import {
  acpAgentModelSuggestions,
  modelSuggestionsForProvider,
  openRouterModelsQuery,
} from '@/lib/models'
import { createSessionInput, NEW_SESSION_AGENT_KEY } from '@/lib/newSessionConfig'
import { acpReasoningEffortOptions } from '@/lib/reasoningEfforts'

export function useNewThreadControls() {
  const settingsQuery = useQuery(agentSettingsQuery)
  const agentSettings = settingsQuery.data
  const agents = useMemo(() => enabledACPAgents(agentSettings), [agentSettings])
  const runtimeReady = settingsQuery.isSuccess
  const runtimeAvailable = runtimeReady && agents.length > 0

  const [runtime, setRuntime] = useState(() => localStorage.getItem(NEW_SESSION_AGENT_KEY) || 'jaz')
  const [providerOverride, setProviderOverride] = useState<string | null>(null)
  const [modelOverride, setModelOverride] = useState<string | null>(null)
  const [effortOverride, setEffortOverride] = useState<string | null>(null)

  const selectRuntime = (next: string) => {
    setRuntime(next)
    setProviderOverride(null)
    setModelOverride(null)
    setEffortOverride(null)
  }

  useEffect(() => {
    localStorage.setItem(NEW_SESSION_AGENT_KEY, runtime)
  }, [runtime])

  useEffect(() => {
    if (!runtimeReady || agents.includes(runtime)) return
    const next = agents.includes('jaz') ? 'jaz' : (agents[0] ?? '')
    if (next !== runtime) selectRuntime(next)
  }, [agents, runtime, runtimeReady])

  const model = runtimeModelState(agentSettings, runtime, providerOverride)
  const { usesProvider, providers: runtimeProviders, provider, selectedProvider } = model
  const selectedModel = modelOverride ?? model.defaultModel
  const effort = effortOverride ?? model.defaultEffort

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: usesProvider && provider === 'openrouter',
  })
  const modelSuggestions = usesProvider
    ? modelSuggestionsForProvider(selectedProvider, openRouterModels.data ?? [])
    : acpAgentModelSuggestions(runtime)

  const composer = composerConfig()

  return {
    agentSettings,
    agents,
    runtimeReady,
    runtimeAvailable,
    // Picker visibility lives here so the controls and the mobile summary agree.
    showAgentPicker: agents.length > 1,
    showModelPicker: !composer.hideModelPicker,
    showProjectPicker: !composer.hideProjectPicker,
    runtime,
    selectRuntime,
    model: selectedModel,
    modelSuggestions,
    modelsLoading: openRouterModels.isLoading,
    usesProvider,
    providers: usesProvider ? runtimeProviders.map((p) => ({ value: p.id, label: p.label })) : undefined,
    provider: usesProvider ? provider : undefined,
    setProvider: (next: string) => {
      setProviderOverride(next)
      setModelOverride(null)
      setEffortOverride(null)
    },
    setModel: (next: string) => setModelOverride(next),
    effort,
    effortOptions: acpReasoningEffortOptions(agentSettings, runtime),
    setEffort: (next: string) => setEffortOverride(next === '' ? null : next),
    sessionConfig: (extra: { directory: string; worktree: boolean }, title?: string): CreateSessionInput =>
      createSessionInput(
        agentSettings,
        { agent: runtime, ...extra, providerOverride, modelOverride, effortOverride },
        title,
      ),
  }
}

export function AgentModelControls({
  controls,
  placement,
  disabled,
}: {
  controls: ReturnType<typeof useNewThreadControls>
  placement?: 'above' | 'below'
  disabled?: boolean
}) {
  if (!controls.runtimeAvailable) {
    return controls.runtimeReady ? (
      <span className="px-1.5 text-[13px] text-ink-3">Connect an agent in Settings</span>
    ) : null
  }
  return (
    <>
      {controls.showAgentPicker ? (
        <RuntimeSelect
          value={controls.runtime}
          agents={controls.agents}
          placement={placement}
          disabled={disabled}
          onChange={controls.selectRuntime}
        />
      ) : null}
      {controls.showModelPicker ? (
        <ModelSelect
          value={controls.model}
          suggestions={controls.modelSuggestions}
          loading={controls.modelsLoading}
          placement={placement}
          disabled={disabled}
          onChange={controls.setModel}
          providers={controls.providers}
          provider={controls.provider}
          onProviderChange={controls.usesProvider ? controls.setProvider : undefined}
          effort={controls.effort}
          effortOptions={controls.effortOptions}
          onEffortChange={controls.setEffort}
        />
      ) : null}
    </>
  )
}
