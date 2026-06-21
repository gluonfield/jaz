import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  ChevronDown,
  KeyRound,
  LoaderCircle,
  Lock,
  LogIn,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useState } from 'react'
import { AgentLogo } from '@/components/acp/AgentLogo'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { Segmented } from '@/components/ui/Segmented'
import { Select } from '@/components/ui/Select'
import { Switch } from '@/components/ui/Switch'
import { authProviderLabel, onboardingAgentLabel } from '@/lib/agentLabel'
import type { ACPAuthLogin, OnboardingACPProbe } from '@/lib/api/types'
import { localDeviceLabel } from '@/lib/deviceLabel'

export const onboardingEase = [0.22, 1, 0.36, 1] as const

export const onboardingStagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.08 } },
}

export const onboardingRise = {
  hidden: { opacity: 0, y: 12, filter: 'blur(5px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.42, ease: onboardingEase } },
}

export type OnboardingStep = 'agents' | 'memory'

export function AgentSetupStep({
  probes,
  remote,
  acpKeysByAgent,
  loginJobs,
  loginPending,
  canContinue,
  onRefresh,
  onStartLogin,
  onAPIKeyChange,
  onContinue,
}: {
  probes: OnboardingACPProbe[]
  remote: boolean
  acpKeysByAgent: Record<string, string>
  loginJobs: Record<string, ACPAuthLogin>
  loginPending?: string
  canContinue: boolean
  onRefresh: () => void
  onStartLogin: (agent: string) => void
  onAPIKeyChange: (agent: string, value: string) => void
  onContinue: () => void
}) {
  const deviceLabel = localDeviceLabel()
  return (
    <motion.div
      variants={onboardingRise}
      initial="hidden"
      animate="show"
      exit={{ opacity: 0, y: -6, filter: 'blur(3px)', transition: { duration: 0.16, ease: onboardingEase } }}
    >
      <SectionLabel>Coding agents</SectionLabel>
      <div className="grid gap-1.5">
        {probes.map((probe) => (
          <AgentCard
            key={probe.agent}
            probe={probe}
            agentHost={remote ? 'your server' : deviceLabel}
            apiKeyValue={acpKeysByAgent[probe.agent] ?? ''}
            loginJob={loginJobs[probe.agent]}
            loginPending={loginPending === probe.agent}
            onStartLogin={() => onStartLogin(probe.agent)}
            onAPIKeyChange={(value) => onAPIKeyChange(probe.agent, value)}
          />
        ))}
      </div>

      <div className="mt-4 flex items-center gap-2.5 px-1">
        <Lock size={13} className="shrink-0 text-ink-3" />
        <p className="text-pretty text-[12px] leading-relaxed text-ink-3">
          {remote
            ? 'Your logins and keys are stored on your server and never leave it.'
            : `Your logins and keys are stored on ${deviceLabel} and never leave your machine.`}
        </p>
      </div>

      <div className="mt-5 flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between">
        <p className="min-h-5 text-pretty text-[12px] text-ink-3">
          {canContinue ? 'Continue to choose how memory works.' : 'Connect one coding agent to continue.'}
        </p>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="lg" onClick={onRefresh} title="Re-check agent status">
            Refresh
          </Button>
          <Button variant="primary" size="lg" disabled={!canContinue} onClick={onContinue}>
            Continue
            <ArrowRight size={14} />
          </Button>
        </div>
      </div>
    </motion.div>
  )
}

export function MemorySetupStep({
  enabled,
  agent,
  agents,
  saving,
  error,
  canFinish,
  onEnabledChange,
  onAgentChange,
  onBack,
  onFinish,
}: {
  enabled: boolean
  agent: string
  agents: string[]
  saving: boolean
  error: string
  canFinish: boolean
  onEnabledChange: (enabled: boolean) => void
  onAgentChange: (agent: string) => void
  onBack: () => void
  onFinish: () => void
}) {
  const options = agents.map((value) => ({
    value,
    label: onboardingAgentLabel(value),
  }))
  return (
    <motion.div
      variants={onboardingRise}
      initial="hidden"
      animate="show"
      exit={{ opacity: 0, y: -6, filter: 'blur(3px)', transition: { duration: 0.16, ease: onboardingEase } }}
    >
      <div className="rounded-[12px] bg-surface p-3 shadow-sm">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-[13.5px] font-medium text-ink">Memory</p>
            <p className="mt-0.5 text-pretty text-[12px] leading-relaxed text-ink-2">
              Memory lets Jaz remember your preferences, decisions, and project context, so agents
              understand you across projects instead of starting cold.
            </p>
          </div>
          <Switch checked={enabled} onChange={onEnabledChange} aria-label="Enable memory" />
        </div>
        <div className={enabled ? 'mt-4' : 'pointer-events-none mt-4 opacity-50'}>
          <SectionLabel>Select an agent</SectionLabel>
          <Select
            value={agent}
            options={options}
            disabled={!enabled || options.length === 0}
            onChange={onAgentChange}
            aria-label="Select memory agent"
            className="h-10 w-full justify-between rounded-[9px] bg-bg px-3 text-[13px]"
          />
        </div>
      </div>

      <div className="mt-5 flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between">
        <p className={`min-h-5 text-pretty text-[12px] ${error ? 'text-danger' : 'text-ink-3'}`}>
          {error ||
            (enabled
              ? agent
                ? `Using ${onboardingAgentLabel(agent)} for memory.`
                : 'Select an agent to continue.'
              : 'Memory will stay off.')}
        </p>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="lg" onClick={onBack}>
            <ArrowLeft size={14} />
            Back
          </Button>
          <Button variant="primary" size="lg" disabled={!canFinish || saving} onClick={onFinish}>
            {saving && <LoaderCircle size={14} className="animate-spin" />}
            Finish setup
          </Button>
        </div>
      </div>
    </motion.div>
  )
}

export function OnboardingProgress({
  step,
  agentsComplete,
}: {
  step: OnboardingStep
  agentsComplete: boolean
}) {
  const items = [
    { value: 'agents', label: 'Agents', complete: agentsComplete },
    { value: 'memory', label: 'Memory', complete: false },
  ] as const
  return (
    <ol
      aria-label="Setup progress"
      className="flex shrink-0 items-center text-[12px]"
    >
      {items.map((item, index) => {
        const active = step === item.value
        const complete = item.complete
        return (
          <li
            key={item.value}
            aria-current={active ? 'step' : undefined}
            className="flex items-center"
          >
            <span
              className={`grid size-[18px] place-items-center rounded-full ring-1 ${
                complete
                  ? 'bg-primary-soft text-primary-strong ring-primary/20'
                  : active
                    ? 'bg-primary text-on-primary ring-primary'
                    : 'bg-bg text-ink-3 ring-border'
              }`}
            >
              {complete ? <Check size={11} /> : <span className="size-1.5 rounded-full bg-current" />}
            </span>
            <span
              className={`ml-1.5 font-medium ${
                active ? 'text-ink' : complete ? 'text-ink-2' : 'text-ink-3'
              }`}
            >
              {item.label}
            </span>
            {index === 0 ? (
              <span
                aria-hidden
                className={`mx-2 h-px w-8 ${complete ? 'bg-primary/45' : 'bg-border'}`}
              />
            ) : null}
          </li>
        )
      })}
    </ol>
  )
}

type AgentState = 'ready' | 'action' | 'missing'

export function agentReady(probe: OnboardingACPProbe, keyDraft: string): boolean {
  return Boolean(probe.available || probe.api_key_configured || keyDraft.trim())
}

function agentState(probe: OnboardingACPProbe, keyDraft: string): AgentState {
  if (agentReady(probe, keyDraft)) return 'ready'
  if (!probe.installed) return 'missing'
  return 'action'
}

function AgentCard({
  probe,
  agentHost,
  apiKeyValue,
  loginJob,
  loginPending,
  onStartLogin,
  onAPIKeyChange,
}: {
  probe: OnboardingACPProbe
  agentHost: string
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
  const canLogin = Boolean(probe.auth_command_available)
  const [expanded, setExpanded] = useState(false)
  const [method, setMethod] = useState<'login' | 'key'>(canKey && (!canLogin || apiKeyReady) ? 'key' : 'login')
  useEffect(() => {
    if (canKey && !canLogin && method === 'login') setMethod('key')
  }, [canKey, canLogin, method])
  const actionable = state === 'action'
  const companionAppBlocked = Boolean(probe.app_installed && !probe.available && !probe.auth_command_available)
  const missingLabel = companionAppBlocked ? `Needs ${onboardingAgentLabel(probe.agent)}` : undefined
  const missingDetail = companionAppBlocked
    ? `${probe.app_name || authProviderLabel(probe.agent)} is installed on ${agentHost}, but ${onboardingAgentLabel(probe.agent)} is not available to jaz.`
    : state === 'missing'
      ? probe.reason
      : ''

  return (
    <div className="relative">
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
            <StatePill state={state} label={missingLabel} />
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
        {missingDetail ? (
          <p className="px-3 pb-2 text-pretty text-[12px] leading-relaxed text-ink-3">{missingDetail}</p>
        ) : null}

        <AnimatePresence initial={false}>
          {expanded && actionable ? (
            <motion.div
              key="body"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.2, ease: onboardingEase }}
              className="overflow-hidden"
            >
              <div className="flex flex-col gap-2.5 px-3 pb-3 pt-0.5">
                {canKey && canLogin ? (
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

                {(method === 'login' && canLogin) || !canKey ? (
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
