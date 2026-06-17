import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  ChevronDown,
  ExternalLink,
  KeyRound,
  LoaderCircle,
  LogIn,
  Save,
  Terminal,
} from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { ProviderLogo } from '@/components/settings/ProviderLogo'
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
  type ModelSuggestion,
  modelSuggestionsForProvider,
  openRouterModelsQuery,
} from '@/lib/models'
import { keys } from '@/lib/query/keys'
import { acpReasoningEffortOptions, REASONING_EFFORT_OPTIONS } from '@/lib/reasoningEfforts'

const inputClass =
  'h-7 w-full rounded-full bg-ink/10 px-3 text-[12px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25 disabled:opacity-50'

const rowControlClass = 'w-full md:w-[320px]'
type ACPAuthDraft = AgentSettingsData['acp'][string]['auth']

const EASE = [0.22, 1, 0.36, 1] as const

// Where to grab a key for each native provider — the answer to "where does the
// key even come from?". Keyed by the backend provider id.
const PROVIDER_KEY_URLS: Record<string, string> = {
  openrouter: 'https://openrouter.ai/keys',
  openai: 'https://platform.openai.com/api-keys',
}

type ProviderConnection = 'connected' | 'disconnected' | 'no-key'

// Drops the scheme so an endpoint reads as a compact host+path chip.
function prettyEndpoint(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

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

// Shared shell for the two agent-settings screens: load the settings, hold an
// editable draft plus write-only provider keys, and expose a save that re-seeds
// the draft and refreshes dependent queries. `label` only varies the toast copy.
function useAgentSettingsDraft(label: string) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const settings = useQuery(agentSettingsQuery)
  const [draft, setDraft] = useState<AgentSettingsData | null>(null)
  // The backend never returns provider key values, so they live outside the draft.
  const [providerKeys, setProviderKeys] = useState<Record<string, string>>({})

  useEffect(() => {
    if (settings.data) setDraft(cloneAgentSettings(settings.data))
  }, [settings.data])

  const save = useMutation({
    mutationFn: (input: AgentSettingsData) => updateAgentSettings(input, providerKeys),
    onSuccess: (saved) => {
      setDraft(cloneAgentSettings(saved))
      setProviderKeys({})
      toast(`Saved ${label}`)
    },
    onError: (error: Error) => toast(`Couldn't save ${label}: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  const dirty = useMemo(
    () => settingsKey(draft) !== settingsKey(settings.data ?? null),
    [draft, settings.data],
  )
  const providerKeyDirty = Object.values(providerKeys).some((value) => value.trim().length > 0)

  return { queryClient, toast, settings, draft, setDraft, providerKeys, setProviderKeys, save, dirty, providerKeyDirty }
}

// The chrome both agent-settings screens share: heading, Save button, body slot.
function SettingsSection({
  title,
  description,
  canSave,
  saving,
  onSave,
  children,
}: {
  title: string
  description: string
  canSave: boolean
  saving: boolean
  onSave: () => void
  children: ReactNode
}) {
  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">{title}</p>
          <p className="mt-0.5 text-pretty text-[13px] text-ink-2">{description}</p>
        </div>
        <Button variant="primary" size="md" disabled={!canSave} onClick={onSave}>
          <Save size={14} />
          {saving ? 'Saving...' : 'Save changes'}
        </Button>
      </div>
      <div className="mt-4">{children}</div>
    </section>
  )
}

export function AgentProvidersSettings() {
  const { settings, draft, setDraft, providerKeys, setProviderKeys, save, dirty, providerKeyDirty } =
    useAgentSettingsDraft('providers')

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: draft?.native.model_provider === 'openrouter',
  })
  // Every known model provider — keys are shared, so this one list connects the
  // native agent and every ACP agent set to provider defaults. Implemented ones
  // (the native agent can run them) sort first; locals/customs follow.
  const allProviders = draft?.providers ?? []
  const nativeProviders = allProviders.filter((provider) => provider.implemented)
  const invalid = draft
    ? (draft.native.model_provider ?? '').trim() === '' || draft.native.model.trim() === ''
    : true
  const canSave = draft != null && !invalid && (dirty || providerKeyDirty) && !save.isPending

  const selectedProvider = draft?.native.model_provider ?? ''
  const selectedNativeProvider = nativeProviders.find((provider) => provider.id === selectedProvider)
  const nativeModelSuggestions = modelSuggestionsForProvider(
    selectedNativeProvider,
    openRouterModels.data ?? [],
  )

  // Switching the native default carries its model: keep a hand-typed model, but
  // swap a still-default model to the new provider's default so it stays valid.
  const setNativeProvider = (model_provider: string) => {
    if (!draft) return
    const nextProvider = nativeProviders.find((provider) => provider.id === model_provider)
    const currentProvider = nativeProviders.find(
      (provider) => provider.id === draft.native.model_provider,
    )
    const model =
      draft.native.model.trim() === '' || draft.native.model === currentProvider?.default_model
        ? nextProvider?.default_model || draft.native.model
        : draft.native.model
    setDraft({ ...draft, native: { ...draft.native, model_provider, model } })
  }

  return (
    <SettingsSection
      title="Providers"
      description="Configure model providers once. ACP agents can reuse them when they are set to use provider defaults."
      canSave={canSave}
      saving={save.isPending}
      onSave={() => draft && save.mutate(draft)}
    >
      {settings.isError ? (
        <p className="py-2 text-[13px] text-danger">{settings.error.message}</p>
      ) : settings.isPending || !draft ? (
        <SkeletonRows count={3} />
      ) : (
        <div className="flex flex-col gap-1.5">
          {allProviders.map((provider) => (
            <ProviderRow
              key={provider.id}
              provider={provider}
              keyDraft={providerKeys[provider.id] ?? ''}
              isNativeDefault={provider.implemented && provider.id === selectedProvider}
              nativeModel={draft.native.model}
              nativeReasoning={draft.native.reasoning_effort ?? ''}
              modelSuggestions={nativeModelSuggestions}
              modelsLoading={openRouterModels.isLoading}
              disabled={save.isPending}
              onKeyChange={(value) => setProviderKeys({ ...providerKeys, [provider.id]: value })}
              onUseForNative={() => setNativeProvider(provider.id)}
              onNativeModelChange={(model) =>
                setDraft({ ...draft, native: { ...draft.native, model } })
              }
              onNativeReasoningChange={(reasoning_effort) =>
                setDraft({ ...draft, native: { ...draft.native, reasoning_effort } })
              }
            />
          ))}
        </div>
      )}
    </SettingsSection>
  )
}

export function ACPAgentsSettings() {
  const { queryClient, toast, settings, draft, setDraft, providerKeys, setProviderKeys, save, dirty, providerKeyDirty } =
    useAgentSettingsDraft('agent settings')

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

  const invalid = draft ? hasEnabledACPWithoutCommand(draft) || hasInvalidACPProvider(draft) : true
  const canSave = draft != null && !invalid && (dirty || providerKeyDirty) && !save.isPending

  return (
    <SettingsSection
      title="Agents (ACP)"
      description="Configure ACP runtimes, auth, and per-agent defaults."
      canSave={canSave}
      saving={save.isPending}
      onSave={() => draft && save.mutate(draft)}
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
              providerKeys={providerKeys}
              disabled={save.isPending}
              loginJob={loginJobs[agent]}
              loginPending={login.isPending && login.variables?.agent === agent}
              disconnecting={disconnect.isPending && disconnect.variables === agent}
              onStartLogin={(auth) => login.mutate({ agent, auth })}
              onDisconnect={() => disconnect.mutate(agent)}
              onProviderKeyChange={(provider, value) =>
                setProviderKeys({ ...providerKeys, [provider]: value })
              }
              onChange={setDraft}
            />
          ))}
        </div>
      )}
    </SettingsSection>
  )
}

type ProviderOption = AgentSettingsData['providers'][number]

// One row in the providers list: a collapsed header with the brand mark, a
// connection pill and a check, expanding to the key field (and, for the native
// default, its model + reasoning). Mirrors the onboarding provider card so the
// connect-a-provider gesture reads the same in both places.
function ProviderRow({
  provider,
  keyDraft,
  isNativeDefault,
  nativeModel,
  nativeReasoning,
  modelSuggestions,
  modelsLoading,
  disabled,
  onKeyChange,
  onUseForNative,
  onNativeModelChange,
  onNativeReasoningChange,
}: {
  provider: ProviderOption
  keyDraft: string
  isNativeDefault: boolean
  nativeModel: string
  nativeReasoning: string
  modelSuggestions: ModelSuggestion[]
  modelsLoading: boolean
  disabled: boolean
  onKeyChange: (value: string) => void
  onUseForNative: () => void
  onNativeModelChange: (value: string) => void
  onNativeReasoningChange: (value: string) => void
}) {
  // A provider needs a key only if it has an env var to store one into — the
  // backend omits requires_api_key when false, so a missing api_key_env (Ollama)
  // is the reliable "no key" signal.
  const needsKey = Boolean(provider.api_key_env) && provider.requires_api_key !== false
  const connected = needsKey ? Boolean(provider.configured || keyDraft.trim()) : true
  const state: ProviderConnection = needsKey ? (connected ? 'connected' : 'disconnected') : 'no-key'
  const keyUrl = PROVIDER_KEY_URLS[provider.id]
  // The native default opens by default so its model/reasoning are one glance
  // away; the rest stay collapsed until tapped.
  const [expanded, setExpanded] = useState(isNativeDefault)

  return (
    <div className="overflow-hidden rounded-[12px] bg-surface">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
          <ProviderLogo provider={provider.id} />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-[13.5px] font-medium text-ink">{provider.label}</span>
            <ProviderPill state={state} />
            {isNativeDefault ? (
              <span className="inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium text-ink-2 ring-1 ring-border ring-inset">
                Native default
              </span>
            ) : null}
          </span>
          {provider.base_url ? (
            <span className="truncate font-mono text-[11px] text-ink-3">
              {prettyEndpoint(provider.base_url)}
            </span>
          ) : null}
        </span>
        {state === 'connected' ? <CheckCircle2 size={17} className="shrink-0 text-primary" /> : null}
        <ChevronDown
          size={15}
          className={`shrink-0 text-ink-3 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}
        />
      </button>

      <AnimatePresence initial={false}>
        {expanded ? (
          <motion.div
            key="body"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2, ease: EASE }}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-3 px-3 pb-3 pt-0.5">
              {needsKey ? (
                <div className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-[12px] font-medium text-ink-2">API key</span>
                    {provider.api_key_env ? (
                      <span className="font-mono text-[11px] text-ink-3">{provider.api_key_env}</span>
                    ) : null}
                  </div>
                  <Input
                    type="password"
                    value={keyDraft}
                    disabled={disabled}
                    onChange={(event) => onKeyChange(event.target.value)}
                    placeholder={
                      provider.configured
                        ? 'Configured — paste a new key to replace it'
                        : 'Paste an API key'
                    }
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono text-[12px]"
                    aria-label={`${provider.label} API key`}
                  />
                  {keyUrl ? (
                    <button
                      type="button"
                      onClick={() => window.open(keyUrl, '_blank', 'noopener,noreferrer')}
                      className="inline-flex w-fit items-center gap-1 text-[12px] text-primary transition-colors duration-150 hover:text-primary-strong"
                    >
                      Where do I find my {provider.label} key?
                      <ExternalLink size={12} />
                    </button>
                  ) : null}
                </div>
              ) : (
                <p className="text-pretty text-[12px] text-ink-3">
                  Runs locally on your machine — no API key required.
                </p>
              )}

              {provider.implemented ? (
                isNativeDefault ? (
                  <div className="flex flex-col gap-3 rounded-[10px] bg-bg p-3">
                    <p className="text-[11px] font-medium text-ink-3">Native agent default</p>
                    <NativeDefaultField label="Model">
                      <ModelCombobox
                        value={nativeModel}
                        suggestions={modelSuggestions}
                        loading={modelsLoading}
                        disabled={disabled}
                        onChange={onNativeModelChange}
                        aria-label="Native model"
                        className="w-full sm:w-[230px]"
                      />
                    </NativeDefaultField>
                    <NativeDefaultField label="Reasoning">
                      <Select
                        value={nativeReasoning}
                        options={settingsReasoningOptions()}
                        disabled={disabled}
                        onChange={onNativeReasoningChange}
                        aria-label="Native reasoning effort"
                        className="w-full sm:w-[230px]"
                      />
                    </NativeDefaultField>
                  </div>
                ) : (
                  <Button
                    variant="secondary"
                    size="md"
                    disabled={disabled}
                    onClick={onUseForNative}
                    className="w-fit ring-1 ring-border ring-inset"
                  >
                    Use for native agent
                  </Button>
                )
              ) : (
                <p className="text-pretty text-[12px] text-ink-3">
                  Available to ACP agents set to use this provider.
                </p>
              )}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function NativeDefaultField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:justify-between">
      <span className="text-[13px] text-ink-2">{label}</span>
      {children}
    </div>
  )
}

function ProviderPill({ state }: { state: ProviderConnection }) {
  const tone =
    state === 'connected'
      ? 'bg-primary-soft text-primary-strong'
      : state === 'no-key'
        ? 'bg-surface-2 text-ink-3'
        : 'bg-accent-soft text-accent-strong'
  const text =
    state === 'connected' ? 'Connected' : state === 'no-key' ? 'No key needed' : 'Not connected'
  return (
    <span
      className={`inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium ${tone}`}
    >
      {text}
    </span>
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
