import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  Check,
  CheckCircle2,
  ChevronDown,
  ExternalLink,
  KeyRound,
  Laptop,
  LoaderCircle,
  LogIn,
  Server,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { completeOnboarding, onboardingQuery } from '@/lib/api/onboarding'
import { getACPAuthLogin, startACPAuthLogin } from '@/lib/api/settings'
import type { ACPAgentAuth, ACPAuthLogin, AgentSettings, OnboardingACPProbe, OnboardingStatus } from '@/lib/api/types'
import { isLoopbackUrl, useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'

const EASE = [0.22, 1, 0.36, 1] as const

const stagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.08 } },
}

const rise = {
  hidden: { opacity: 0, y: 12, filter: 'blur(5px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.42, ease: EASE } },
}

// Where to grab a key for each native provider — the answer to "where does the
// key even come from?". Keyed by the backend provider id.
const PROVIDER_KEY_URLS: Record<string, string> = {
  openrouter: 'https://openrouter.ai/keys',
  openai: 'https://platform.openai.com/api-keys',
}

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
  const connection = useConnection()
  const remote = !isLoopbackUrl(connection.url)
  const [draft, setDraft] = useState(() => draftFromStatus(status))
  const [keysByProvider, setKeysByProvider] = useState<Record<string, string>>({})
  const [acpKeysByAgent, setACPKeysByAgent] = useState<Record<string, string>>({})
  const [loginJobs, setLoginJobs] = useState<Record<string, ACPAuthLogin>>({})

  useEffect(() => {
    setDraft(draftFromStatus(status))
  }, [status])

  const providerStatus = useMemo(
    () => new Map(status.native_providers.map((provider) => [provider.id, provider])),
    [status.native_providers],
  )
  const selectedProvider = draft.native.model_provider || draft.providers[0]?.id || ''
  const selectedProviderStatus = providerStatus.get(selectedProvider)
  const selectedProviderKey = keysByProvider[selectedProvider]?.trim() ?? ''
  const nativeReady = Boolean(selectedProviderStatus?.configured || selectedProviderKey)

  const readyAgents = useMemo(
    () =>
      new Set(
        status.acp
          .filter((probe) => probe.available || acpKeysByAgent[probe.agent]?.trim())
          .map((probe) => probe.agent),
      ),
    [status.acp, acpKeysByAgent],
  )
  const canFinish = readyAgents.size > 0 || nativeReady

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
    mutationFn: () => {
      // No enable toggles in onboarding: every agent that ended up with a
      // credential turns on; the rest stay off. Users refine this in Settings.
      const next = cloneSettings(draft)
      for (const probe of status.acp) {
        next.acp[probe.agent] = {
          ...next.acp[probe.agent],
          enabled: readyAgents.has(probe.agent),
        }
      }
      return completeOnboarding({
        settings: next,
        provider_keys: selectedProviderKey ? { [selectedProvider]: selectedProviderKey } : undefined,
        acp_keys: compactSecrets(acpKeysByAgent),
        completed: true,
      })
    },
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
        variants={stagger}
        initial="hidden"
        animate="show"
        className="min-w-0 w-full max-w-[calc(100vw-40px)] md:max-w-[560px]"
      >
        <motion.div variants={rise} className="mb-5">
          <BackendChip remote={remote} url={connection.url} />
          <h1 className="mt-3 text-balance text-[22px] font-semibold tracking-tight text-ink">
            Connect your agents
          </h1>
          <p className="mt-2 text-pretty text-[13px] text-ink-2">
            {remote
              ? 'Credentials live on your server. Sign in to a coding agent there, or give jaz its own provider key — whichever you add turns on automatically.'
              : 'Sign in to a coding agent on this Mac, or give jaz its own provider key. Whatever you add turns on automatically.'}
          </p>
        </motion.div>

        <motion.div variants={rise}>
          <SectionLabel>Coding agents</SectionLabel>
          <div className="grid gap-2.5">
            {status.acp.map((probe) => (
              <AgentCard
                key={probe.agent}
                probe={probe}
                auth={draft.acp[probe.agent]?.auth}
                apiKeyValue={acpKeysByAgent[probe.agent] ?? ''}
                loginJob={loginJobs[probe.agent]}
                loginPending={login.isPending && login.variables?.agent === probe.agent}
                onStartLogin={() => login.mutate({ agent: probe.agent, auth: draft.acp[probe.agent]?.auth })}
                onAPIKeyChange={(value) => setACPKeysByAgent({ ...acpKeysByAgent, [probe.agent]: value })}
              />
            ))}
          </div>
        </motion.div>

        <motion.div variants={rise} className="mt-5">
          <SectionLabel>Native agent</SectionLabel>
          <NativeAgentCard
            providers={draft.providers}
            selectedProvider={selectedProvider}
            configured={Boolean(selectedProviderStatus?.configured)}
            apiKeyEnv={selectedProviderStatus?.api_key_env}
            apiKeyValue={keysByProvider[selectedProvider] ?? ''}
            remote={remote}
            disabled={save.isPending}
            onProviderChange={setProvider}
            onAPIKeyChange={(value) => setKeysByProvider({ ...keysByProvider, [selectedProvider]: value })}
          />
        </motion.div>

        <motion.div
          variants={rise}
          className="mt-5 flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between"
        >
          <p className="min-h-5 text-pretty text-[12px] text-ink-3">
            {!canFinish
              ? 'Add one coding agent or a native key to continue.'
              : save.error
                ? save.error.message
                : `${summary(readyAgents.size, nativeReady)} ready.`}
          </p>
          <div className="flex items-center gap-1.5">
            <Button variant="ghost" size="lg" onClick={onRefresh} title="Re-check agent status">
              Refresh
            </Button>
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
      </motion.div>
    </OnboardingShell>
  )
}

type AgentState = 'ready' | 'action' | 'missing'

function agentState(probe: OnboardingACPProbe, keyDraft: string): AgentState {
  if (probe.available || keyDraft.trim()) return 'ready'
  if (!probe.installed) return 'missing'
  return 'action'
}

function AgentCard({
  probe,
  auth,
  apiKeyValue,
  loginJob,
  loginPending,
  onStartLogin,
  onAPIKeyChange,
}: {
  probe: OnboardingACPProbe
  auth?: ACPAgentAuth
  apiKeyValue: string
  loginJob?: ACPAuthLogin
  loginPending: boolean
  onStartLogin: () => void
  onAPIKeyChange: (value: string) => void
}) {
  const reducedMotion = useReducedMotion()
  const apiKeyEnv = probe.api_key?.source_env
  const apiKeyReady = Boolean(probe.api_key_configured || apiKeyValue.trim())
  const state = agentState(probe, apiKeyValue)
  const running = loginPending || loginJob?.status === 'running'
  const canLogin = state !== 'missing'
  const canKey = Boolean(apiKeyEnv)
  // A ready agent collapses to a confirmation row; an actionable one opens to
  // its sign-in / key controls. "missing" can't act, so it never opens.
  const [expanded, setExpanded] = useState(state === 'action')
  const [method, setMethod] = useState<'login' | 'key'>(apiKeyReady && canKey ? 'key' : 'login')

  const monogram = agentLabel(probe.agent).charAt(0).toUpperCase()
  const headline = state === 'ready' ? readyHeadline(probe, apiKeyValue) : agentStateLabel(state)
  const subtext =
    state === 'ready'
      ? readySubtext(probe, apiKeyValue)
      : state === 'missing'
        ? probe.reason || 'Install this agent’s CLI to use it.'
        : probe.reason || 'Sign in once and jaz keeps using it.'

  return (
    <div className="relative">
      {/* live state: a rainbow comet circles the card while a sign-in runs,
          the same vocabulary the composer uses while jaz is alive */}
      <AnimatePresence>
        {running ? (
          <motion.div
            aria-hidden
            className="pointer-events-none absolute -inset-[1.5px]"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1, ...(reducedMotion ? {} : { '--ring-angle': ['0deg', '360deg'] }) }}
            exit={{ opacity: 0 }}
            transition={{
              opacity: { duration: 0.25, ease: 'easeOut' },
              '--ring-angle': { duration: 2.6, ease: 'linear', repeat: Infinity },
            }}
          >
            <div className="absolute inset-0 rounded-[16px]" style={{ background: RAINBOW_BEAM }} />
          </motion.div>
        ) : null}
      </AnimatePresence>

      <div
        className={`relative overflow-hidden rounded-[16px] bg-surface transition-shadow duration-150 ${
          running ? '' : 'shadow-[0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]'
        }`}
      >
        <button
          type="button"
          aria-expanded={expanded}
          disabled={state === 'missing'}
          onClick={() => setExpanded((open) => !open)}
          className="flex w-full items-center gap-3 px-3 py-3 text-left transition-colors duration-150 enabled:hover:bg-surface-2/60 disabled:cursor-default"
        >
          <span
            className={`grid size-10 shrink-0 place-items-center rounded-full text-[15px] font-semibold ${
              state === 'ready'
                ? 'bg-primary-soft text-primary-strong'
                : state === 'missing'
                  ? 'bg-bg text-ink-3 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]'
                  : 'bg-bg text-ink-2 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]'
            }`}
          >
            {monogram}
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate text-[14px] font-medium text-ink">{agentLabel(probe.agent)}</span>
              <StatePill state={state} label={headline} />
            </span>
            <span className="mt-0.5 block truncate text-[12px] text-ink-3">{subtext}</span>
          </span>
          {state === 'ready' ? (
            <CheckCircle2 size={18} className="shrink-0 text-primary" />
          ) : state === 'action' ? (
            <ChevronDown
              size={16}
              className={`shrink-0 text-ink-3 transition-transform duration-150 ${expanded ? 'rotate-180' : ''}`}
            />
          ) : null}
        </button>

        <AnimatePresence initial={false}>
          {expanded && state !== 'missing' ? (
            <motion.div
              key="body"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.18, ease: EASE }}
              className="overflow-hidden"
            >
              <div className="border-t border-border/70 px-3 pb-3 pt-3">
                {canLogin && canKey ? (
                  <div className="mb-3 flex gap-1 rounded-full bg-bg p-1 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
                    <Button size="sm" active={method === 'login'} className="flex-1" onClick={() => setMethod('login')}>
                      <LogIn size={13} />
                      Sign in
                    </Button>
                    <Button size="sm" active={method === 'key'} className="flex-1" onClick={() => setMethod('key')}>
                      <KeyRound size={13} />
                      API key
                    </Button>
                  </div>
                ) : null}

                {(method === 'login' || !canKey) && canLogin ? (
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
                      {running ? 'Waiting for sign-in…' : `Sign in with ${agentLabel(probe.agent)}`}
                    </Button>
                    {!probe.auth_command_available && probe.auth_command_reason ? (
                      <p className="text-[12px] text-danger">{probe.auth_command_reason}</p>
                    ) : null}
                    <AuthLoginStatus job={loginJob} running={running} />
                  </div>
                ) : canKey ? (
                  <div className="grid gap-2">
                    <p className="text-pretty text-[12px] text-ink-3">
                      Paste a provider key and jaz passes it to {agentLabel(probe.agent)} directly.
                    </p>
                    <Input
                      type="password"
                      value={apiKeyValue}
                      onChange={(event) => onAPIKeyChange(event.target.value)}
                      placeholder={probe.api_key_configured ? `${apiKeyEnv} already set` : apiKeyEnv || 'API key'}
                      autoComplete="off"
                      spellCheck={false}
                      className="font-mono text-[12px]"
                      aria-label={`${agentLabel(probe.agent)} API key`}
                    />
                    <p className="text-[12px] text-ink-3">Stored on the backend as {apiKeyEnv}.</p>
                  </div>
                ) : null}
              </div>
            </motion.div>
          ) : null}
        </AnimatePresence>
      </div>
    </div>
  )
}

function NativeAgentCard({
  providers,
  selectedProvider,
  configured,
  apiKeyEnv,
  apiKeyValue,
  remote,
  disabled,
  onProviderChange,
  onAPIKeyChange,
}: {
  providers: AgentSettings['providers']
  selectedProvider: string
  configured: boolean
  apiKeyEnv?: string
  apiKeyValue: string
  remote: boolean
  disabled: boolean
  onProviderChange: (value: string) => void
  onAPIKeyChange: (value: string) => void
}) {
  const provider = providers.find((item) => item.id === selectedProvider)
  const label = provider?.label || 'your provider'
  const keyUrl = PROVIDER_KEY_URLS[selectedProvider]
  const ready = configured || apiKeyValue.trim().length > 0

  return (
    <div className="overflow-hidden rounded-[16px] bg-surface shadow-[0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
      <div className="flex items-center gap-3 px-3 py-3">
        <span
          className={`grid size-10 shrink-0 place-items-center rounded-full ${
            ready ? 'bg-primary-soft text-primary-strong' : 'bg-bg text-ink-2 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]'
          }`}
        >
          <Laptop size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <p className="truncate text-[14px] font-medium text-ink">jaz native</p>
            {ready ? <StatePill state="ready" label={configured ? 'Connected' : 'Key added'} /> : null}
          </div>
          <p className="mt-0.5 text-pretty text-[12px] text-ink-3">
            jaz’s own agent calls {label} directly with an API key you provide.
          </p>
        </div>
        {ready ? <CheckCircle2 size={18} className="shrink-0 text-primary" /> : null}
      </div>

      <div className="grid gap-3 border-t border-border/70 px-3 py-3">
        <div className="grid gap-2 md:grid-cols-[150px_minmax(0,1fr)] md:items-center">
          <span className="text-[12px] font-medium text-ink-2">Provider</span>
          <Select
            value={selectedProvider}
            options={providers.map((item) => ({
              value: item.id,
              label: item.label,
              description: item.api_key_env,
            }))}
            onChange={onProviderChange}
            disabled={disabled}
            aria-label="Native provider"
            className="h-9"
          />
        </div>

        {configured ? (
          <div className="flex min-h-9 items-center gap-2 rounded-[12px] bg-primary/10 px-3 text-[13px] text-ink">
            <Check size={15} className="shrink-0 text-primary" />
            {apiKeyEnv} is already configured on the backend.
          </div>
        ) : (
          <div className="grid gap-1.5">
            <div className="flex items-center justify-between gap-2">
              <span className="text-[12px] font-medium text-ink-2">Paste your {label} API key</span>
              {keyUrl ? (
                <button
                  type="button"
                  onClick={() => window.open(keyUrl, '_blank', 'noopener,noreferrer')}
                  className="inline-flex items-center gap-1 text-[12px] text-primary transition-colors duration-150 hover:text-primary-strong"
                >
                  Where do I get this?
                  <ExternalLink size={12} />
                </button>
              ) : null}
            </div>
            <Input
              type="password"
              value={apiKeyValue}
              onChange={(event) => onAPIKeyChange(event.target.value)}
              disabled={disabled || !selectedProvider}
              placeholder={apiKeyEnv ? `${apiKeyEnv} (sk-…)` : 'API key'}
              autoComplete="off"
              spellCheck={false}
              className="font-mono text-[12px]"
              aria-label={`${label} API key`}
            />
            <p className="text-[12px] text-ink-3">
              {remote ? 'Stored on your server' : 'Stored on this Mac'} as {apiKeyEnv}. Never leaves the backend.
            </p>
          </div>
        )}
      </div>
    </div>
  )
}

function StatePill({ state, label }: { state: AgentState; label: string }) {
  const tone =
    state === 'ready'
      ? 'bg-primary-soft text-primary-strong'
      : state === 'missing'
        ? 'bg-surface-2 text-ink-3'
        : 'bg-accent-soft text-accent-strong'
  return (
    <span className={`inline-flex shrink-0 items-center rounded-full px-1.5 py-[3px] text-[11px] font-medium ${tone}`}>
      {label}
    </span>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return <p className="mb-2 px-0.5 text-[12px] font-medium text-ink-2">{children}</p>
}

function summary(agentCount: number, nativeReady: boolean): string {
  const parts: string[] = []
  if (agentCount > 0) parts.push(`${agentCount} agent${agentCount === 1 ? '' : 's'}`)
  if (nativeReady) parts.push('native')
  return parts.join(' + ') || 'Nothing'
}

function agentStateLabel(state: AgentState): string {
  if (state === 'missing') return 'Not installed'
  return 'Needs sign-in'
}

function readyHeadline(probe: OnboardingACPProbe, keyDraft: string): string {
  if (!probe.available && keyDraft.trim()) return 'Key added'
  return 'Connected'
}

function readySubtext(probe: OnboardingACPProbe, keyDraft: string): string {
  if (!probe.available && keyDraft.trim()) return `Will pass your key to ${agentLabel(probe.agent)}.`
  if (probe.auth_kind === 'api_key') return 'Using an explicit API key.'
  if (probe.refresh_owner === 'coding_agent_cli') return `${agentLabel(probe.agent)} manages its own token.`
  return 'Signed in and ready.'
}

function authLoginHint(probe: OnboardingACPProbe, auth?: ACPAgentAuth): string {
  if (probe.agent === 'grok') return 'jaz runs Grok’s normal login on this backend.'
  if (auth?.mode === 'existing_cli') return `jaz reuses the ${agentLabel(probe.agent)} login on this backend.`
  return `jaz runs ${agentLabel(probe.agent)} login and stores it for this backend.`
}

function BackendChip({ remote, url }: { remote: boolean; url: string }) {
  const host = (() => {
    try {
      return new URL(url).host
    } catch {
      return url
    }
  })()
  return (
    <span className="inline-flex items-center gap-2 rounded-full bg-surface px-2.5 py-1 text-[12px] text-ink-2 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
      {remote ? <Server size={13} className="text-primary" /> : <Laptop size={13} className="text-primary" />}
      <span className="text-ink">{remote ? 'Connected to server' : 'Running on this Mac'}</span>
      <span className="font-mono text-[11px] text-ink-3">{host}</span>
    </span>
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
