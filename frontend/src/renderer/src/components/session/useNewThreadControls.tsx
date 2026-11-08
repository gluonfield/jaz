import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { RuntimeSelect } from '@/components/session/NewThreadControls'
import { ModelSelect } from '@/components/session/ModelSelect'
import { ReasoningSelect } from '@/components/session/ReasoningSelect'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import type { CreateSessionInput } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { composerConfig } from '@/lib/jazDefaults'
import { useModelReasoningState } from '@/lib/modelReasoning'
import { parseModelSelections, pickerEffortOptions, type ModelSelection } from '@/lib/modelPicker'
import { modelSuggestionFor } from '@/lib/modelSuggestion'
import { NEW_SESSION_AGENT_KEY, NEW_SESSION_MODELS_KEY } from '@/lib/newSessionConfig'

export function useNewThreadControls() {
  const settingsQuery = useQuery(agentSettingsQuery)
  const agentSettings = settingsQuery.data
  const agents = useMemo(() => enabledACPAgents(agentSettings), [agentSettings])
  const runtimeReady = settingsQuery.isSuccess
  const runtimeAvailable = runtimeReady && agents.length > 0

  const [runtime, setRuntime] = useState(() => localStorage.getItem(NEW_SESSION_AGENT_KEY) || '')
  const [selections, setSelections] = useState(() => parseModelSelections(localStorage.getItem(NEW_SESSION_MODELS_KEY)))

  const selectRuntime = (next: string) => {
    setRuntime(next)
    if (next) localStorage.setItem(NEW_SESSION_AGENT_KEY, next)
    else localStorage.removeItem(NEW_SESSION_AGENT_KEY)
  }

  useEffect(() => {
    if (!runtimeReady || agents.includes(runtime)) return
    const next = agents[0] ?? ''
    if (next === runtime) return
    setRuntime(next)
    localStorage.removeItem(NEW_SESSION_AGENT_KEY)
  }, [agents, runtime, runtimeReady])

  const model = runtimeModelState(agentSettings, runtime)
  const { usesProvider, provider, selectedProvider } = model
  const agentOptions = agentSettings?.acp_options?.[runtime]
  const selectionKey = `${runtime}/${provider}`
  const selection = selections[selectionKey]
  const selectedModel = selection?.model ?? model.defaultModel
  const requestedEffort = selection?.effort ?? model.defaultEffort

  const {
    modelSuggestions,
    modelsLoading,
    reasoningOptions: effortOptions,
    effectiveReasoningEffort: effort,
    reasoningStatus,
    reasoningBlocked,
  } = useModelReasoningState({
    settings: agentSettings,
    agent: runtime,
    model: selectedModel,
    reasoningEffort: requestedEffort,
    usesProvider,
    provider,
    selectedProvider,
  })

  const composer = composerConfig()
  const fastModeConfigID = selectedModel && (!usesProvider || provider === agentOptions?.default_model_provider)
    && agentOptions?.fast_mode_models?.includes(modelSuggestionFor(modelSuggestions, selectedModel)?.value ?? selectedModel)
    ? agentOptions.fast_mode_config_id : undefined
  const setSelection = (next: ModelSelection) => {
    const updated = { ...selections, [selectionKey]: { ...selection, ...next } }
    setSelections(updated)
    localStorage.setItem(NEW_SESSION_MODELS_KEY, JSON.stringify(updated))
  }
  const fastMode = fastModeConfigID ? {
    checked: selection?.fastMode ?? false,
    onChange: (checked: boolean) => setSelection({ model: selectedModel, effort, fastMode: checked }),
  } : undefined

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
    modelsLoading,
    reasoningStatus,
    reasoningBlocked,
    // ready means a start now uses the settled agent and model, or reports why it cannot.
    ready: runtimeReady && (!runtimeAvailable || agents.includes(runtime)) && !(reasoningBlocked && reasoningStatus === 'pending'),
    setSelection,
    fastMode,
    effort,
    effortOptions,
    // The launched config IS the resolved config shown in the UI — same model,
    // provider, and clamped effort — so display and launch cannot diverge.
    sessionConfig: (extra: { directory: string; worktree: boolean; canvas?: boolean }, title?: string): CreateSessionInput => ({
      ...(title ? { title } : {}),
      runtime: 'acp',
      agent: runtime,
      directory: extra.directory,
      worktree: extra.worktree,
      ...(extra.canvas ? { artifact_surface: 'canvas' as const } : {}),
      ...(usesProvider && provider ? { model_provider: provider } : {}),
      ...(selectedModel ? { model: selectedModel } : {}),
      ...(effort ? { reasoning_effort: effort } : {}),
      ...(fastModeConfigID && selection?.fastMode !== undefined
        ? { config_options: { [fastModeConfigID]: selection.fastMode ? 'on' : 'off' } } : {}),
    }),
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
        <>
          <ModelSelect
            key={controls.runtime}
            value={controls.model}
            suggestions={controls.modelSuggestions}
            loading={controls.modelsLoading}
            placement={placement}
            disabled={disabled}
            onChange={controls.setSelection}
            effort={controls.effort}
            fastMode={controls.fastMode ? { ...controls.fastMode, disabled } : undefined}
          />
          <ReasoningSelect
            value={controls.effort}
            options={pickerEffortOptions(controls.effortOptions)}
            status={controls.runtime === 'opencode' ? controls.reasoningStatus : undefined}
            disabled={disabled}
            onChange={(effort) => controls.setSelection({ model: controls.model, effort })}
          />
        </>
      ) : null}
    </>
  )
}
