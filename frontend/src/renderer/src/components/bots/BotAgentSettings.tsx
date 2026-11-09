import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { ReasoningSelect } from '@/components/session/ReasoningSelect'
import { Select } from '@/components/ui/Select'
import { agentLabel } from '@/lib/agentLabel'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { AgentSessionState, Bot } from '@/lib/api/types'
import { useModelReasoningState } from '@/lib/modelReasoning'
import { pickerEffortOptions } from '@/lib/modelPicker'
import { useUpdateBot } from './useUpdateBot'

// The bot's agent and model in the composer's picker. A running agent's own
// options are the truth for the bot; before it starts, the bot's stored pick
// is, and then the agent's default. One save at a time, so an agent switch and
// a model change cannot race.
export function BotAgentSettings({ bot, agentSession, working }: { bot: Bot; agentSession?: AgentSessionState; working: boolean }) {
  const settings = useQuery(agentSettingsQuery).data
  const update = useUpdateBot(bot.id)
  const disabled = working || update.isPending
  const agent = bot.agent ?? ''
  const live = (category: string) => agentSession?.config_options?.find((option) => option.category === category)?.current_value
  const runtime = runtimeModelState(settings, agent)
  const model = live('model') || bot.model || runtime.defaultModel
  const reasoning = useModelReasoningState({
    settings,
    agent,
    model,
    reasoningEffort: bot.reasoning_effort || live('thought_level') || runtime.defaultEffort,
    usesProvider: runtime.usesProvider,
    provider: runtime.provider,
    selectedProvider: runtime.selectedProvider,
    sessionOptions: agentSession?.config_options,
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
            update.mutate({ agent: next })
          }}
        />
      </Row>
      <Row label="Model">
        <div className="-mr-2.5 flex min-w-0 flex-1 justify-end">
          <ModelSelect
            key={agent}
            value={model}
            effort={reasoning.effectiveReasoningEffort}
            suggestions={reasoning.modelSuggestions}
            loading={reasoning.modelsLoading}
            disabled={disabled}
            placement="below"
            align="end"
            onChange={(next) => update.mutate({ model: next.model, reasoning_effort: next.effort })}
          />
        </div>
      </Row>
      {reasoning.reasoningOptions.length > 1 || agent === 'opencode' ? (
        <Row label="Reasoning">
          <div className="-mr-2.5 flex min-w-0 flex-1 justify-end">
            <ReasoningSelect
              labeled={false}
              value={reasoning.effectiveReasoningEffort}
              options={agent === 'opencode' ? reasoning.reasoningOptions : pickerEffortOptions(reasoning.reasoningOptions)}
              status={agent === 'opencode' ? reasoning.reasoningStatus : undefined}
              disabled={disabled}
              onChange={(reasoning_effort) => update.mutate({ model: bot.model || model, reasoning_effort })}
            />
          </div>
        </Row>
      ) : null}
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
