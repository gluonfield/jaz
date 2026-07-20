import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, CircleAlert, KeyRound, LoaderCircle, LogIn } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { SettingsSection, useAgentSettingsDraft } from '@/components/settings/agentSettingsShell'
import { Button } from '@/components/ui/Button'
import { Collapse } from '@/components/ui/Collapse'
import { Input } from '@/components/ui/Input'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { Segmented } from '@/components/ui/Segmented'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentAPIKeyCopy, agentLabel, authProviderLabel } from '@/lib/agentLabel'
import {
  acpAgentEnableable,
  acpAgentEnabled,
  acpProviderUsesNativeAuth,
  acpUsesModelProvider,
  modelProviderConnected,
  modelProviderRequiresKey,
  normalizeACPAgentEnabled,
  normalizeACPAgentsEnabled,
  selectableACPModelProviders,
  selectedACPModelProvider,
} from '@/lib/agentRuntimes'
import { cloneAgentSettings, disconnectACPAuth, startACPAuthLogin } from '@/lib/api/settings'
import type {
  ACPAgentAuthStatus,
  ACPAuthLogin,
  AgentSettings as AgentSettingsData,
  ModelProviderOption,
} from '@/lib/api/types'
import { useACPLoginPolling } from '@/lib/hooks/useACPLoginPolling'
import { useModelReasoningState } from '@/lib/modelReasoning'
import { keys } from '@/lib/query/keys'

const rowControlClass = 'w-full md:w-[320px]'
type ACPAuthDraft = AgentSettingsData['acp'][string]['auth']
const emptyACPAgent: AgentSettingsData['acp'][string] = {
  enabled: false,
  model_provider: '',
  model: '',
  reasoning_effort: '',
  auth: { mode: 'auto', path: '' },
}

// Returns a clone with one ACP agent turned on — used when a sign-in or API key
// connects an agent, so it becomes usable without a separate toggle.
function withEnabledAgent(settings: AgentSettingsData, agent: string): AgentSettingsData {
  const next = cloneAgentSettings(settings)
  const current = next.acp[agent]
  if (current) next.acp[agent] = { ...current, enabled: true }
  return next
}

function withLoginAuth(settings: AgentSettingsData, agent: string): AgentSettingsData {
  if (agent === 'grok') return settings
  const next = cloneAgentSettings(settings)
  const current = next.acp[agent]
  if (current) next.acp[agent] = { ...current, auth: loginAuth(agent, current.auth) }
  return next
}

function loginAuth(agent: string, current: ACPAuthDraft): ACPAuthDraft {
  if (agent === 'antigravity') return { mode: 'existing_cli' }
  if (agent === 'grok' || current?.mode === 'jaz_profile') return current
  return { mode: 'jaz_profile', path: '' }
}

function hasInvalidACPProvider(settings: AgentSettingsData): boolean {
  return settings.agents.some((agent) => {
    if (!acpUsesModelProvider(settings, agent)) return false
    const current = settings.acp[agent]
    if (!acpAgentEnabled(settings, agent)) return false
    return (current?.model ?? '').trim() === ''
  })
}

export function ACPAgentsSettings({ onOpenProviders }: { onOpenProviders: () => void }) {
  const { settings, draft, setDraft, save, dirty } = useAgentSettingsDraft('agent settings')
  // This screen owns extra mutations (sign-in, disconnect) beyond the shared save.
  const queryClient = useQueryClient()
  const toast = useToast()

  // A finished sign-in connects the agent: turn it on and persist so it works
  // right away (matching onboarding, no extra Enabled toggle). The hook reads
  // this through a ref, so `draft`/`save` are always current here.
  const { loginJobs, trackLoginJob, forgetLoginJob } = useACPLoginPolling((job) => {
    if (job.status === 'succeeded' && draft?.acp[job.agent]) {
      const next = withEnabledAgent(withLoginAuth(draft, job.agent), job.agent)
      save.mutate(next)
    } else {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
    }
    queryClient.invalidateQueries({ queryKey: keys.acpAgents })
  })

  const login = useMutation({
    mutationFn: ({ agent, auth }: { agent: string; auth?: ACPAuthDraft }) =>
      startACPAuthLogin(agent, auth),
    onSuccess: (job) => {
      trackLoginJob(job)
      toast(`Started ${authProviderLabel(job.agent)} sign-in`)
    },
    onError: (error: Error) => toast(`Couldn't start sign-in: ${error.message}`, 'danger'),
  })

  const disconnect = useMutation({
    mutationFn: (agent: string) => disconnectACPAuth(agent),
    onSuccess: (settings, agent) => {
      forgetLoginJob(agent)
      setDraft(cloneAgentSettings(settings))
      queryClient.setQueryData<AgentSettingsData>(keys.agentSettings, settings)
      toast(`Disconnected ${authProviderLabel(agent)}`)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
    onError: (error: Error) => toast(`Couldn't disconnect: ${error.message}`, 'danger'),
  })

  const invalid = draft ? hasInvalidACPProvider(draft) : true
  const canSave = draft != null && !invalid && dirty && !save.isPending

  return (
    <SettingsSection
      title="Agents (ACP)"
      description="Configure ACP agents, auth, and per-agent defaults."
      canSave={canSave}
      saving={save.isPending}
      onSave={() => draft && save.mutate(normalizeACPAgentsEnabled(draft))}
    >
      {settings.isError ? (
        <p className="py-2 text-[13px] text-danger">{settings.error.message}</p>
      ) : settings.isPending || !draft ? (
        <SkeletonRows count={3} />
      ) : (
        <div className="flex flex-col gap-3">
          {draft.agents.map((agent) => (
            <ACPAgentRow
              key={agent}
              agent={agent}
              settings={draft}
              disabled={save.isPending}
              loginJob={loginJobs[agent]}
              loginPending={login.isPending && login.variables?.agent === agent}
              disconnecting={disconnect.isPending && disconnect.variables === agent}
              onStartLogin={(auth) =>
                login.mutate({ agent, auth: loginAuth(agent, auth) })
              }
              onDisconnect={() => disconnect.mutate(agent)}
              onOpenProviders={onOpenProviders}
              onChange={setDraft}
            />
          ))}
        </div>
      )}
    </SettingsSection>
  )
}

function ACPAgentRow({
  agent,
  settings,
  disabled,
  loginJob,
  loginPending,
  disconnecting,
  onStartLogin,
  onDisconnect,
  onOpenProviders,
  onChange,
}: {
  agent: string
  settings: AgentSettingsData
  disabled: boolean
  loginJob?: ACPAuthLogin
  loginPending: boolean
  disconnecting: boolean
  onStartLogin: (auth: ACPAuthDraft) => void
  onDisconnect: () => void
  onOpenProviders: () => void
  onChange: (settings: AgentSettingsData) => void
}) {
  const current = settings.acp[agent] ?? emptyACPAgent
  const authStatus = settings.acp_auth?.[agent]
  const options = settings.acp_options?.[agent]
  const supportsAuth = options?.supports_auth ?? true
  const usesModelProvider = acpUsesModelProvider(settings, agent)
  const providerOptions = selectableACPModelProviders(settings, agent)
  const selectedProvider = selectedACPModelProvider(settings, agent)
  const selectedProviderNativeAuth = acpProviderUsesNativeAuth(settings, agent, current.model_provider)
  const showAuthPanel = supportsAuth && (!usesModelProvider || selectedProviderNativeAuth)
  const providerReady = acpAgentEnableable(settings, agent)
  const checked = acpAgentEnabled(settings, agent)
  const enableDescription = providerReady
    ? 'Show this ACP client in agent pickers.'
    : selectedProviderNativeAuth || (supportsAuth && !usesModelProvider)
      ? `Connect ${authProviderLabel(agent)} or add an API key before enabling.`
      : usesModelProvider && selectedProvider
        ? `Connect ${selectedProvider.label} in Model Providers before enabling.`
        : 'Select a provider before enabling.'
  const [expanded, setExpanded] = useState(false)
  const reasoningModel = (current.model ?? '').trim() || selectedProvider?.default_model || ''
  const reasoningEffort = current.reasoning_effort ?? ''
  const {
    modelSuggestions,
    modelsLoading,
    reasoningOptions,
    reasoningEffortSupported,
    reasoningForModel,
  } = useModelReasoningState({
    settings,
    agent,
    model: reasoningModel,
    reasoningEffort,
    usesProvider: usesModelProvider,
    provider: current.model_provider,
    selectedProvider,
    settingsMode: true,
  })
  const normalizedReasoningEffort = reasoningEffortSupported ? reasoningEffort : ''
  const update = useCallback((next: Partial<typeof current>) => {
    const value = { ...current, ...next }
    const nextSettings = {
      ...settings,
      acp: {
        ...settings.acp,
        [agent]: value,
      },
    }
    onChange(normalizeACPAgentEnabled(nextSettings, agent))
  }, [agent, current, onChange, settings])
  useEffect(() => {
    if (reasoningEffort !== normalizedReasoningEffort) {
      update({ reasoning_effort: normalizedReasoningEffort })
    }
  }, [normalizedReasoningEffort, reasoningEffort, update])

  return (
    <SettingsCard className="overflow-hidden">
      {/* Collapsed header: the whole row toggles expand; the Switch stays live so
          an agent can be enabled/disabled without opening its details. */}
      <div className="group flex items-center gap-3 pr-3 transition-colors duration-150 hover:bg-surface-2/50 focus-within:bg-surface-2/50">
        <button
          type="button"
          onClick={() => setExpanded((open) => !open)}
          aria-expanded={expanded}
          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${agentLabel(agent)}`}
          className="flex min-w-0 flex-1 items-center gap-2.5 py-2 pl-3 text-left"
        >
          <ChevronDown
            size={15}
            className={`shrink-0 text-ink-3 transition-transform duration-150 group-hover:text-ink ${expanded ? '' : '-rotate-90'}`}
          />
          <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
            <AgentAvatar agent={agent} size={18} />
          </span>
          <span className="min-w-0">
            <span className="block truncate text-[13px] font-medium text-ink" title={agent}>
              {agentLabel(agent)}
            </span>
            {/* An enabled agent is self-evidently connected, so only surface the
                actionable hint when it can't be enabled yet. */}
            {providerReady ? null : (
              <span className="mt-0.5 flex min-w-0 items-center gap-1 text-[12px] text-ink-3">
                <CircleAlert size={12} className="shrink-0 text-accent" />
                <span className="truncate">{enableDescription}</span>
              </span>
            )}
          </span>
        </button>
        <Switch
          checked={checked}
          disabled={disabled || !providerReady}
          onChange={(enabled) => update({ enabled })}
          aria-label={`Enable ${agentLabel(agent)}`}
        />
      </div>

      <Collapse open={expanded}>
        {showAuthPanel ? (
          <div className="border-t border-border/70 px-3 py-2.5">
            {/* Auth is never gated by the Enabled toggle: you must be able to connect
                an agent you skipped in onboarding. Connecting it turns it on.
                Provider-backed agents inherit their key from the linked Model
                Provider, so they show a read-only connection status instead. */}
            <AgentAuthPanel
              agent={agent}
              disabled={disabled}
              status={authStatus}
              apiKeyValue={settings.acp_keys?.[agent] ?? ''}
              loginJob={loginJob}
              loginPending={loginPending}
              disconnecting={disconnecting}
              onStartLogin={() => onStartLogin(current.auth)}
              onDisconnect={onDisconnect}
              onAPIKeyChange={(value) =>
                onChange({
                  ...settings,
                  // Adding a key connects the agent — enable it so it's usable.
                  acp: value.trim() ? { ...settings.acp, [agent]: { ...current, enabled: true } } : settings.acp,
                  acp_keys: {
                    ...(settings.acp_keys ?? {}),
                    [agent]: value,
                  },
                })
              }
            />
          </div>
        ) : null}

        {usesModelProvider ? (
          <>
            <SettingsRow title="Provider" description="API provider used for this ACP client.">
              <Select
                value={current.model_provider ?? ''}
                options={providerOptions.map((provider) => ({
                  value: provider.id,
                  label: provider.label,
                  description: provider.base_url,
                }))}
                disabled={disabled}
                onChange={(model_provider) => {
                  const nextProvider = providerOptions.find((provider) => provider.id === model_provider)
                  const model =
                    (current.model ?? '').trim() === '' ||
                    current.model === selectedProvider?.default_model
                      ? (nextProvider?.default_model ?? '')
                      : current.model
                  update({ model_provider, model })
                }}
                aria-label={`${agentLabel(agent)} provider`}
                className={rowControlClass}
              />
            </SettingsRow>
            {selectedProviderNativeAuth ? null : (
              <div className="border-t border-border/70 px-3 py-2.5">
                <ProviderConnectionStrip provider={selectedProvider} onOpenProviders={onOpenProviders} />
              </div>
            )}
          </>
        ) : null}

        <SettingsRow title="Model" description="Model copied into new threads for this client.">
          <ModelCombobox
            value={current.model ?? ''}
            suggestions={modelSuggestions}
            loading={modelsLoading}
            disabled={disabled}
            onChange={(model) => {
              const nextReasoning = reasoningForModel(model, reasoningEffort)
              update({
                model,
                reasoning_effort: nextReasoning.supported ? reasoningEffort : '',
              })
            }}
            aria-label={`${agentLabel(agent)} model`}
            className={rowControlClass}
          />
        </SettingsRow>

        {/* Show only when there's an effort to choose beyond "None": agents whose
            thinking level is model-encoded (Antigravity) advertise no efforts. */}
        {reasoningOptions.some((option) => option.value !== '') ? (
          <SettingsRow title="Reasoning" description="Reasoning effort copied into new threads.">
            <Select
              value={normalizedReasoningEffort}
              options={reasoningOptions}
              disabled={disabled}
              onChange={(reasoning_effort) => update({ reasoning_effort })}
              aria-label={`${agentLabel(agent)} reasoning effort`}
              className={rowControlClass}
            />
          </SettingsRow>
        ) : null}
      </Collapse>
    </SettingsCard>
  )
}

function AgentAuthPanel({
  agent,
  disabled,
  status,
  apiKeyValue,
  loginJob,
  loginPending,
  disconnecting,
  onStartLogin,
  onDisconnect,
  onAPIKeyChange,
}: {
  agent: string
  disabled: boolean
  status?: ACPAgentAuthStatus
  apiKeyValue: string
  loginJob?: ACPAuthLogin
  loginPending: boolean
  disconnecting: boolean
  onStartLogin: () => void
  onDisconnect: () => void
  onAPIKeyChange: (value: string) => void
}) {
  const apiKeyEnv = status?.api_key?.source_env
  const canKey = Boolean(apiKeyEnv)
  const canLogin = Boolean(status?.login_command && status.login_command_available)
  const keyCopy = agentAPIKeyCopy(agent, authProviderLabel(agent), Boolean(status?.api_key_configured))
  const hasDraftKey = apiKeyValue.trim().length > 0
  const running = loginPending || loginJob?.status === 'running'
  const [method, setMethod] = useState<'login' | 'key'>(
    canKey && (!canLogin || hasDraftKey || status?.api_key_configured) ? 'key' : 'login',
  )
  useEffect(() => {
    if (canKey && !canLogin && method === 'login') setMethod('key')
  }, [canKey, canLogin, method])

  // Connected: a clean confirmation + a way to disconnect (or switch method).
  if (status?.authenticated) {
    const viaKey = status.auth_kind === 'api_key'
    const noKey = status.auth_kind === 'none'
    return (
      <div className="flex items-center justify-between gap-3 rounded-control bg-bg px-3 py-2.5">
        <span className="flex min-w-0 items-center gap-2 text-[13px] text-ink">
          <CheckCircle2 size={16} className="shrink-0 text-primary" />
          {noKey
            ? 'No provider key required'
            : viaKey
              ? keyCopy.connected
              : `Connected with your ${authProviderLabel(agent)} account`}
        </span>
        {noKey ? null : (
          <Button variant="ghost" size="sm" disabled={disabled || disconnecting} onClick={onDisconnect}>
            {disconnecting ? <LoaderCircle size={13} className="animate-spin" /> : null}
            Disconnect
          </Button>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2.5">
      {canKey ? (
        canLogin ? (
          <Segmented
            layoutId={`settings-auth-${agent}`}
            value={method}
            onChange={setMethod}
            disabled={disabled}
            options={[
              { value: 'login', label: 'Sign in', icon: <LogIn size={13} /> },
              { value: 'key', label: 'API key', icon: <KeyRound size={13} /> },
            ]}
          />
        ) : null
      ) : null}

      {(method === 'login' && canLogin) || !canKey ? (
        <div className="flex flex-col items-start gap-2">
          <Button variant="primary" size="md" disabled={disabled || running} onClick={onStartLogin}>
            {running ? <LoaderCircle size={14} className="animate-spin" /> : <LogIn size={14} />}
            {running ? 'Waiting for sign-in…' : `Sign in with ${authProviderLabel(agent)}`}
          </Button>
          <div className="w-full">
            <AuthLoginStatus job={loginJob} running={running} />
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          <Input
            type="password"
            value={apiKeyValue}
            disabled={disabled}
            onChange={(event) => onAPIKeyChange(event.target.value)}
            placeholder={keyCopy.placeholder}
            autoComplete="off"
            spellCheck={false}
            className="h-8 rounded-full bg-bg px-3 py-0 font-mono text-[12px]"
            aria-label={`${agentLabel(agent)} API key`}
          />
          <p className="text-[12px] text-ink-3">
            {keyCopy.description}
          </p>
        </div>
      )}
    </div>
  )
}

// Provider-backed agents inherit their key from the linked Model Provider, so the
// ACP screen never edits a key here — it shows the provider's connection status and
// sends the user to Model Providers to manage the key.
function ProviderConnectionStrip({
  provider,
  onOpenProviders,
}: {
  provider?: ModelProviderOption
  onOpenProviders: () => void
}) {
  const connected = provider ? modelProviderConnected(provider) : false
  const needsKey = provider ? modelProviderRequiresKey(provider) : false
  const label = provider?.label ?? 'this provider'
  return (
    <div className="flex flex-col gap-2 rounded-control bg-bg px-3 py-2.5">
      <div className="flex items-center justify-between gap-3">
        <span className="flex min-w-0 items-center gap-2 text-[13px] text-ink">
          {connected ? (
            <CheckCircle2 size={16} className="shrink-0 text-primary" />
          ) : (
            <CircleAlert size={16} className="shrink-0 text-accent" />
          )}
          <span className="min-w-0 truncate">
            {!provider
              ? 'Select a provider above'
              : connected
                ? `Connected via ${label}`
                : `${label} isn’t connected yet`}
          </span>
        </span>
        <Button variant="ghost" size="sm" onClick={onOpenProviders}>
          Model Providers
        </Button>
      </div>
      <p className="text-[12px] text-ink-3">
        {!provider
          ? 'Pick a provider to see its connection status.'
          : !needsKey
            ? `${label} needs no API key.`
            : connected
              ? 'API key is inherited from the linked provider — manage it in Model Providers.'
              : 'Add this provider’s API key in Model Providers; this agent inherits it.'}
      </p>
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
    <div className="grid grid-cols-1 gap-2 border-t border-border/70 px-3 py-2.5 first:border-t-0 md:grid-cols-[minmax(0,1fr)_minmax(220px,320px)] md:items-center">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        <p className="mt-0.5 text-[12px] text-ink-3">{description}</p>
      </div>
      <div className="min-w-0 md:justify-self-end">{children}</div>
    </div>
  )
}
