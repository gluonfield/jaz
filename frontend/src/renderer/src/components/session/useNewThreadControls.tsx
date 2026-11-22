import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import { agentLabel } from '@/lib/agentLabel'
import { cloudModelProfilesQuery, cloudProfileToAgentOption } from '@/lib/api/cloudThreads'
import type { CreateSessionInput } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { useHasCloud } from '@/lib/cloudSession'
import { useIsCloud } from '@/lib/connection'
import { composerConfig } from '@/lib/jazDefaults'
import { useModelReasoningState } from '@/lib/modelReasoning'
import {
  parseModelSelections,
  pickerEffortOptions,
  type ModelSelection,
  type PickerAgentOption,
} from '@/lib/modelPicker'
import { modelSuggestionFor } from '@/lib/modelSuggestion'
import {
  NEW_SESSION_AGENT_KEY,
  NEW_SESSION_CLOUD_PROFILE_KEY,
  NEW_SESSION_MODELS_KEY,
} from '@/lib/newSessionConfig'

export function useNewThreadControls(targetLocation?: 'local' | 'cloud') {
  const isPrimaryCloud = useIsCloud()
  const hasCloud = useHasCloud()
  const usingCloudProfiles = targetLocation ? targetLocation === 'cloud' : isPrimaryCloud
  const settingsQuery = useQuery({ ...agentSettingsQuery, enabled: !usingCloudProfiles && !isPrimaryCloud })
  const cloudProfilesQuery = useQuery({ ...cloudModelProfilesQuery, enabled: usingCloudProfiles && hasCloud })
  const agentSettings = usingCloudProfiles ? undefined : settingsQuery.data
  const cloudProfiles = usingCloudProfiles ? (cloudProfilesQuery.data ?? []) : []

  const agentItems: PickerAgentOption[] = useMemo(() => {
    if (usingCloudProfiles) {
      return cloudProfiles.map(cloudProfileToAgentOption)
    }
    return enabledACPAgents(agentSettings).map((id) => ({ id, location: 'local' as const }))
  }, [agentSettings, cloudProfiles, usingCloudProfiles])

  const agents = useMemo(() => agentItems.map((item) => item.id), [agentItems])
  const runtimeReady = usingCloudProfiles ? (!hasCloud || cloudProfilesQuery.isSuccess) : settingsQuery.isSuccess
  const runtimeAvailable = runtimeReady && agents.length > 0

  const [localRuntime, setLocalRuntime] = useState(() => localStorage.getItem(NEW_SESSION_AGENT_KEY) || '')
  const [cloudRuntime, setCloudRuntime] = useState(() => localStorage.getItem(NEW_SESSION_CLOUD_PROFILE_KEY) || '')
  const [selections, setSelections] = useState(() => parseModelSelections(localStorage.getItem(NEW_SESSION_MODELS_KEY)))

  const runtime = usingCloudProfiles ? cloudRuntime : localRuntime
  const selectRuntime = (next: string) => {
    if (usingCloudProfiles) {
      setCloudRuntime(next)
      if (next) localStorage.setItem(NEW_SESSION_CLOUD_PROFILE_KEY, next)
      else localStorage.removeItem(NEW_SESSION_CLOUD_PROFILE_KEY)
      return
    }
    setLocalRuntime(next)
    if (next) localStorage.setItem(NEW_SESSION_AGENT_KEY, next)
    else localStorage.removeItem(NEW_SESSION_AGENT_KEY)
  }

  useEffect(() => {
    if (!runtimeReady || agents.includes(runtime)) return
    const next = agents[0] ?? ''
    if (next === runtime) return
    if (usingCloudProfiles) {
      setCloudRuntime(next)
      localStorage.removeItem(NEW_SESSION_CLOUD_PROFILE_KEY)
    } else {
      setLocalRuntime(next)
      localStorage.removeItem(NEW_SESSION_AGENT_KEY)
    }
  }, [agents, runtime, runtimeReady, usingCloudProfiles])

  const activeCloudProfile = usingCloudProfiles
    ? cloudProfiles.find((profile) => profile.profile_id === runtime) ?? cloudProfiles[0]
    : undefined

  const activeAgentLabel = agentItems.find((item) => item.id === runtime)?.label || agentLabel(runtime)

  const model = runtimeModelState(agentSettings, runtime)
  const { usesProvider, provider } = model
  const agentOptions = agentSettings?.acp_options?.[runtime]
  const selectionKey = usingCloudProfiles
    ? `cloud:${activeCloudProfile?.profile_id ?? runtime}`
    : `${runtime}/${provider}`
  const selection = selections[selectionKey]
  const selectedModel = selection?.model
    ?? (usingCloudProfiles
      ? activeCloudProfile?.default_model || activeCloudProfile?.models?.[0]?.value || ''
      : model.defaultModel)
  const requestedEffort = selection?.effort
    ?? (usingCloudProfiles ? activeCloudProfile?.default_effort || '' : model.defaultEffort)

  const localReasoning = useModelReasoningState({
    settings: usingCloudProfiles ? undefined : agentSettings,
    agent: usingCloudProfiles ? '' : runtime,
    model: selectedModel,
    reasoningEffort: requestedEffort,
    provider,
  })

  const cloudModels = activeCloudProfile?.models ?? []
  const cloudSelectedSuggestion = modelSuggestionFor(cloudModels, selectedModel)
  const cloudEffortOptions = (cloudSelectedSuggestion?.reasoning?.efforts ?? []).map((value) => ({
    value,
    label: value.charAt(0).toUpperCase() + value.slice(1),
  }))
  const cloudEffort = cloudEffortOptions.some((o) => o.value === requestedEffort)
    ? requestedEffort
    : cloudSelectedSuggestion?.reasoning?.default_effort ?? cloudEffortOptions[0]?.value ?? ''

  const modelSuggestions = usingCloudProfiles ? cloudModels : localReasoning.modelSuggestions
  const modelsLoading = usingCloudProfiles ? cloudProfilesQuery.isPending : localReasoning.modelsLoading
  const effortOptions = usingCloudProfiles ? cloudEffortOptions : localReasoning.reasoningOptions
  const effort = usingCloudProfiles ? cloudEffort : localReasoning.effectiveReasoningEffort
  const reasoningStatus = usingCloudProfiles ? 'ready' as const : localReasoning.reasoningStatus
  const reasoningBlocked = usingCloudProfiles ? false : localReasoning.reasoningBlocked

  const composer = composerConfig()
  const fastModeConfigID = !usingCloudProfiles
    && selectedModel
    && (!usesProvider || provider === agentOptions?.default_model_provider)
    && agentOptions?.fast_mode_models?.includes(modelSuggestionFor(modelSuggestions, selectedModel)?.value ?? selectedModel)
    ? agentOptions.fast_mode_config_id
    : undefined

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
    agentItems,
    activeAgentLabel,
    isCloudTarget: usingCloudProfiles,
    runtimeReady,
    runtimeAvailable,
    // Picker visibility lives here so the controls and the mobile summary agree.
    showAgentPicker: agents.length > 1 || usingCloudProfiles,
    showModelPicker: !composer.hideModelPicker,
    showProjectPicker: !composer.hideProjectPicker && !usingCloudProfiles,
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
      agent: usingCloudProfiles ? (activeCloudProfile?.agent || 'codex') : runtime,
      ...(usingCloudProfiles
        ? {}
        : {
            directory: extra.directory,
            worktree: extra.worktree,
            ...(extra.canvas ? { artifact_surface: 'canvas' as const } : {}),
          }),
      ...(usingCloudProfiles && activeCloudProfile
        ? {
            model_profile_id: activeCloudProfile.profile_id,
            model_provider: activeCloudProfile.provider,
          }
        : usesProvider && provider
          ? { model_provider: provider }
          : {}),
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
      <span className="px-1.5 text-[13px] text-ink-3">
        {controls.isCloudTarget ? 'No cloud models configured' : 'Connect an agent in Settings'}
      </span>
    ) : null
  }
  if (!controls.showModelPicker && !controls.showAgentPicker) {
    return null
  }
  const model = controls.showModelPicker
    ? {
        value: controls.model,
        suggestions: controls.modelSuggestions,
        loading: controls.modelsLoading,
        effort: controls.effort,
        effortOptions: pickerEffortOptions(controls.effortOptions),
        fastMode: controls.fastMode ? { ...controls.fastMode, disabled } : undefined,
      }
    : { value: '', suggestions: [], effort: '', effortOptions: [] }
  return (
    <ModelSelect
      {...model}
      agent={{
        value: controls.runtime,
        options: controls.agents,
        items: controls.agentItems,
        onChange: controls.selectRuntime,
      }}
      placement={placement}
      disabled={disabled}
      onChange={controls.setSelection}
    />
  )
}
