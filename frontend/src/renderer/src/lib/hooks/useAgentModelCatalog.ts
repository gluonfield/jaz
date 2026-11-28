import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { agentLabel } from '@/lib/agentLabel'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import { resolveCloudModelSelection } from '@/lib/api/cloudThreads'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { ReasoningEffortOption } from '@/lib/api/types'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { useCloudModelProfilesQuery } from '@/lib/hooks/useCloudCatalogQueries'
import type { ModelPickerOption, PickerAgentOption } from '@/lib/modelPicker'
import { useModelReasoningState } from '@/lib/modelReasoning'

export interface AgentModelCatalogInput {
  enabled?: boolean
  agent?: string
  provider?: string
  model?: string
  effort?: string
}

export interface AgentSelectOption {
  value: string
  label: string
}

export interface PickedAgentSelection {
  agent: string
  provider: string
  label: string
  model: string
  effort: string
  resolvedAgent: string
  botPatch: { agent: string; model?: string; reasoning_effort?: string }
}

export interface AgentModelCatalogResult {
  usingCloud: boolean
  ready: boolean
  loading: boolean
  agents: string[]
  agentItems?: PickerAgentOption[]
  agentOptions: AgentSelectOption[]
  agent: string
  defaultAgent: string
  model: string
  modelSuggestions: ModelPickerOption[]
  effort: string
  requestedEffort: string
  effortOptions: ReasoningEffortOption[]
  pickAgent: (nextAgent: string) => PickedAgentSelection
}

// Single transport-neutral model catalog for Bot settings, Loop forms, and
// dialogs: resolves either Cloud model profiles or Local ACP agent catalogs
// behind one shape so picker UIs stay free of Cloud/Local branching.
export function useAgentModelCatalog(input: AgentModelCatalogInput = {}): AgentModelCatalogResult {
  const enabled = input.enabled ?? true
  const cloud = useCloudSession()
  const cloudOnly = useIsCloudOnly()
  const usingCloud = cloudOnly || cloud.active
  const settingsQuery = useQuery({ ...agentSettingsQuery, enabled: enabled && !usingCloud })
  const cloudProfilesQuery = useQuery({ ...useCloudModelProfilesQuery(), enabled: enabled && usingCloud && cloud.active })
  const settings = usingCloud ? undefined : settingsQuery.data
  const cloudProfiles = useMemo(
    () => (usingCloud ? (cloudProfilesQuery.data ?? []) : []),
    [cloudProfilesQuery.data, usingCloud],
  )
  const localAgents = useMemo(() => enabledACPAgents(settings), [settings])
  const cloudSelection = useMemo(
    () =>
      usingCloud
        ? resolveCloudModelSelection(cloudProfiles, {
            profileId: input.provider || input.agent,
            agent: input.agent,
            model: input.model,
            effort: input.effort,
          })
        : null,
    [cloudProfiles, input.agent, input.effort, input.model, input.provider, usingCloud],
  )
  const localRuntime = runtimeModelState(settings, input.agent ?? '', input.provider)
  const localModel = input.model || localRuntime.defaultModel
  const localRequestedEffort = input.effort || localRuntime.defaultEffort
  const localReasoning = useModelReasoningState({
    settings: usingCloud ? undefined : settings,
    agent: usingCloud ? '' : (input.agent ?? ''),
    model: localModel,
    reasoningEffort: localRequestedEffort,
    provider: localRuntime.provider,
  })

  const agents = usingCloud ? (cloudSelection?.agents ?? []) : localAgents
  const currentAgent = input.agent ?? ''
  const agent = usingCloud ? (cloudSelection?.profileId || currentAgent) : currentAgent
  const defaultAgent = usingCloud
    ? (cloudSelection?.profileId ?? '')
    : (localAgents.includes(currentAgent) ? currentAgent : (localAgents[0] ?? ''))
  const ready = usingCloud ? (!cloud.active || cloudProfilesQuery.isSuccess) : settingsQuery.isSuccess

  const agentOptions = useMemo<AgentSelectOption[]>(() => {
    if (usingCloud) {
      const options = cloudProfiles.map((profile) => ({
        value: profile.profile_id,
        label: profile.display_name || profile.provider || profile.profile_id,
      }))
      const lowered = currentAgent.toLowerCase()
      const known =
        !currentAgent ||
        cloudProfiles.some(
          (profile) =>
            profile.profile_id === currentAgent ||
            profile.provider === lowered ||
            (profile.agent ?? '').toLowerCase() === lowered,
        )
      return known ? options : [{ value: currentAgent, label: agentLabel(currentAgent) }, ...options]
    }
    const list = !currentAgent || localAgents.includes(currentAgent) ? localAgents : [currentAgent, ...localAgents]
    return list.map((id) => ({ value: id, label: agentLabel(id) }))
  }, [cloudProfiles, currentAgent, localAgents, usingCloud])

  const pickAgent = (nextAgent: string): PickedAgentSelection => {
    if (usingCloud) {
      const picked = resolveCloudModelSelection(cloudProfiles, { profileId: nextAgent })
      const resolvedAgent = picked.profile?.agent || nextAgent
      return {
        agent: nextAgent,
        provider: nextAgent,
        label: picked.profile?.display_name || agentLabel(nextAgent),
        model: picked.model,
        effort: picked.effort,
        resolvedAgent,
        botPatch: { agent: resolvedAgent, model: picked.model, reasoning_effort: picked.effort },
      }
    }
    return {
      agent: nextAgent,
      provider: '',
      label: agentLabel(nextAgent),
      model: '',
      effort: '',
      resolvedAgent: nextAgent,
      botPatch: { agent: nextAgent },
    }
  }

  return {
    usingCloud,
    ready,
    loading: usingCloud ? cloudProfilesQuery.isPending : localReasoning.modelsLoading,
    agents,
    agentItems: usingCloud ? cloudSelection?.agentItems : undefined,
    agentOptions,
    agent,
    defaultAgent,
    model: usingCloud ? (cloudSelection?.model ?? '') : localModel,
    modelSuggestions: usingCloud ? (cloudSelection?.modelSuggestions ?? []) : localReasoning.modelSuggestions,
    effort: usingCloud ? (cloudSelection?.effort ?? '') : localReasoning.effectiveReasoningEffort,
    requestedEffort: usingCloud ? (cloudSelection?.effort ?? '') : localRequestedEffort,
    effortOptions: usingCloud ? (cloudSelection?.effortOptions ?? []) : localReasoning.reasoningOptions,
    pickAgent,
  }
}
