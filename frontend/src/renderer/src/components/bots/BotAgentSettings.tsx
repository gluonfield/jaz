import { useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { Select } from '@/components/ui/Select'
import { agentLabel } from '@/lib/agentLabel'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { AgentSessionState, Bot } from '@/lib/api/types'
import { useModelReasoningState } from '@/lib/modelReasoning'
import { pickerEffortOptions } from '@/lib/modelPicker'
import { useUpdateBot } from './useUpdateBot'

// A saved pick has already been validated. The cached live controls may still
// describe the previous turn; they supply the available options, not an override
// of a newly acknowledged selection.
export function BotAgentSettings({ bot, agentSession, working }: { bot: Bot; agentSession?: AgentSessionState; working: boolean }) {
  const settings = useQuery(agentSettingsQuery).data
  const update = useUpdateBot(bot.id)
  const disabled = working || update.isPending
  const agent = bot.agent ?? ''
  const live = (category: string) => agentSession?.config_options?.find((option) => option.category === category)?.current_value
  const [pendingProvider, setPendingProvider] = useState<string>()
  const runtime = runtimeModelState(settings, agent, pendingProvider ?? bot.model_provider)
  const model = pendingProvider
    ? ''
    : bot.model || (!runtime.usesProvider ? live('model') : '') || runtime.defaultModel
  const reasoning = useModelReasoningState({
    settings,
    agent,
    model,
    reasoningEffort: live('thought_level') || bot.reasoning_effort || runtime.defaultEffort,
    usesProvider: runtime.usesProvider,
    provider: runtime.provider,
    selectedProvider: runtime.selectedProvider,
  })
  return (
    <div className="-mx-2.5 flex flex-col">
      <Row label="Agent">
        <Select
          aria-label="Agent"
          variant="plain"
          value={agent}
          options={agentOptions(enabledACPAgents(settings), agent)}
          disabled={disabled}
          onChange={(next) => {
            if (next === agent) return
            // Another agent keeps the bot, its chat and routines, but starts
            // with no memory of the conversation, so the move asks first.
            const label = agentLabel(next)
            if (!window.confirm(`Move ${bot.name} to ${label}? ${label} starts with a fresh memory; the chat and routines stay.`)) return
            setPendingProvider(undefined)
            update.mutate({ agent: next })
          }}
        />
      </Row>
      {runtime.usesProvider && runtime.providers.length > 1 && (
        <Row label="Provider">
          <Select
            aria-label="Provider"
            variant="plain"
            value={runtime.provider}
            options={runtime.providers.map(({ id, label }) => ({ value: id, label }))}
            disabled={disabled}
            onChange={(provider) => setPendingProvider(provider === bot.model_provider ? undefined : provider)}
          />
        </Row>
      )}
      <Row label="Model">
        <div className="-mr-2.5 flex min-w-0 flex-1 justify-end">
          <ModelSelect
            key={`${agent}:${runtime.provider}`}
            value={model}
            effort={reasoning.effectiveReasoningEffort}
            suggestions={reasoning.modelSuggestions}
            effortOptions={pickerEffortOptions(reasoning.reasoningOptions)}
            loading={reasoning.modelsLoading}
            disabled={disabled}
            placement="below"
            align="end"
            onChange={(next) => {
              if (!next.model) return
              update.mutate({
                model: next.model,
                model_provider: runtime.provider || undefined,
                reasoning_effort: next.effort,
              }, { onSuccess: () => setPendingProvider(undefined) })
            }}
          />
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

// The enabled agents, keeping one still in use after it was turned off.
function agentOptions(agents: string[], current: string) {
  return (!current || agents.includes(current) ? agents : [current, ...agents]).map((agent) => ({ value: agent, label: agentLabel(agent) }))
}
