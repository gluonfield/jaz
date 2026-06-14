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
  Lock,
  LogIn,
  Server,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { AgentLogo } from '@/components/acp/AgentLogo'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { authProviderLabel, onboardingAgentLabel } from '@/lib/agentLabel'
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
      toast(`Started ${authProviderLabel(job.agent)} sign-in`)
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
        className="min-w-0 w-full max-w-[calc(100vw-40px)] md:max-w-[460px]"
      >
        <motion.div variants={rise} className="mb-5">
          <BackendChip remote={remote} url={connection.url} />
          <h1 className="mt-3 text-balance text-[20px] font-semibold tracking-tight text-ink">
            Connect your agents
          </h1>
          <p className="mt-1 text-pretty text-[13px] leading-relaxed text-ink-2">
            Sign in to a coding agent, or give jaz its own provider key. Anything you connect turns on
            automatically.
          </p>
        </motion.div>

        <motion.div variants={rise}>
          <SectionLabel>Coding agents</SectionLabel>
          <div className="grid gap-1.5">
            {status.acp.map((probe) => (
              <AgentCard
                key={probe.agent}
                probe={probe}
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
            apiKeyValue={keysByProvider[selectedProvider] ?? ''}
            disabled={save.isPending}
            onProviderChange={setProvider}
            onAPIKeyChange={(value) => setKeysByProvider({ ...keysByProvider, [selectedProvider]: value })}
          />
        </motion.div>

        <motion.div variants={rise} className="mt-4 flex items-center gap-2.5 px-1">
          <Lock size={13} className="shrink-0 text-ink-3" />
          <p className="text-pretty text-[12px] leading-relaxed text-ink-3">
            {remote
              ? 'Your logins and keys are stored on your server and never leave it.'
              : 'Your logins and keys are stored on this Mac and never leave your machine.'}
          </p>
        </motion.div>

        <motion.div
          variants={rise}
          className="mt-5 flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between"
        >
          <p className="min-h-5 text-pretty text-[12px] text-ink-3">
            {!canFinish
              ? 'Connect one coding agent or a native key to continue.'
              : save.error
                ? save.error.message
                : "You're ready — finish whenever you like."}
          </p>
          <div className="flex items-center gap-1">
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
  apiKeyValue,
  loginJob,
  loginPending,
  onStartLogin,
  onAPIKeyChange,
}: {
  probe: OnboardingACPProbe
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
  const canKey = Boolean(apiKeyEnv)
  // Everything starts collapsed; a row only opens when the user taps it.
  const [expanded, setExpanded] = useState(false)
  const [method, setMethod] = useState<'login' | 'key'>(apiKeyReady && canKey ? 'key' : 'login')
  const actionable = state === 'action'

  return (
    <div className="relative">
      {/* live state: a rainbow comet circles the card while a sign-in runs —
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
            <div className="absolute inset-0 rounded-[12px]" style={{ background: RAINBOW_BEAM }} />
          </motion.div>
        ) : null}
      </AnimatePresence>

      <div className="relative overflow-hidden rounded-[12px] bg-surface">
        <button
          type="button"
          aria-expanded={expanded}
          disabled={!actionable}
          onClick={() => setExpanded((open) => !open)}
          className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors duration-150 enabled:hover:bg-surface-2/50 disabled:cursor-default"
        >
          <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
            <AgentLogo agent={probe.agent} />
          </span>
          <span className="flex min-w-0 flex-1 items-center gap-2">
            <span className="truncate text-[13.5px] font-medium text-ink">
              {onboardingAgentLabel(probe.agent)}
            </span>
            <StatePill state={state} />
          </span>
          {state === 'ready' ? (
            <CheckCircle2 size={17} className="shrink-0 text-primary" />
          ) : actionable ? (
            <ChevronDown
              size={15}
              className={`shrink-0 text-ink-3 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}
            />
          ) : null}
        </button>

        <AnimatePresence initial={false}>
          {expanded && actionable ? (
            <motion.div
              key="body"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.2, ease: EASE }}
              className="overflow-hidden"
            >
              <div className="flex flex-col gap-2.5 px-3 pb-3 pt-0.5">
                {canKey ? (
                  <Segmented
                    layoutId={`onboarding-method-${probe.agent}`}
                    value={method}
                    onChange={setMethod}
                    options={[
                      { value: 'login', label: 'Sign in', icon: <LogIn size={13} /> },
                      { value: 'key', label: 'API key', icon: <KeyRound size={13} /> },
                    ]}
                  />
                ) : null}

                {method === 'login' || !canKey ? (
                  <div className="flex flex-col items-start gap-2.5">
                    <Button
                      variant="primary"
                      size="md"
                      disabled={!probe.auth_command_available || running}
                      onClick={onStartLogin}
                    >
                      {running ? <LoaderCircle size={14} className="animate-spin" /> : <LogIn size={14} />}
                      {running ? 'Waiting for sign-in…' : `Sign in with ${authProviderLabel(probe.agent)}`}
                    </Button>
                    {!probe.auth_command_available && probe.auth_command_reason ? (
                      <p className="text-[12px] text-danger">{probe.auth_command_reason}</p>
                    ) : null}
                    <div className="w-full">
                      <AuthLoginStatus job={loginJob} running={running} />
                    </div>
                  </div>
                ) : (
                  <div className="flex flex-col gap-2">
                    <Input
                      type="password"
                      value={apiKeyValue}
                      onChange={(event) => onAPIKeyChange(event.target.value)}
                      placeholder={probe.api_key_configured ? 'Already set up' : 'Paste an API key'}
                      autoComplete="off"
                      spellCheck={false}
                      className="font-mono text-[12px]"
                      aria-label={`${onboardingAgentLabel(probe.agent)} API key`}
                    />
                    <p className="text-[12px] text-ink-3">
                      jaz passes this key straight to {onboardingAgentLabel(probe.agent)}.
                    </p>
                  </div>
                )}
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
  apiKeyValue,
  disabled,
  onProviderChange,
  onAPIKeyChange,
}: {
  providers: AgentSettings['providers']
  selectedProvider: string
  configured: boolean
  apiKeyValue: string
  disabled: boolean
  onProviderChange: (value: string) => void
  onAPIKeyChange: (value: string) => void
}) {
  const provider = providers.find((item) => item.id === selectedProvider)
  const label = provider?.label || 'a model provider'
  const keyUrl = PROVIDER_KEY_URLS[selectedProvider]
  const hasDraft = apiKeyValue.trim().length > 0
  const pillState: AgentState = configured || hasDraft ? 'ready' : 'action'
  const pillLabel = configured ? 'Connected' : hasDraft ? 'Key added' : 'Needs key'
  // Like the coding agents, native collapses by default and opens on tap. It
  // stays expandable in every state (no auto-collapse) because its key is typed
  // in the body — collapsing on the first keystroke would yank the field away.
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="overflow-hidden rounded-[12px] bg-surface">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <span className="truncate text-[13.5px] font-medium text-ink">jaz native</span>
          <StatePill state={pillState} label={pillLabel} />
        </span>
        {configured ? <CheckCircle2 size={17} className="shrink-0 text-primary" /> : null}
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
            <div className="flex flex-col gap-2.5 px-3 pb-3 pt-0.5">
              <p className="text-pretty text-[12px] text-ink-3">
                jaz’s own agent connects directly to {label} with an API key you provide.
              </p>
              <div className="flex items-center justify-between gap-3">
                <span className="text-[13px] text-ink-2">Provider</span>
                <Select
                  value={selectedProvider}
                  options={providers.map((item) => ({ value: item.id, label: item.label }))}
                  onChange={onProviderChange}
                  disabled={disabled}
                  aria-label="Native provider"
                  className="h-8 min-w-[160px]"
                />
              </div>

              {configured ? (
                <div className="flex items-center gap-1.5 px-0.5 text-[12px] text-ink-2">
                  <Check size={14} className="shrink-0 text-primary" />
                  Your {label} key is already set up.
                </div>
              ) : (
                <div className="flex flex-col gap-2">
                  <Input
                    type="password"
                    value={apiKeyValue}
                    onChange={(event) => onAPIKeyChange(event.target.value)}
                    disabled={disabled || !selectedProvider}
                    placeholder={`Paste your ${label} API key`}
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono text-[12px]"
                    aria-label={`${label} API key`}
                  />
                  {keyUrl ? (
                    <button
                      type="button"
                      onClick={() => window.open(keyUrl, '_blank', 'noopener,noreferrer')}
                      className="inline-flex w-fit items-center gap-1 text-[12px] text-primary transition-colors duration-150 hover:text-primary-strong"
                    >
                      Where do I find my {label} key?
                      <ExternalLink size={12} />
                    </button>
                  ) : null}
                </div>
              )}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

// Inline segmented control, the elegant SidePanelControl pattern: a quiet pill
// track where the active option carries a spring-animated lozenge. Sizes to its
// content — never full width.
function Segmented<T extends string>({
  value,
  options,
  onChange,
  layoutId,
}: {
  value: T
  options: { value: T; label: string; icon?: ReactNode }[]
  onChange: (value: T) => void
  layoutId: string
}) {
  return (
    <div className="inline-flex h-8 items-center self-start rounded-full bg-bg p-0.5">
      {options.map((option) => {
        const active = option.value === value
        return (
          <motion.button
            key={option.value}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(option.value)}
            whileTap={{ scale: 0.96 }}
            className={`relative flex h-7 cursor-pointer items-center gap-1.5 rounded-full px-3 text-[13px] font-medium whitespace-nowrap transition-colors duration-150 ${
              active ? 'text-ink' : 'text-ink-2 hover:text-ink'
            }`}
          >
            {active ? (
              <motion.span
                layoutId={layoutId}
                transition={{ type: 'spring', duration: 0.32, bounce: 0 }}
                className="absolute inset-0 rounded-full bg-surface-2 shadow-sm ring-1 ring-border/50"
              />
            ) : null}
            <span className="relative flex items-center gap-1.5">
              {option.icon}
              {option.label}
            </span>
          </motion.button>
        )
      })}
    </div>
  )
}

function StatePill({ state, label }: { state: AgentState; label?: string }) {
  const tone =
    state === 'ready'
      ? 'bg-primary-soft text-primary-strong'
      : state === 'missing'
        ? 'bg-surface-2 text-ink-3'
        : 'bg-accent-soft text-accent-strong'
  const text = label ?? (state === 'ready' ? 'Connected' : state === 'missing' ? 'Not installed' : 'Needs sign-in')
  return (
    <span className={`inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium ${tone}`}>
      {text}
    </span>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return <p className="mb-2 px-1 text-[12px] font-medium text-ink-3">{children}</p>
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
    <span className="inline-flex items-center gap-2 rounded-full bg-surface px-2.5 py-1 text-[12px] text-ink-2">
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
        <div className="flex min-h-full w-full items-start justify-center py-6 md:py-10">
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
