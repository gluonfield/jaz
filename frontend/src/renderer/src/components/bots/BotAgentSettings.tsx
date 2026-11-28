import { useQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { type ReactNode, useMemo, useState } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { Checkbox } from '@/components/ui/Checkbox'
import { Popover } from '@/components/ui/Popover'
import { Select } from '@/components/ui/Select'
import { agentLabel } from '@/lib/agentLabel'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import { resolveCloudModelSelection } from '@/lib/api/cloudThreads'
import { useCloudModelProfilesQuery } from '@/lib/hooks/useCloudCatalogQueries'
import { pluginsQuery } from '@/lib/api/plugins'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { AgentSessionState, Bot } from '@/lib/api/types'
import { useCloudSession, useIsCloudOnly, useIsCloudViewer } from '@/lib/cloudSession'
import { useModelReasoningState } from '@/lib/modelReasoning'
import { pickerEffortOptions } from '@/lib/modelPicker'
import { useUpdateBot } from './useUpdateBot'

// The bot's agent, model, and plugin allowlist. A running agent's own options
// are the truth for the bot; before it starts, the bot's stored pick is, and
// then the agent's default.
export function BotAgentSettings({ bot, agentSession, working }: { bot: Bot; agentSession?: AgentSessionState; working: boolean }) {
  const cloud = useCloudSession()
  const cloudOnly = useIsCloudOnly()
  const isViewer = useIsCloudViewer()
  const usingCloudProfiles = cloudOnly || cloud.active
  const settings = useQuery({ ...agentSettingsQuery, enabled: !usingCloudProfiles }).data
  const cloudProfilesQuery = useQuery({ ...useCloudModelProfilesQuery(), enabled: usingCloudProfiles && cloud.active })
  const cloudProfiles = useMemo(
    () => (usingCloudProfiles ? (cloudProfilesQuery.data ?? []) : []),
    [cloudProfilesQuery.data, usingCloudProfiles],
  )
  const installedPlugins = (useQuery(pluginsQuery).data ?? []).filter((plugin) => plugin.installed && plugin.enabled)
  const update = useUpdateBot(bot.id)
  const [pluginsOpen, setPluginsOpen] = useState(false)
  const disabled = working || update.isPending || isViewer
  const agent = bot.agent ?? ''
  const liveModel = agentSession?.config_options?.find((option) => option.category === 'model')?.current_value
  const liveEffort = agentSession?.config_options?.find((option) => option.category === 'thought_level')?.current_value
  const runtime = runtimeModelState(settings, agent)
  const localModel = liveModel || bot.model || runtime.defaultModel
  const localReasoning = useModelReasoningState({
    settings: usingCloudProfiles ? undefined : settings,
    agent: usingCloudProfiles ? '' : agent,
    model: localModel,
    reasoningEffort: liveEffort || bot.reasoning_effort || runtime.defaultEffort,
    provider: runtime.provider,
  })
  const cloudSelection = useMemo(
    () =>
      usingCloudProfiles
        ? resolveCloudModelSelection(cloudProfiles, {
            profileId: agent,
            agent,
            model: liveModel || bot.model,
            effort: liveEffort || bot.reasoning_effort,
          })
        : null,
    [agent, bot.model, bot.reasoning_effort, cloudProfiles, liveEffort, liveModel, usingCloudProfiles],
  )
  const model = usingCloudProfiles ? (cloudSelection?.model ?? '') : localModel
  const modelSuggestions = usingCloudProfiles ? (cloudSelection?.modelSuggestions ?? []) : localReasoning.modelSuggestions
  const effort = usingCloudProfiles ? (cloudSelection?.effort ?? '') : localReasoning.effectiveReasoningEffort
  const effortOptions = usingCloudProfiles ? (cloudSelection?.effortOptions ?? []) : localReasoning.reasoningOptions
  const modelsLoading = usingCloudProfiles ? cloudProfilesQuery.isPending : localReasoning.modelsLoading
  const selectValue = usingCloudProfiles ? (cloudSelection?.profileId || agent) : agent
  const selectOptions = usingCloudProfiles
    ? cloudAgentOptions(cloudProfiles, agent)
    : agentOptions(enabledACPAgents(settings), agent)
  const inheritsAll = bot.plugins === undefined
  const selectedPlugins = new Set(bot.plugins ?? [])
  const pluginsLabel = inheritsAll
    ? 'All enabled'
    : selectedPlugins.size === 0
      ? 'None'
      : `${selectedPlugins.size} selected`

  const togglePlugin = (pluginID: string, checked: boolean) => {
    const next = new Set(inheritsAll ? installedPlugins.map((plugin) => plugin.id) : selectedPlugins)
    if (checked) next.add(pluginID)
    else next.delete(pluginID)
    update.mutate({ plugins: Array.from(next).sort() })
  }

  return (
    <div className="-mx-2.5 flex flex-col">
      <Row label="Agent">
        <Select
          aria-label="Agent"
          variant="plain"
          value={selectValue}
          options={selectOptions}
          disabled={disabled || selectOptions.length === 0}
          onChange={(next) => {
            if (next === selectValue) return
            if (usingCloudProfiles) {
              const picked = resolveCloudModelSelection(cloudProfiles, { profileId: next })
              const label = picked.profile?.display_name || agentLabel(next)
              if (!window.confirm(`Move ${bot.name} to ${label}? ${label} starts with a fresh memory; the chat and routines stay.`)) return
              update.mutate({
                agent: picked.profile?.agent || next,
                model: picked.model,
                reasoning_effort: picked.effort,
              })
              return
            }
            const label = agentLabel(next)
            if (!window.confirm(`Move ${bot.name} to ${label}? ${label} starts with a fresh memory; the chat and routines stay.`)) return
            update.mutate({ agent: next })
          }}
        />
      </Row>
      <Row label="Model">
        <div className="-mr-2.5 flex min-w-0 flex-1 justify-end">
          <ModelSelect
            key={selectValue}
            value={model}
            effort={effort}
            suggestions={modelSuggestions}
            effortOptions={pickerEffortOptions(effortOptions)}
            loading={modelsLoading}
            disabled={disabled}
            placement="below"
            align="end"
            onChange={(next) => update.mutate({ model: next.model, reasoning_effort: next.effort })}
          />
        </div>
      </Row>
      <Row label="Plugins">
        <div className="flex min-w-0 flex-1 justify-end">
          <Popover
            open={pluginsOpen}
            onClose={() => setPluginsOpen(false)}
            placement="below"
            align="end"
            trigger={
              <button
                type="button"
                disabled={disabled}
                onClick={() => setPluginsOpen((value) => !value)}
                className="inline-flex h-7 cursor-pointer items-center gap-1 rounded-md px-2 text-[13px] text-ink-2 transition-colors hover:bg-surface-2 hover:text-ink disabled:cursor-default disabled:opacity-50"
              >
                <span>{pluginsLabel}</span>
                <ChevronDown size={13} aria-hidden />
              </button>
            }
          >
            <div className="w-60 space-y-1 p-2">
              <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[12.5px] font-medium text-ink hover:bg-list-hover">
                <Checkbox
                  checked={inheritsAll}
                  onChange={(checked) => {
                    if (checked) {
                      update.mutate({ inherit_plugins: true })
                    } else {
                      update.mutate({ plugins: installedPlugins.map((plugin) => plugin.id) })
                    }
                  }}
                />
                <span>All enabled plugins</span>
              </label>
              {installedPlugins.length > 0 ? (
                <div className="max-h-56 space-y-0.5 overflow-y-auto border-t border-border/60 pt-1">
                  {installedPlugins.map((plugin) => {
                    const checked = inheritsAll || selectedPlugins.has(plugin.id)
                    return (
                      <label
                        key={plugin.id}
                        className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[12.5px] text-ink-2 hover:bg-list-hover hover:text-ink"
                      >
                        <Checkbox
                          checked={checked}
                          onChange={(next) => togglePlugin(plugin.id, next)}
                        />
                        <span className="truncate">{plugin.display_name}</span>
                      </label>
                    )
                  })}
                </div>
              ) : null}
            </div>
          </Popover>
        </div>
      </Row>
    </div>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex h-9 items-center gap-3 rounded-lg px-2.5 transition-colors duration-150 hover:bg-list-hover">
      <span className="shrink-0 text-[13px] text-ink">{label}</span>
      {children}
    </div>
  )
}

function agentOptions(agents: string[], current: string) {
  return (!current || agents.includes(current) ? agents : [current, ...agents]).map((agent) => ({ value: agent, label: agentLabel(agent) }))
}

function cloudAgentOptions(profiles: { profile_id: string; display_name?: string; provider: string; agent?: string }[], current: string) {
  const options = profiles.map((profile) => ({
    value: profile.profile_id,
    label: profile.display_name || profile.provider || profile.profile_id,
  }))
  if (
    current &&
    !profiles.some(
      (profile) =>
        profile.profile_id === current ||
        profile.provider === current.toLowerCase() ||
        (profile.agent ?? '').toLowerCase() === current.toLowerCase(),
    )
  ) {
    return [{ value: current, label: agentLabel(current) }, ...options]
  }
  return options
}
