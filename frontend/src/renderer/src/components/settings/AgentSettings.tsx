import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, KeyRound, LoaderCircle, LogIn, Save, Terminal } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { Segmented } from '@/components/ui/Segmented'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel, authProviderLabel } from '@/lib/agentLabel'
import {
  agentSettingsQuery,
  cloneAgentSettings,
  disconnectACPAuth,
  startACPAuthLogin,
  updateAgentSettings,
} from '@/lib/api/settings'
import {
  acpUsesModelProvider,
  acpUsesNativeProvider,
  selectableACPModelProviders,
} from '@/lib/agentRuntimes'
import { useACPLoginPolling } from '@/lib/hooks/useACPLoginPolling'
import type { ACPAgentAuthStatus, ACPAuthLogin, AgentSettings as AgentSettingsData } from '@/lib/api/types'
import {
  acpAgentModelSuggestions,
  modelSuggestionsForProvider,
  OPENAI_MODELS,
  openRouterModelsQuery,
} from '@/lib/models'
import { keys } from '@/lib/query/keys'
import { acpReasoningEffortOptions, REASONING_EFFORT_OPTIONS } from '@/lib/reasoningEfforts'

const inputClass =
  'h-7 w-full rounded-full bg-ink/10 px-3 text-[12px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25 disabled:opacity-50'

const rowControlClass = 'w-full md:w-[320px]'
type ACPAuthDraft = AgentSettingsData['acp'][string]['auth']

// Here '' means "no effort configured" rather than "inherit the default".
const settingsReasoningOptions = (options = REASONING_EFFORT_OPTIONS) =>
  options.map((option) => (option.value === '' ? { ...option, label: 'None' } : option))

function settingsKey(settings: AgentSettingsData | null): string {
  return settings ? JSON.stringify(settings) : ''
}

// Returns a clone with one ACP agent turned on — used when a sign-in or API key
// connects an agent, so it becomes a usable runtime without a separate toggle.
function withEnabledAgent(settings: AgentSettingsData, agent: string): AgentSettingsData {
  const next = cloneAgentSettings(settings)
  const current = next.acp[agent]
  if (current) next.acp[agent] = { ...current, enabled: true }
  return next
}

function hasEnabledACPWithoutCommand(settings: AgentSettingsData): boolean {
  return settings.agents.some((agent) => {
    if (!agentRequiresCommand(settings, agent)) return false
    const current = settings.acp[agent]
    return Boolean(current?.enabled) && (current.command ?? '').trim() === ''
  })
}

function agentRequiresCommand(settings: AgentSettingsData, agent: string): boolean {
  return settings.acp_options?.[agent]?.requires_command ?? true
}

function agentUsesModelProvider(settings: AgentSettingsData, agent: string): boolean {
  return acpUsesModelProvider(settings, agent)
}

function hasInvalidACPProvider(settings: AgentSettingsData): boolean {
  return settings.agents.some((agent) => {
    if (!agentUsesModelProvider(settings, agent)) return false
    const current = settings.acp[agent]
    if (!current?.enabled) return false
    return (current.model_provider ?? '').trim() === '' || (current.model ?? '').trim() === ''
  })
}

export function AgentProvidersSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const settings = useQuery(agentSettingsQuery)
  const [draft, setDraft] = useState<AgentSettingsData | null>(null)
  // A freshly pasted native provider key, keyed by provider id. Write-only —
  // the backend never returns the value, so it lives outside the draft.
  const [providerKeys, setProviderKeys] = useState<Record<string, string>>({})

  useEffect(() => {
    if (settings.data) setDraft(cloneAgentSettings(settings.data))
  }, [settings.data])

  const save = useMutation({
    mutationFn: (input: AgentSettingsData) => updateAgentSettings(input, providerKeys),
    onSuccess: (saved) => {
      setDraft(cloneAgentSettings(saved))
      setProviderKeys({})
      toast('Saved agent settings')
    },
    onError: (error: Error) => toast(`Couldn't save agent settings: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  // A finished sign-in connects the agent: turn it on and persist so it works
  // right away (matching onboarding, no extra Enabled toggle). The hook reads
  // this through a ref, so `draft`/`save` are always current here.
  const { loginJobs, trackLoginJob, forgetLoginJob } = useACPLoginPolling((job) => {
    if (job.status === 'succeeded' && draft?.acp[job.agent] && !draft.acp[job.agent].enabled) {
      save.mutate(withEnabledAgent(draft, job.agent))
    } else {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
    }
    queryClient.invalidateQueries({ queryKey: keys.acpAgents })
  })

  const login = useMutation({
    mutationFn: ({ agent, auth }: { agent: string; auth?: AgentSettingsData['acp'][string]['auth'] }) =>
      startACPAuthLogin(agent, auth),
    onSuccess: (job) => {
      trackLoginJob(job)
      toast(`Started ${authProviderLabel(job.agent)} sign-in`)
    },
    onError: (error: Error) => toast(`Couldn't start sign-in: ${error.message}`, 'danger'),
  })

  const disconnect = useMutation({
    mutationFn: (agent: string) => disconnectACPAuth(agent),
    onSuccess: (_status, agent) => {
      forgetLoginJob(agent)
      toast(`Disconnected ${authProviderLabel(agent)}`)
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
    onError: (error: Error) => toast(`Couldn't disconnect: ${error.message}`, 'danger'),
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
  const providerKeyDirty = Object.values(providerKeys).some((value) => value.trim().length > 0)
  const nativeProviders = (draft?.providers ?? []).filter((provider) => provider.implemented)
  const invalid = draft
    ? (draft.native.model_provider ?? '').trim() === '' ||
      draft.native.model.trim() === '' ||
      hasEnabledACPWithoutCommand(draft) ||
      hasInvalidACPProvider(draft)
    : true
  const canSave = draft != null && !invalid && (dirty || providerKeyDirty) && !save.isPending

  const selectedProvider = draft?.native.model_provider ?? ''
  const selectedNativeProvider = nativeProviders.find((provider) => provider.id === selectedProvider)
  const selectedProviderEnv = selectedNativeProvider?.api_key_env
  const selectedProviderConfigured = Boolean(selectedNativeProvider?.configured)

  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Providers</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Configure model providers once. ACP agents can reuse them when they are set to use provider defaults.
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
                    options={nativeProviders.map((provider) => ({
                      value: provider.id,
                      label: provider.label,
                      description: provider.base_url,
                    }))}
                    disabled={save.isPending}
                    onChange={(model_provider) => {
                      const nextProvider = nativeProviders.find((provider) => provider.id === model_provider)
                      const currentProvider = nativeProviders.find(
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
                <SettingsRow
                  title="Provider key"
                  description={
                    selectedProviderConfigured
                      ? `${selectedProviderEnv} is configured — paste a new key to replace it.`
                      : `Paste an API key; stored on the backend as ${selectedProviderEnv ?? 'the provider env var'}.`
                  }
                >
                  <Input
                    type="password"
                    value={providerKeys[selectedProvider] ?? ''}
                    disabled={save.isPending || !selectedProvider}
                    onChange={(event) =>
                      setProviderKeys({ ...providerKeys, [selectedProvider]: event.target.value })
                    }
                    placeholder={
                      selectedProviderConfigured
                        ? `${selectedProviderEnv} configured`
                        : (selectedProviderEnv ?? 'API key')
                    }
                    autoComplete="off"
                    spellCheck={false}
                    className={`${rowControlClass} h-8 rounded-full bg-bg px-3 py-0 font-mono text-[12px]`}
                    aria-label="Native provider API key"
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
                    options={settingsReasoningOptions()}
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

          </div>
        )}
      </div>
    </section>
  )
}

export function ACPAgentsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const settings = useQuery(agentSettingsQuery)
  const [draft, setDraft] = useState<AgentSettingsData | null>(null)
  const [providerKeys, setProviderKeys] = useState<Record<string, string>>({})

  useEffect(() => {
    if (settings.data) setDraft(cloneAgentSettings(settings.data))
  }, [settings.data])

  const save = useMutation({
    mutationFn: (input: AgentSettingsData) => updateAgentSettings(input, providerKeys),
    onSuccess: (saved) => {
      setDraft(cloneAgentSettings(saved))
      setProviderKeys({})
      toast('Saved agent settings')
    },
    onError: (error: Error) => toast(`Couldn't save agent settings: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  const { loginJobs, trackLoginJob, forgetLoginJob } = useACPLoginPolling((job) => {
    if (job.status === 'succeeded' && draft?.acp[job.agent] && !draft.acp[job.agent].enabled) {
      save.mutate(withEnabledAgent(draft, job.agent))
    } else {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
    }
    queryClient.invalidateQueries({ queryKey: keys.acpAgents })
  })

  const login = useMutation({
    mutationFn: ({ agent, auth }: { agent: string; auth?: AgentSettingsData['acp'][string]['auth'] }) =>
      startACPAuthLogin(agent, auth),
    onSuccess: (job) => {
      trackLoginJob(job)
      toast(`Started ${authProviderLabel(job.agent)} sign-in`)
    },
    onError: (error: Error) => toast(`Couldn't start sign-in: ${error.message}`, 'danger'),
  })

  const disconnect = useMutation({
    mutationFn: (agent: string) => disconnectACPAuth(agent),
    onSuccess: (_status, agent) => {
      forgetLoginJob(agent)
      toast(`Disconnected ${authProviderLabel(agent)}`)
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
    onError: (error: Error) => toast(`Couldn't disconnect: ${error.message}`, 'danger'),
  })

  const dirty = useMemo(
    () => settingsKey(draft) !== settingsKey(settings.data ?? null),
    [draft, settings.data],
  )
  const providerKeyDirty = Object.values(providerKeys).some((value) => value.trim().length > 0)
  const invalid = draft ? hasEnabledACPWithoutCommand(draft) || hasInvalidACPProvider(draft) : true
  const canSave = draft != null && !invalid && (dirty || providerKeyDirty) && !save.isPending

  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Agents (ACP)</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Configure ACP runtimes, auth, and per-agent defaults.
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
          <div className="flex flex-col gap-3">
            {draft.agents.map((agent) => (
              <ACPAgentRow
                key={agent}
                agent={agent}
                settings={draft}
                providerKeys={providerKeys}
                disabled={save.isPending}
                loginJob={loginJobs[agent]}
                loginPending={login.isPending && login.variables?.agent === agent}
                disconnecting={disconnect.isPending && disconnect.variables === agent}
                onStartLogin={(auth) => login.mutate({ agent, auth })}
                onDisconnect={() => disconnect.mutate(agent)}
                onProviderKeyChange={(provider, value) => setProviderKeys({ ...providerKeys, [provider]: value })}
                onChange={setDraft}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

function ACPAgentRow({
  agent,
  settings,
  providerKeys,
  disabled,
  loginJob,
  loginPending,
  disconnecting,
  onStartLogin,
  onDisconnect,
  onProviderKeyChange,
  onChange,
}: {
  agent: string
  settings: AgentSettingsData
  providerKeys: Record<string, string>
  disabled: boolean
  loginJob?: ACPAuthLogin
  loginPending: boolean
  disconnecting: boolean
  onStartLogin: (auth: ACPAuthDraft) => void
  onDisconnect: () => void
  onProviderKeyChange: (provider: string, value: string) => void
  onChange: (settings: AgentSettingsData) => void
}) {
  const current = settings.acp[agent] ?? {
    enabled: false,
    command: '',
    model_provider: '',
    model: '',
    reasoning_effort: '',
    auth: { mode: 'auto', path: '' },
  }
  const authStatus = settings.acp_auth?.[agent]
  const options = settings.acp_options?.[agent]
  const requiresCommand = options?.requires_command ?? true
  const supportsAuth = options?.supports_auth ?? true
  const usesNativeProvider = acpUsesNativeProvider(settings, agent)
  const usesModelProvider = acpUsesModelProvider(settings, agent)
  const [commandOpen, setCommandOpen] = useState(requiresCommand && (current.command ?? '').trim() === '')
  useEffect(() => {
    if (requiresCommand && current.enabled && (current.command ?? '').trim() === '') setCommandOpen(true)
  }, [current.command, current.enabled, requiresCommand])
  const controlsDisabled = disabled || !current.enabled
  const providerOptions = selectableACPModelProviders(settings, agent)
  const selectedProvider = providerOptions.find((provider) => provider.id === current.model_provider)
  const selectedProviderEnv = selectedProvider?.api_key_env
  const selectedProviderConfigured = Boolean(selectedProvider?.configured)
  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: usesModelProvider && current.model_provider === 'openrouter',
  })
  const modelSuggestions = usesModelProvider
    ? modelSuggestionsForProvider(selectedProvider, openRouterModels.data ?? [])
    : acpAgentModelSuggestions(agent)
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

      {supportsAuth ? (
        <div className="border-t border-border/70 px-3 py-3">
      {/* Auth is never gated by the Enabled toggle: you must be able to connect
          an agent you skipped in onboarding. Connecting it turns it on. */}
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

      {usesNativeProvider ? null : (
        <>
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
                  disabled={controlsDisabled}
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
              <SettingsRow
                title="Provider key"
                description={
                  selectedProvider?.requires_api_key === false
                    ? 'This provider does not need an API key.'
                    : selectedProviderConfigured
                      ? `${selectedProviderEnv} is configured — paste a new key to replace it.`
                      : `Paste an API key; stored on the backend as ${selectedProviderEnv ?? 'the provider env var'}.`
                }
              >
                <Input
                  type="password"
                  value={providerKeys[current.model_provider ?? ''] ?? ''}
                  disabled={
                    disabled ||
                    !(current.model_provider ?? '').trim() ||
                    selectedProvider?.requires_api_key === false
                  }
                  onChange={(event) =>
                    onProviderKeyChange(current.model_provider ?? '', event.target.value)
                  }
                  placeholder={
                    selectedProvider?.requires_api_key === false
                      ? 'No key required'
                      : selectedProviderConfigured
                        ? `${selectedProviderEnv} configured`
                        : (selectedProviderEnv ?? 'API key')
                  }
                  autoComplete="off"
                  spellCheck={false}
                  className={`${rowControlClass} h-8 rounded-full bg-bg px-3 py-0 font-mono text-[12px]`}
                  aria-label={`${agentLabel(agent)} provider API key`}
                />
              </SettingsRow>
            </>
          ) : null}
          <SettingsRow title="Model" description="Model copied into new threads for this client.">
            <ModelCombobox
              value={current.model ?? ''}
              suggestions={modelSuggestions}
              loading={openRouterModels.isLoading}
              disabled={controlsDisabled}
              onChange={(model) => update({ model })}
              aria-label={`${agentLabel(agent)} model`}
              className={rowControlClass}
            />
          </SettingsRow>

          <SettingsRow title="Reasoning" description="Reasoning effort copied into new threads.">
            <Select
              value={current.reasoning_effort ?? ''}
              options={settingsReasoningOptions(acpReasoningEffortOptions(settings, agent))}
              disabled={controlsDisabled}
              onChange={(reasoning_effort) => update({ reasoning_effort })}
              aria-label={`${agentLabel(agent)} reasoning effort`}
              className={rowControlClass}
            />
          </SettingsRow>
        </>
      )}

      {requiresCommand ? (
        <>
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
        </>
      ) : null}
    </div>
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
      <div className="flex items-center justify-between gap-3 rounded-[10px] bg-bg px-3 py-2.5">
        <span className="flex min-w-0 items-center gap-2 text-[13px] text-ink">
          <CheckCircle2 size={16} className="shrink-0 text-primary" />
          {noKey
            ? 'No provider key required'
            : viaKey
              ? 'Connected with an API key'
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
            placeholder={status?.api_key_configured ? 'Already set up' : 'Paste an API key'}
            autoComplete="off"
            spellCheck={false}
            className="h-8 rounded-full bg-bg px-3 py-0 font-mono text-[12px]"
            aria-label={`${agentLabel(agent)} API key`}
          />
          <p className="text-[12px] text-ink-3">
            jaz passes this key straight to {authProviderLabel(agent)}.
          </p>
        </div>
      )}
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
