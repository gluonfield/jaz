import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  ChevronDown,
  KeyRound,
  LoaderCircle,
  LogIn,
  RefreshCw,
  Server,
} from 'lucide-react'
import { motion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { completeOnboarding, onboardingQuery } from '@/lib/api/onboarding'
import { getACPAuthLogin, startACPAuthLogin } from '@/lib/api/settings'
import type { ACPAgentAuth, ACPAuthLogin, AgentSettings, OnboardingACPProbe, OnboardingStatus } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

const EASE = [0.22, 1, 0.36, 1] as const

export function OnboardingGate({ children }: { children: ReactNode }) {
  const onboarding = useQuery(onboardingQuery)

  if (window.jaz?.windowKind === 'board') return <>{children}</>
  if (onboarding.isPending) return <OnboardingShell><SkeletonRows count={4} /></OnboardingShell>
  if (onboarding.isError) {
    return (
      <OnboardingShell>
        <StatusBlock
          icon={<AlertCircle size={16} />}
          title="Couldn't load onboarding"
          text={onboarding.error.message}
        />
      </OnboardingShell>
    )
  }
  if (onboarding.data.completed) return <>{children}</>
  return <OnboardingScreen status={onboarding.data} onRefresh={() => void onboarding.refetch()} />
}

function OnboardingScreen({ status, onRefresh }: { status: OnboardingStatus; onRefresh: () => void }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState(() => draftFromStatus(status))
  const [keysByProvider, setKeysByProvider] = useState<Record<string, string>>({})
  const [acpKeysByAgent, setACPKeysByAgent] = useState<Record<string, string>>({})
  const [openAgent, setOpenAgent] = useState(status.acp[0]?.agent ?? '')
  const [loginJobs, setLoginJobs] = useState<Record<string, ACPAuthLogin>>({})

  useEffect(() => {
    setDraft(draftFromStatus(status))
    setOpenAgent((current) => current || status.acp[0]?.agent || '')
  }, [status])

  const providerStatus = useMemo(
    () => new Map(status.native_providers.map((provider) => [provider.id, provider])),
    [status.native_providers],
  )
  const selectedProvider = draft.native.model_provider || draft.providers[0]?.id || ''
  const selectedProviderStatus = providerStatus.get(selectedProvider)
  const selectedProviderKey = keysByProvider[selectedProvider]?.trim() ?? ''
  const acpStatus = useMemo(() => new Map(status.acp.map((probe) => [probe.agent, probe])), [status.acp])
  const acpEnabled = draft.agents.some((agent) => {
    if (!draft.acp[agent]?.enabled) return false
    const probe = acpStatus.get(agent)
    return Boolean(probe?.available || acpKeysByAgent[agent]?.trim())
  })
  const nativeReady = Boolean(selectedProviderStatus?.configured || selectedProviderKey)
  const canFinish = acpEnabled || nativeReady

  const login = useMutation({
    mutationFn: ({ agent, auth }: { agent: string; auth?: ACPAgentAuth }) => startACPAuthLogin(agent, auth),
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
            queryClient.invalidateQueries({ queryKey: keys.onboarding })
            queryClient.invalidateQueries({ queryKey: keys.agentSettings })
            queryClient.invalidateQueries({ queryKey: keys.acpAgents })
          }
        })
      }
    }, 1000)
    return () => window.clearInterval(timer)
  }, [loginJobs, queryClient])

  const save = useMutation({
    mutationFn: () =>
      completeOnboarding({
        settings: draft,
        provider_keys: selectedProviderKey ? { [selectedProvider]: selectedProviderKey } : undefined,
        acp_keys: compactSecrets(acpKeysByAgent),
        completed: true,
      }),
    onSuccess: (saved) => {
      queryClient.setQueryData(keys.onboarding, saved)
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  const setProvider = (model_provider: string) => {
    const next = draft.providers.find((provider) => provider.id === model_provider)
    const current = draft.providers.find((provider) => provider.id === draft.native.model_provider)
    const model =
      draft.native.model.trim() === '' || draft.native.model === current?.default_model
        ? next?.default_model || draft.native.model
        : draft.native.model
    setDraft({ ...draft, native: { ...draft.native, model_provider, model } })
  }

  return (
    <OnboardingShell>
      <motion.div
        initial={{ opacity: 0, y: 10, filter: 'blur(6px)' }}
        animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
        transition={{ duration: 0.45, ease: EASE }}
        className="min-w-0 w-full max-w-[calc(100vw-40px)] md:max-w-[640px]"
      >
        <div className="mb-5">
          <h1 className="text-balance text-[22px] font-semibold text-ink">Connect Jaz to its agents</h1>
          <p className="mt-2 max-w-[620px] text-pretty text-[13px] text-ink-3">
            Agent credentials are checked on the backend machine. A remote backend needs its own
            Codex, Claude, and Grok sign-ins.
          </p>
        </div>

        <div className="overflow-hidden rounded-[14px] bg-surface/85 p-1 shadow-[0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent),0_18px_60px_rgba(0,0,0,0.10)] backdrop-blur-[2px]">
          <StepRow icon={<Server size={16} />} title="Backend" detail="Connected">
            <CheckCircle2 size={17} className="text-primary" />
          </StepRow>

          <div className="border-t border-border/70 px-3 py-3">
            <div className="mb-3 flex items-center gap-2">
              <Bot size={16} className="text-ink-3" />
              <p className="text-[13px] font-medium text-ink">ACP clients</p>
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto"
                onClick={onRefresh}
                aria-label="Refresh agent status"
                title="Refresh agent status"
              >
                <RefreshCw size={13} />
                Refresh
              </Button>
            </div>
            <div className="grid gap-2">
              {status.acp.map((probe) => (
                <AgentToggle
                  key={probe.agent}
                  probe={probe}
                  enabled={Boolean(draft.acp[probe.agent]?.enabled)}
                  open={openAgent === probe.agent}
                  auth={draft.acp[probe.agent]?.auth}
                  apiKeyValue={acpKeysByAgent[probe.agent] ?? ''}
                  loginJob={loginJobs[probe.agent]}
                  loginPending={login.isPending && login.variables?.agent === probe.agent}
                  onOpenChange={() => setOpenAgent((current) => current === probe.agent ? '' : probe.agent)}
                  onStartLogin={() =>
                    login.mutate({ agent: probe.agent, auth: draft.acp[probe.agent]?.auth })
                  }
                  onAPIKeyChange={(value) =>
                    setACPKeysByAgent({ ...acpKeysByAgent, [probe.agent]: value })
                  }
                  onChange={(enabled) =>
                    setDraft({
                      ...draft,
                      acp: {
                        ...draft.acp,
                        [probe.agent]: { ...draft.acp[probe.agent], enabled },
                      },
                    })
                  }
                />
              ))}
            </div>
          </div>

          <div className="border-t border-border/70 px-3 py-3">
            <div className="mb-3 flex items-center gap-2">
              <KeyRound size={16} className="text-ink-3" />
              <p className="text-[13px] font-medium text-ink">Native Agent</p>
            </div>
            <div className="grid gap-3 md:grid-cols-[220px_minmax(0,1fr)] md:items-start">
              <Select
                value={selectedProvider}
                options={draft.providers.map((provider) => ({
                  value: provider.id,
                  label: provider.label,
                  description: provider.api_key_env,
                }))}
                onChange={setProvider}
                disabled={save.isPending}
                aria-label="Native provider"
                className="h-9"
              />
              {selectedProviderStatus?.configured ? (
                <div className="flex min-h-9 items-center rounded-control bg-primary/10 px-3 text-[13px] text-ink">
                  {selectedProviderStatus.api_key_env} configured
                </div>
              ) : (
                <Input
                  type="password"
                  value={keysByProvider[selectedProvider] ?? ''}
                  onChange={(event) =>
                    setKeysByProvider({ ...keysByProvider, [selectedProvider]: event.target.value })
                  }
                  disabled={save.isPending || !selectedProvider}
                  placeholder={selectedProviderStatus?.api_key_env || 'API key'}
                  autoComplete="off"
                  spellCheck={false}
                />
              )}
            </div>
          </div>
        </div>

        <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="min-h-5 text-[12px] text-ink-3">
            {!canFinish ? 'Sign in to an ACP client on this backend or add a Native Agent key.' : save.error?.message}
          </p>
          <Button
            variant="primary"
            size="lg"
            disabled={!canFinish || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending && <LoaderCircle size={14} className="animate-spin" />}
            Finish setup
          </Button>
        </div>
      </motion.div>
    </OnboardingShell>
  )
}

function AgentToggle({
  probe,
  enabled,
  open,
  auth,
  apiKeyValue,
  loginJob,
  loginPending,
  onOpenChange,
  onStartLogin,
  onAPIKeyChange,
  onChange,
}: {
  probe: OnboardingACPProbe
  enabled: boolean
  open: boolean
  auth?: ACPAgentAuth
  apiKeyValue: string
  loginJob?: ACPAuthLogin
  loginPending: boolean
  onOpenChange: () => void
  onStartLogin: () => void
  onAPIKeyChange: (value: string) => void
  onChange: (enabled: boolean) => void
}) {
  const status = agentStatusText(probe)
  const apiKeyEnv = probe.api_key?.source_env
  const apiKeyReady = Boolean(probe.api_key_configured || apiKeyValue.trim())
  const running = loginPending || loginJob?.status === 'running'
  const [method, setMethod] = useState<'login' | 'key'>(apiKeyReady ? 'key' : 'login')
  const ready = Boolean(probe.available || apiKeyReady)
  return (
    <div className="overflow-hidden rounded-[12px] bg-bg shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
      <button
        type="button"
        aria-expanded={open}
        onClick={onOpenChange}
        className="flex min-h-[58px] w-full items-center gap-3 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-surface/70"
      >
        <span className={`size-2 rounded-full ${ready ? 'bg-primary' : 'bg-ink/30'}`} />
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-[13px] font-medium text-ink">{agentLabel(probe.agent)}</span>
            <span className={`text-[12px] ${ready ? 'text-primary' : 'text-ink-3'}`}>{status}</span>
          </span>
          <span className="mt-0.5 block truncate text-[12px] text-ink-3">
            {probe.authenticated ? authReadyText(probe) : probe.reason || authLoginHint(probe, auth)}
          </span>
        </span>
        <span
          className="flex items-center gap-2"
          onClick={(event) => event.stopPropagation()}
        >
          <Switch
            checked={enabled}
            disabled={!probe.installed || !ready}
            onChange={onChange}
            aria-label={`Enable ${agentLabel(probe.agent)}`}
          />
          <ChevronDown
            size={15}
            className={`text-ink-3 transition-transform duration-150 ${open ? 'rotate-180' : ''}`}
          />
        </span>
      </button>

      {open ? (
        <motion.div
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={{ duration: 0.16, ease: EASE }}
          className="border-t border-border/70 px-3 pb-3 pt-2.5"
        >
          <div className="mb-3 flex rounded-full bg-surface p-1">
            <Button
              size="sm"
              active={method === 'login'}
              className="flex-1"
              onClick={() => setMethod('login')}
            >
              <LogIn size={13} />
              Auth login
            </Button>
            <Button
              size="sm"
              active={method === 'key'}
              className="flex-1"
              disabled={!apiKeyEnv}
              onClick={() => setMethod('key')}
            >
              <KeyRound size={13} />
              API key
            </Button>
          </div>

          {method === 'login' ? (
            <div className="grid gap-2">
              <p className="text-pretty text-[12px] text-ink-3">{authLoginHint(probe, auth)}</p>
              <Button
                variant="primary"
                size="md"
                disabled={!probe.auth_command_available || running}
                onClick={onStartLogin}
                className="w-full"
              >
                {running ? <LoaderCircle size={14} className="animate-spin" /> : <LogIn size={14} />}
                {running ? 'Waiting for sign-in...' : `Sign in with ${agentLabel(probe.agent)}`}
              </Button>
              {!probe.auth_command_available && probe.auth_command_reason ? (
                <p className="text-[12px] text-danger">{probe.auth_command_reason}</p>
              ) : null}
              <AuthLoginStatus job={loginJob} running={running} />
            </div>
          ) : (
            <div className="grid gap-2">
              <p className="text-pretty text-[12px] text-ink-3">
                Use this only when you want Jaz to pass an explicit provider key to this agent.
              </p>
              <Input
                type="password"
                value={apiKeyValue}
                onChange={(event) => onAPIKeyChange(event.target.value)}
                placeholder={probe.api_key_configured ? `${apiKeyEnv} configured` : apiKeyEnv || 'API key'}
                autoComplete="off"
                spellCheck={false}
                className="h-9 rounded-full bg-surface px-3 py-0 text-[12px]"
                aria-label={`${agentLabel(probe.agent)} API key fallback`}
              />
              <span
                className={`inline-flex h-7 w-fit items-center rounded-full px-2.5 text-[12px] ${
                  apiKeyReady ? 'bg-primary-soft text-primary-strong' : 'bg-surface text-ink-3'
                }`}
              >
                {apiKeyReady ? 'API key ready' : apiKeyEnv || 'No API key option'}
              </span>
            </div>
          )}
        </motion.div>
      ) : null}
    </div>
  )
}

function agentStatusText(probe: OnboardingACPProbe): string {
  if (!probe.installed) return 'Missing'
  if (!probe.authenticated) return 'Needs sign-in'
  if (!probe.available) return 'Needs setup'
  return 'Ready'
}

function authReadyText(probe: OnboardingACPProbe): string {
  if (probe.auth_kind === 'api_key') return 'Using explicit API key fallback.'
  if (probe.refresh_owner === 'coding_agent_cli') return 'The coding agent owns token refresh.'
  return 'Signed in.'
}

function authLoginHint(probe: OnboardingACPProbe, auth?: ACPAgentAuth): string {
  if (probe.agent === 'grok') return "Jaz runs Grok's normal login on this backend."
  if (auth?.mode === 'existing_cli') return `Jaz runs ${agentLabel(probe.agent)} login against the backend user profile.`
  return `Jaz runs ${agentLabel(probe.agent)} login and stores it for this backend.`
}

function StepRow({ icon, title, detail, children }: { icon: ReactNode; title: string; detail: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-3 py-3">
      <span className="grid size-8 place-items-center rounded-full bg-bg text-ink-3 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">{icon}</span>
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        <p className="mt-0.5 text-[12px] text-ink-3">{detail}</p>
      </div>
      {children}
    </div>
  )
}

function StatusBlock({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <div className="mx-auto flex w-full max-w-[420px] items-start gap-3 rounded-card bg-surface p-4 text-ink">
      <span className="mt-0.5 text-danger">{icon}</span>
      <div>
        <p className="text-[13px] font-medium">{title}</p>
        <p className="mt-1 text-[12px] text-ink-3">{text}</p>
      </div>
    </div>
  )
}

function OnboardingShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full flex-col bg-bg">
      <div className="titlebar-drag h-[52px] shrink-0" />
      <main className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-5 pb-[52px]">
        <div className="flex min-h-full w-full items-start justify-center py-6 md:py-8">
          {children}
        </div>
      </main>
    </div>
  )
}

function draftFromStatus(status: OnboardingStatus): AgentSettings {
  const settings = cloneSettings(status.settings)
  for (const probe of status.acp) {
    const current = settings.acp[probe.agent]
    settings.acp[probe.agent] = {
      ...current,
      auth: onboardingAuth(current?.auth, probe.recommended_auth),
      enabled: probe.available && Boolean(settings.acp[probe.agent]?.enabled ?? true),
    }
  }
  return settings
}

function onboardingAuth(current?: ACPAgentAuth, recommended?: ACPAgentAuth): ACPAgentAuth {
  if (current?.mode && current.mode !== 'auto') return current
  return {
    mode: recommended?.mode || current?.mode || 'auto',
    path: recommended?.path ?? current?.path ?? '',
  }
}

function cloneSettings(settings: AgentSettings): AgentSettings {
  return {
    native: { ...settings.native },
    providers: [...(settings.providers ?? [])],
    acp_auth: { ...(settings.acp_auth ?? {}) },
    acp_keys: { ...(settings.acp_keys ?? {}) },
    acp: Object.fromEntries(
      Object.entries(settings.acp ?? {}).map(([agent, value]) => [
        agent,
        { ...value, auth: value.auth ? { ...value.auth } : undefined },
      ]),
    ),
    agents: [...(settings.agents ?? [])],
  }
}

function compactSecrets(values: Record<string, string>): Record<string, string> | undefined {
  const out = Object.fromEntries(
    Object.entries(values)
      .map(([key, value]) => [key, value.trim()] as const)
      .filter(([, value]) => value.length > 0),
  )
  return Object.keys(out).length > 0 ? out : undefined
}
