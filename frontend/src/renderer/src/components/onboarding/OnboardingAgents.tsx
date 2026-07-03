import { CheckCircle2, ChevronDown, KeyRound, LoaderCircle, Lock, LogIn } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useState } from 'react'
import { AgentLogo } from '@/components/acp/AgentLogo'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { Segmented } from '@/components/ui/Segmented'
import { authProviderLabel, onboardingAgentLabel } from '@/lib/agentLabel'
import type { ACPAuthLogin, OnboardingACPProbe } from '@/lib/api/types'
import { localDeviceLabel } from '@/lib/deviceLabel'
import { onboardingEase } from './OnboardingParts'

export function AgentList({
  probes,
  remote,
  acpKeysByAgent,
  loginJobs,
  loginPending,
  onStartLogin,
  onAPIKeyChange,
}: {
  probes: OnboardingACPProbe[]
  remote: boolean
  acpKeysByAgent: Record<string, string>
  loginJobs: Record<string, ACPAuthLogin>
  loginPending?: string
  onStartLogin: (agent: string) => void
  onAPIKeyChange: (agent: string, value: string) => void
}) {
  const deviceLabel = localDeviceLabel()
  return (
    <div>
      <div className="grid grid-cols-1 gap-1.5">
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
      <p className="mt-3 flex items-center justify-center gap-1.5 text-[12px] text-ink-3">
        <Lock size={12} className="shrink-0" />
        {remote ? 'Logins and keys stay on your server.' : `Logins and keys stay on ${deviceLabel}.`}
      </p>
    </div>
  )
}

type AgentState = 'ready' | 'action' | 'missing' | 'downloading' | 'failed'

export function agentReady(probe: OnboardingACPProbe, keyDraft: string): boolean {
  return probeReady(probe) || (adapterReady(probe) && Boolean(keyDraft.trim()))
}

function probeReady(probe: OnboardingACPProbe): boolean {
  return adapterReady(probe) && Boolean(probe.available || probe.api_key_configured)
}

function agentState(probe: OnboardingACPProbe): AgentState {
  const authReady = Boolean(probe.authenticated || probe.api_key_configured)
  if (authReady && probe.managed_adapter?.state === 'downloading') return 'downloading'
  if (authReady && probe.managed_adapter?.state === 'failed') return 'failed'
  if (authReady && probe.managed_adapter?.state === 'missing') return 'downloading'
  if (probeReady(probe)) return 'ready'
  if (!probe.installed) return 'missing'
  return 'action'
}

function adapterReady(probe: OnboardingACPProbe): boolean {
  return !probe.managed_adapter || probe.managed_adapter.state === 'ready'
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
  const state = agentState(probe)
  const running = loginPending || loginJob?.status === 'running'
  const canKey = Boolean(apiKeyEnv)
  const canLogin = Boolean(probe.auth_command_available)
  const [expanded, setExpanded] = useState(false)
  const [chosen, setChosen] = useState<'login' | 'key'>(apiKeyReady ? 'key' : 'login')
  const method = canKey && !canLogin ? 'key' : !canKey ? 'login' : chosen
  const actionable = state === 'action'
  const companionAppBlocked = Boolean(probe.app_installed && !probe.available && !probe.auth_command_available)
  const missingLabel = companionAppBlocked ? `Needs ${onboardingAgentLabel(probe.agent)}` : undefined
  let missingDetail = ''
  if (companionAppBlocked) {
    missingDetail = `${probe.app_name || authProviderLabel(probe.agent)} is installed on ${agentHost}, but ${onboardingAgentLabel(probe.agent)} is not available to jaz.`
  } else if (state === 'downloading') {
    missingDetail = probe.managed_adapter?.message || 'Downloading the managed adapter.'
  } else if (state === 'failed') {
    missingDetail = probe.managed_adapter?.message || 'Managed adapter download failed.'
  } else if (state === 'missing') {
    missingDetail = probe.reason ?? ''
  }

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
            {state !== 'ready' ? <StatePill state={state} label={missingLabel} /> : null}
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
                    onChange={setChosen}
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
                    {!canLogin && probe.auth_command_reason ? (
                      <p className="text-[12px] text-ink-3">
                        Sign-in unavailable: {probe.auth_command_reason}. Use an API key instead.
                      </p>
                    ) : null}
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
      : state === 'missing' || state === 'failed'
        ? 'bg-surface-2 text-ink-3'
        : 'bg-accent-soft text-accent-strong'
  const text =
    label ??
    (state === 'ready'
      ? 'Connected'
      : state === 'missing'
        ? 'Not installed'
        : state === 'downloading'
          ? 'Downloading'
          : state === 'failed'
            ? 'Download failed'
            : 'Needs sign-in')
  return (
    <span className={`inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium ${tone}`}>
      {text}
    </span>
  )
}
