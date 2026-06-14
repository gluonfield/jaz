import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, KeyRound, LoaderCircle, LogIn, Save, ShieldCheck, Terminal } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { agentSettingsQuery, getACPAuthLogin, startACPAuthLogin, updateAgentSettings } from '@/lib/api/settings'
import type { ACPAgentAuthStatus, ACPAuthLogin, AgentSettings as AgentSettingsData } from '@/lib/api/types'
import { acpAgentModelSuggestions, OPENAI_MODELS, openRouterModelsQuery } from '@/lib/models'
import { keys } from '@/lib/query/keys'
import { acpReasoningEffortOptions, REASONING_EFFORT_OPTIONS } from '@/lib/reasoningEfforts'

const inputClass =
  'h-7 w-full rounded-full bg-ink/10 px-3 text-[12px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25 disabled:opacity-50'

const rowControlClass = 'w-full md:w-[320px]'
type ACPAuthDraft = AgentSettingsData['acp'][string]['auth']
const AUTH_MODE_OPTIONS = [
  { value: 'auto', label: 'Auto', description: 'Use the best profile detected on the backend' },
  { value: 'existing_cli', label: 'Existing CLI', description: 'Reuse the backend user profile' },
  { value: 'jaz_profile', label: 'Jaz profile', description: 'Use isolated Jaz agent auth' },
]

const authModeOptions = (agent: string) =>
  agent === 'grok' ? AUTH_MODE_OPTIONS.filter((option) => option.value !== 'jaz_profile') : AUTH_MODE_OPTIONS

const authModeValue = (agent: string, mode: 'auto' | 'existing_cli' | 'jaz_profile' | undefined) =>
  agent === 'grok' && mode === 'jaz_profile' ? 'auto' : (mode ?? 'auto')

// Here '' means "no effort configured" rather than "inherit the default".
const settingsReasoningOptions = (options = REASONING_EFFORT_OPTIONS) =>
  options.map((option) => (option.value === '' ? { ...option, label: 'None' } : option))

function cloneSettings(settings: AgentSettingsData): AgentSettingsData {
  return {
    native: { ...settings.native },
    providers: [...(settings.providers ?? [])],
    acp_auth: { ...(settings.acp_auth ?? {}) },
    acp_keys: { ...(settings.acp_keys ?? {}) },
    acp: Object.fromEntries(
      Object.entries(settings.acp).map(([agent, value]) => [
        agent,
        { ...value, auth: value.auth ? { ...value.auth } : undefined },
      ]),
    ),
    agents: [...settings.agents],
    acp_options: Object.fromEntries(
      Object.entries(settings.acp_options ?? {}).map(([agent, value]) => [
        agent,
        { reasoning_efforts: [...value.reasoning_efforts] },
      ]),
    ),
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
  const [loginJobs, setLoginJobs] = useState<Record<string, ACPAuthLogin>>({})

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

  const login = useMutation({
    mutationFn: ({ agent, auth }: { agent: string; auth?: AgentSettingsData['acp'][string]['auth'] }) =>
      startACPAuthLogin(agent, auth),
    onSuccess: (job) => {
      setLoginJobs((current) => ({ ...current, [job.agent]: job }))
      toast(`Started ${agentLabel(job.agent)} sign-in`)
    },
    onError: (error: Error) => toast(`Couldn't start sign-in: ${error.message}`, 'danger'),
  })

  useEffect(() => {
    const running = Object.values(loginJobs).filter((job) => job.status === 'running')
    if (running.length === 0) return
    const timer = window.setInterval(() => {
      for (const job of running) {
        void getACPAuthLogin(job.id).then((next) => {
          setLoginJobs((current) => ({ ...current, [next.agent]: next }))
          if (next.status !== 'running') {
            queryClient.invalidateQueries({ queryKey: keys.agentSettings })
            queryClient.invalidateQueries({ queryKey: keys.acpAgents })
          }
        })
      }
    }, 1000)
    return () => window.clearInterval(timer)
  }, [loginJobs, queryClient])

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

            <div>
              <p className="pb-2 text-[12px] font-medium text-ink-2">ACP</p>
              <div className="flex flex-col gap-3">
                {draft.agents.map((agent) => (
                  <ACPAgentRow
                    key={agent}
                    agent={agent}
                    settings={draft}
                    disabled={save.isPending}
                    loginJob={loginJobs[agent]}
                    loginPending={login.isPending && login.variables?.agent === agent}
                    onStartLogin={(auth) => login.mutate({ agent, auth })}
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
  loginJob,
  loginPending,
  onStartLogin,
  onChange,
}: {
  agent: string
  settings: AgentSettingsData
  disabled: boolean
  loginJob?: ACPAuthLogin
  loginPending: boolean
  onStartLogin: (auth: ACPAuthDraft) => void
  onChange: (settings: AgentSettingsData) => void
}) {
  const current = settings.acp[agent] ?? {
    enabled: false,
    command: '',
    model: '',
    reasoning_effort: '',
    auth: { mode: 'auto', path: '' },
  }
  const authStatus = settings.acp_auth?.[agent]
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

      <div className="border-t border-border/70 px-3 py-3">
        <AgentAuthPanel
          agent={agent}
          disabled={controlsDisabled}
          authMode={authModeValue(agent, current.auth?.mode)}
          authPath={agent === 'grok' ? '' : (current.auth?.path ?? '')}
          status={authStatus}
          apiKeyValue={settings.acp_keys?.[agent] ?? ''}
          loginJob={loginJob}
          loginPending={loginPending}
          onAuthModeChange={(mode) => update({ auth: { mode, path: '' } })}
          onStartLogin={() => onStartLogin(current.auth)}
          onAPIKeyChange={(value) =>
            onChange({
              ...settings,
              acp_keys: {
                ...(settings.acp_keys ?? {}),
                [agent]: value,
              },
            })
          }
        />
      </div>

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
          options={settingsReasoningOptions(acpReasoningEffortOptions(settings, agent))}
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

function AgentAuthPanel({
  agent,
  disabled,
  authMode,
  authPath,
  status,
  apiKeyValue,
  loginJob,
  loginPending,
  onAuthModeChange,
  onStartLogin,
  onAPIKeyChange,
}: {
  agent: string
  disabled: boolean
  authMode: 'auto' | 'existing_cli' | 'jaz_profile'
  authPath: string
  status?: ACPAgentAuthStatus
  apiKeyValue: string
  loginJob?: ACPAuthLogin
  loginPending: boolean
  onAuthModeChange: (mode: 'auto' | 'existing_cli' | 'jaz_profile') => void
  onStartLogin: () => void
  onAPIKeyChange: (value: string) => void
}) {
  const apiKeyEnv = status?.api_key?.source_env
  const hasDraftKey = apiKeyValue.trim().length > 0
  const [method, setMethod] = useState<'login' | 'key'>(
    hasDraftKey || status?.api_key_configured ? 'key' : 'login',
  )
  const running = loginPending || loginJob?.status === 'running'
  return (
    <div className="grid gap-3 rounded-[12px] bg-bg p-2.5 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div className="min-w-0">
          <p className="text-[13px] font-medium text-ink">Auth</p>
          <p className="mt-0.5 text-[12px] text-ink-3">{authStatusText(status)}</p>
        </div>
        <div className="flex rounded-full bg-surface p-1">
          <Button size="sm" active={method === 'login'} disabled={disabled} onClick={() => setMethod('login')}>
            <LogIn size={13} />
            Auth login
          </Button>
          <Button size="sm" active={method === 'key'} disabled={disabled || !apiKeyEnv} onClick={() => setMethod('key')}>
            <KeyRound size={13} />
            API key
          </Button>
        </div>
      </div>

      <div className="grid gap-2 md:grid-cols-2">
        <AuthStatusPill
          icon={<ShieldCheck size={13} />}
          label={status?.auth_kind === 'oauth' ? 'Account auth' : 'Account'}
          active={status?.auth_kind === 'oauth'}
          detail={authEvidenceText(status)}
        />
        <AuthStatusPill
          icon={<KeyRound size={13} />}
          label="API key"
          active={Boolean(status?.api_key_configured || hasDraftKey)}
          detail={hasDraftKey ? 'Ready to save' : status?.api_key_configured ? 'Configured' : apiKeyEnv || 'Not configured'}
        />
      </div>

      {method === 'login' ? (
        <div className="grid gap-2">
          <div className="flex flex-wrap gap-1">
            {authModeOptions(agent).map((option) => (
              <Button
                key={option.value}
                size="sm"
                active={authMode === option.value}
                disabled={disabled}
                onClick={() => onAuthModeChange(option.value as 'auto' | 'existing_cli' | 'jaz_profile')}
                title={option.description}
              >
                {option.label}
              </Button>
            ))}
          </div>
          <Button
            variant="primary"
            size="md"
            disabled={disabled || running}
            onClick={onStartLogin}
            className="w-full"
          >
            {running ? <LoaderCircle size={14} className="animate-spin" /> : <LogIn size={14} />}
            {running ? 'Waiting for sign-in...' : `Sign in with ${agentLabel(agent)}`}
          </Button>
          {loginJob?.output ? (
            <pre className="max-h-32 overflow-auto whitespace-pre-wrap rounded-[8px] bg-surface px-3 py-2 font-mono text-[11px] leading-relaxed text-ink-2">
              {loginJob.output}
            </pre>
          ) : null}
          {loginJob?.status === 'failed' && loginJob.error ? (
            <p className="text-[12px] text-danger">{loginJob.error}</p>
          ) : null}
        </div>
      ) : apiKeyEnv ? (
        <div className="grid gap-2">
          <Input
            type="password"
            value={apiKeyValue}
            disabled={disabled}
            onChange={(event) => onAPIKeyChange(event.target.value)}
            placeholder={status?.api_key_configured ? `${apiKeyEnv} configured` : apiKeyEnv}
            autoComplete="off"
            spellCheck={false}
            className="h-8 rounded-full bg-surface px-3 py-0 text-[12px]"
            aria-label={`${agentLabel(agent)} API key fallback`}
          />
          <p className="px-1 text-[12px] text-ink-3">
            Stored as {apiKeyEnv}; passed to the agent as {status?.api_key?.target_env || 'provider key'}.
          </p>
        </div>
      ) : null}

      {authPath ? (
        <p className="truncate px-1 font-mono text-[11px] text-ink-3">{authPath}</p>
      ) : null}
    </div>
  )
}

function AuthStatusPill({
  icon,
  label,
  detail,
  active,
}: {
  icon: ReactNode
  label: string
  detail: string
  active: boolean
}) {
  return (
    <div
      className={`flex min-h-10 items-center gap-2 rounded-full px-3 text-[12px] ${
        active ? 'bg-primary-soft text-primary-strong' : 'bg-surface text-ink-3'
      }`}
    >
      <span className="shrink-0">{icon}</span>
      <span className="min-w-0 flex-1 truncate">
        <span className="font-medium">{label}</span>
        <span className="ml-1 text-ink-3">{detail}</span>
      </span>
    </div>
  )
}

function authStatusText(status?: ACPAgentAuthStatus): string {
  if (!status) return 'Checked on the backend machine.'
  if (status.auth_kind === 'oauth') return 'Account login is active.'
  if (status.auth_kind === 'api_key') return 'Using explicit API key fallback.'
  return status.reason || 'No credential detected.'
}

function authEvidenceText(status?: ACPAgentAuthStatus): string {
  if (!status?.authenticated) return 'Needs login'
  if (status.auth_evidence === 'keyring_config') return 'Keychain'
  if (status.auth_evidence === 'auth_json') return 'auth.json'
  if (status.auth_evidence === 'credentials_json') return 'credentials'
  if (status.auth_evidence === 'env') return 'environment'
  if (status.auth_evidence === 'api_key_env') return 'fallback'
  return 'Ready'
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
