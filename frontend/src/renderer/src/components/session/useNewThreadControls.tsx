import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { acpAgentChecking, enabledACPAgents, enabledACPAgentsChecking, runtimeModelState } from '@/lib/agentRuntimes'
import { agentLabel } from '@/lib/agentLabel'
import { cloudProfileToAgentOption, resolveCloudModelSelection } from '@/lib/api/cloudThreads'
import { useCloudModelProfilesQuery } from '@/lib/hooks/useCloudCatalogQueries'
import type { CreateSessionInput } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { useCloudCatalogScope, useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
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
  NEW_SESSION_WORKSPACE_AGENT_KEY,
  NEW_SESSION_MODELS_KEY,
} from '@/lib/newSessionConfig'

export function useNewThreadControls(targetLocation?: 'local' | 'cloud') {
  const cloud = useCloudSession()
  const cloudOnly = useIsCloudOnly()
  const cloudTarget = cloudOnly || targetLocation === 'cloud'
  const mixed = !cloudTarget && cloud.active
  const { workspaceID } = useCloudCatalogScope()
  const localQuery = useQuery({ ...agentSettingsQuery, enabled: !cloudTarget })
  const cloudQuery = useQuery({ ...useCloudModelProfilesQuery(), enabled: cloud.active })
  const agentSettings = cloudTarget ? undefined : localQuery.data
  const cloudProfiles = cloud.active ? (cloudQuery.data ?? []).filter((profile) =>
    cloudTarget || (profile.scope === 'user' && profile.profile_id === `acp:${profile.agent}`),
  ) : []

  const runtimeKey = cloudTarget ? NEW_SESSION_CLOUD_PROFILE_KEY : mixed ? NEW_SESSION_WORKSPACE_AGENT_KEY : NEW_SESSION_AGENT_KEY
  const [runtimes, setRuntimes] = useState(() => Object.fromEntries(
    [NEW_SESSION_AGENT_KEY, NEW_SESSION_CLOUD_PROFILE_KEY, NEW_SESSION_WORKSPACE_AGENT_KEY]
      .map((key) => [key, localStorage.getItem(key) ?? '']),
  ))
  const [selections, setSelections] = useState(() => parseModelSelections(localStorage.getItem(NEW_SESSION_MODELS_KEY)))

  const preferred = runtimes[runtimeKey] || (mixed ? runtimes[NEW_SESSION_AGENT_KEY] || runtimes[NEW_SESSION_CLOUD_PROFILE_KEY] : '')
  const localAgents = enabledACPAgents(agentSettings)
  const savedRuntimeChecking = Boolean(agentSettings?.acp[preferred]?.enabled && acpAgentChecking(agentSettings, preferred))
  if (savedRuntimeChecking && !localAgents.includes(preferred)) localAgents.push(preferred)
  const agentItems: PickerAgentOption[] = [
    ...localAgents.map((id) => mixed ? { id, label: `${agentLabel(id)} · Local`, source: 'local' as const } : { id }),
    ...cloudProfiles.map((profile) => {
      const item = cloudProfileToAgentOption(profile)
      return mixed ? { ...item, label: `${item.label} · Cloud`, source: 'cloud' as const } : item
    }),
  ]
  const agents = agentItems.map((item) => item.id)
  const runtime = agents.includes(preferred) ? preferred : agents[0] ?? ''
  const runtimeChecking = enabledACPAgentsChecking(agentSettings)
  const runtimeReady = (cloudTarget || !localQuery.isPending) && (!cloud.active || !cloudQuery.isPending)
  const runtimeAvailable = Boolean(runtime) && (runtimeReady || runtime === preferred)
  const runtimeError = !cloudTarget && localQuery.isError ? localQuery.error : cloud.active && cloudQuery.isError ? cloudQuery.error : null

  const selectRuntime = (next: string) => {
    setRuntimes((current) => ({ ...current, [runtimeKey]: next }))
    localStorage.setItem(runtimeKey, next)
  }

  const activeCloudProfile = cloudProfiles.find((profile) => profile.profile_id === runtime)
  const activeAgentLabel = agentItems.find((item) => item.id === runtime)?.label || agentLabel(runtime)
  const nativeAgent = activeCloudProfile ? '' : runtime
  const model = runtimeModelState(agentSettings, nativeAgent)
  const { usesProvider, provider } = model
  const agentOptions = agentSettings?.acp_options?.[nativeAgent]
  const selectionKey = activeCloudProfile
    ? `cloud:${cloud.activeWorkspaceId || cloud.url}:${activeCloudProfile.profile_id}`
    : `${nativeAgent}/${provider}`
  const selection = selections[selectionKey]
  const cloudSelection = activeCloudProfile ? resolveCloudModelSelection([activeCloudProfile], selection) : undefined
  const selectedModel = cloudSelection?.model ?? selection?.model ?? model.defaultModel

  const localReasoning = useModelReasoningState({
    settings: agentSettings,
    agent: nativeAgent,
    model: selectedModel,
    reasoningEffort: selection?.effort ?? model.defaultEffort,
    provider,
  })

  const modelSuggestions = cloudSelection?.modelSuggestions ?? localReasoning.modelSuggestions
  const modelsLoading = !cloudSelection && (savedRuntimeChecking || localReasoning.modelsLoading)
  const effortOptions = cloudSelection?.effortOptions ?? localReasoning.reasoningOptions
  const effort = cloudSelection?.effort ?? localReasoning.effectiveReasoningEffort
  const reasoningStatus = cloudSelection ? 'ready' as const : savedRuntimeChecking ? 'pending' as const : localReasoning.reasoningStatus
  const reasoningBlocked = !cloudSelection && (savedRuntimeChecking || localReasoning.reasoningBlocked)

  const composer = composerConfig()
  const fastModeConfigID = !activeCloudProfile
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
    agents,
    agentItems,
    activeAgentLabel,
    isCloudTarget: cloudTarget,
    runtimeReady,
    runtimeAvailable,
    runtimeChecking,
    runtimeError,
    retryRuntime: () => {
      if (!cloudTarget && localQuery.isError) void localQuery.refetch()
      if (cloud.active && cloudQuery.isError) void cloudQuery.refetch()
    },
    // Picker visibility lives here so the controls and the mobile summary agree.
    showAgentPicker: agents.length > 1,
    showModelPicker: !composer.hideModelPicker,
    showProjectPicker: !composer.hideProjectPicker && !cloudTarget,
    runtime,
    selectRuntime,
    model: selectedModel,
    modelSuggestions,
    modelsLoading,
    reasoningStatus,
    reasoningBlocked,
    // ready means a start now uses the settled agent and model, or reports why it cannot.
    ready: runtimeReady && !(reasoningBlocked && reasoningStatus === 'pending'),
    setSelection,
    fastMode,
    effort,
    effortOptions,
    // The launched config IS the resolved config shown in the UI — same model,
    // provider, and clamped effort — so display and launch cannot diverge.
    sessionConfig: (extra: { directory: string; worktree: boolean; canvas?: boolean }, title?: string): CreateSessionInput => ({
      ...(title ? { title } : {}),
      runtime: 'acp',
      agent: activeCloudProfile?.agent || runtime,
      ...(cloudTarget
        ? { backend: 'cloud' as const }
        : {
            directory: extra.directory,
            worktree: extra.worktree,
            ...(extra.canvas ? { artifact_surface: 'canvas' as const } : {}),
          }),
      ...(workspaceID ? { cloud_workspace_id: workspaceID } : {}),
      ...(cloudTarget && activeCloudProfile
        ? {
            model_profile_id: activeCloudProfile.profile_id,
            model_provider: activeCloudProfile.provider,
          }
        : activeCloudProfile
          ? { cloud_credential_profile_id: activeCloudProfile.profile_id }
          : usesProvider && provider ? { model_provider: provider } : {}),
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
    if (controls.runtimeError) {
      const detail = controls.runtimeError instanceof Error ? controls.runtimeError.message : String(controls.runtimeError)
      return (
        <span className="inline-flex items-center gap-1.5 px-1.5 text-[13px] text-danger" title={detail}>
          <span>{controls.isCloudTarget ? "Couldn't load Cloud models" : "Couldn't load agents"}</span>
          <button
            type="button"
            onClick={controls.retryRuntime}
            className="cursor-pointer underline decoration-current/50 underline-offset-2 hover:text-ink"
          >
            Retry
          </button>
        </span>
      )
    }
    return controls.runtimeReady ? (
      <span className="px-1.5 text-[13px] text-ink-3">
        {controls.runtimeChecking
          ? 'Checking agents…'
          : controls.isCloudTarget
            ? 'No cloud models configured'
            : 'Connect an agent in Settings'}
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
