import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Save, Terminal } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { REASONING_EFFORT_OPTIONS } from '@/components/loops/ReasoningEffortSelect'
import { Button } from '@/components/ui/Button'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { agentSettingsQuery, updateAgentSettings } from '@/lib/api/settings'
import type { AgentSettings as AgentSettingsData } from '@/lib/api/types'
import { acpAgentModelSuggestions, OPENAI_MODELS, openRouterModelsQuery } from '@/lib/models'
import { keys } from '@/lib/query/keys'

const inputClass =
  'h-7 w-full rounded-full bg-ink/10 px-3 text-[12px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25 disabled:opacity-50'

const rowControlClass = 'w-full md:w-[320px]'

// Same efforts everywhere; here '' means "no effort configured" rather than
// "inherit the default", hence the relabel.
const reasoningOptions = REASONING_EFFORT_OPTIONS.map((option) =>
  option.value === '' ? { ...option, label: 'None' } : option,
)

function cloneSettings(settings: AgentSettingsData): AgentSettingsData {
  return {
    native: { ...settings.native },
    providers: [...(settings.providers ?? [])],
    acp: Object.fromEntries(
      Object.entries(settings.acp).map(([agent, value]) => [
        agent,
        { ...value },
      ]),
    ),
    agents: [...settings.agents],
  }
}

function settingsKey(settings: AgentSettingsData | null): string {
  return settings ? JSON.stringify(settings) : ''
}

function hasEnabledACPWithoutCommand(settings: AgentSettingsData): boolean {
  return settings.agents.some((agent) => {
    const current = settings.acp[agent]
    return Boolean(current?.enabled) && (current.command ?? '').trim() === ''
  })
}

export function AgentSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const settings = useQuery(agentSettingsQuery)
  const [draft, setDraft] = useState<AgentSettingsData | null>(null)

  useEffect(() => {
    if (settings.data) setDraft(cloneSettings(settings.data))
  }, [settings.data])

  const save = useMutation({
    mutationFn: (input: AgentSettingsData) => updateAgentSettings(input),
    onSuccess: (saved) => {
      setDraft(cloneSettings(saved))
      toast('Saved agent settings')
    },
    onError: (error: Error) => toast(`Couldn't save agent settings: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  const dirty = useMemo(
    () => settingsKey(draft) !== settingsKey(settings.data ?? null),
    [draft, settings.data],
  )
  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: draft?.native.model_provider === 'openrouter',
  })
  const nativeModelSuggestions =
    draft?.native.model_provider === 'openrouter' ? (openRouterModels.data ?? []) : OPENAI_MODELS
  const invalid = draft
    ? (draft.native.model_provider ?? '').trim() === '' ||
      draft.native.model.trim() === '' ||
      hasEnabledACPWithoutCommand(draft)
    : true
  const canSave = draft != null && !invalid && dirty && !save.isPending

  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Agents</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Defaults copied into each new thread.
          </p>
        </div>
        <Button
          variant="primary"
          size="md"
          disabled={!canSave}
          onClick={() => draft && save.mutate(draft)}
        >
          <Save size={14} />
          {save.isPending ? 'Saving...' : 'Save changes'}
        </Button>
      </div>

      <div className="mt-4">
        {settings.isError ? (
          <p className="py-2 text-[13px] text-danger">{settings.error.message}</p>
        ) : settings.isPending || !draft ? (
          <SkeletonRows count={3} />
        ) : (
          <div className="flex flex-col gap-4">
            <div>
              <p className="pb-2 text-[12px] font-medium text-ink-2">Native</p>
              <div className="overflow-hidden rounded-card bg-surface">
                <SettingsRow
                  title="Provider"
                  description="API provider used for native threads."
                >
                  <Select
                    value={draft.native.model_provider ?? ''}
                    options={(draft.providers ?? []).map((provider) => ({
                      value: provider.id,
                      label: provider.label,
                      description: provider.base_url,
                    }))}
                    disabled={save.isPending}
                    onChange={(model_provider) => {
                      const nextProvider = draft.providers.find((provider) => provider.id === model_provider)
                      const currentProvider = draft.providers.find(
                        (provider) => provider.id === draft.native.model_provider,
                      )
                      const model =
                        draft.native.model.trim() === '' ||
                        draft.native.model === currentProvider?.default_model
                          ? nextProvider?.default_model || draft.native.model
                          : draft.native.model
                      setDraft({
                        ...draft,
                        native: {
                          ...draft.native,
                          model_provider,
                          model,
                        },
                      })
                    }}
                    aria-label="Native provider"
                    className={rowControlClass}
                  />
                </SettingsRow>
                <SettingsRow title="Model" description="Default model for native threads.">
                  <ModelCombobox
                    value={draft.native.model}
                    suggestions={nativeModelSuggestions}
                    loading={openRouterModels.isLoading}
                    disabled={save.isPending}
                    onChange={(model) =>
                      setDraft({
                        ...draft,
                        native: { ...draft.native, model },
                      })
                    }
                    aria-label="Native model"
                    className={rowControlClass}
                  />
                </SettingsRow>
                <SettingsRow
                  title="Reasoning"
                  description="Default reasoning effort for native threads."
                >
                  <Select
                    value={draft.native.reasoning_effort ?? ''}
                    options={reasoningOptions}
                    disabled={save.isPending}
                    onChange={(reasoning_effort) =>
                      setDraft({ ...draft, native: { ...draft.native, reasoning_effort } })
                    }
                    aria-label="Native reasoning effort"
                    className={rowControlClass}
                  />
                </SettingsRow>
              </div>
            </div>

            <div>
              <p className="pb-2 text-[12px] font-medium text-ink-2">ACP</p>
              <div className="flex flex-col gap-3">
                {draft.agents.map((agent) => (
                  <ACPAgentRow
                    key={agent}
                    agent={agent}
                    settings={draft}
                    disabled={save.isPending}
                    onChange={setDraft}
                  />
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </section>
  )
}

function ACPAgentRow({
  agent,
  settings,
  disabled,
  onChange,
}: {
  agent: string
  settings: AgentSettingsData
  disabled: boolean
  onChange: (settings: AgentSettingsData) => void
}) {
  const current = settings.acp[agent] ?? {
    enabled: false,
    command: '',
    model: '',
    reasoning_effort: '',
  }
  const [commandOpen, setCommandOpen] = useState((current.command ?? '').trim() === '')
  useEffect(() => {
    if (current.enabled && (current.command ?? '').trim() === '') setCommandOpen(true)
  }, [current.command, current.enabled])
  const controlsDisabled = disabled || !current.enabled
  const update = (next: Partial<typeof current>) =>
    onChange({
      ...settings,
      acp: {
        ...settings.acp,
        [agent]: { ...current, ...next },
      },
    })

  return (
    <div className="overflow-hidden rounded-card bg-surface">
      <div className="flex items-center gap-2 px-3 py-3">
        <span className="min-w-0 truncate text-[13px] font-medium text-ink" title={agent}>
          {agentLabel(agent)}
        </span>
      </div>

      <SettingsRow title="Enabled" description="Show this ACP client as a runtime.">
        <div className="flex h-8 w-full items-center justify-start md:w-[320px] md:justify-end">
          <Switch
            checked={current.enabled}
            disabled={disabled}
            onChange={(enabled) => update({ enabled })}
            aria-label={`Enable ${agentLabel(agent)}`}
          />
        </div>
      </SettingsRow>

      <SettingsRow title="Model" description="Model copied into new threads for this client.">
        <ModelCombobox
          value={current.model ?? ''}
          suggestions={acpAgentModelSuggestions(agent)}
          disabled={controlsDisabled}
          onChange={(model) => update({ model })}
          aria-label={`${agentLabel(agent)} model`}
          className={rowControlClass}
        />
      </SettingsRow>

      <SettingsRow title="Reasoning" description="Reasoning effort copied into new threads.">
        <Select
          value={current.reasoning_effort ?? ''}
          options={reasoningOptions}
          disabled={controlsDisabled}
          onChange={(reasoning_effort) => update({ reasoning_effort })}
          aria-label={`${agentLabel(agent)} reasoning effort`}
          className={rowControlClass}
        />
      </SettingsRow>

      <SettingsRow title="Command" description="Advanced startup command for this ACP client.">
        <Button
          variant="ghost"
          size="md"
          active={commandOpen}
          aria-expanded={commandOpen}
          disabled={disabled}
          onClick={() => setCommandOpen((open) => !open)}
          className="w-full md:w-auto md:justify-self-end"
        >
          <Terminal size={13} />
          {commandOpen ? 'Hide command' : 'Edit command'}
          <ChevronDown
            size={13}
            className={`transition-transform duration-150 ${commandOpen ? 'rotate-180' : ''}`}
          />
        </Button>
      </SettingsRow>

      {commandOpen ? (
        <label className="block border-t border-border/70 px-3 py-3">
          <span className="mb-1 block text-[11px] text-ink-3">Startup command</span>
          <input
            value={current.command ?? ''}
            disabled={controlsDisabled}
            onChange={(event) => update({ command: event.target.value })}
            className={`${inputClass} font-mono`}
          />
        </label>
      ) : null}
    </div>
  )
}

function SettingsRow({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-1 gap-2 border-t border-border/70 px-3 py-3 first:border-t-0 md:grid-cols-[minmax(0,1fr)_minmax(220px,320px)] md:items-center">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        <p className="mt-0.5 text-[12px] text-ink-3">{description}</p>
      </div>
      <div className="min-w-0 md:justify-self-end">{children}</div>
    </div>
  )
}
