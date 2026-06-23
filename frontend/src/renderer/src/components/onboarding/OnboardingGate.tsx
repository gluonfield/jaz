import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { authProviderLabel } from '@/lib/agentLabel'
import { completeOnboarding, onboardingQuery } from '@/lib/api/onboarding'
import { cloneAgentSettings, compactKeys, startACPAuthLogin } from '@/lib/api/settings'
import type { ACPAgentAuth, AgentSettings, OnboardingStatus } from '@/lib/api/types'
import { isLoopbackUrl, useConnection } from '@/lib/connection'
import { localDeviceLabel } from '@/lib/deviceLabel'
import { useACPLoginPolling } from '@/lib/hooks/useACPLoginPolling'
import { keys } from '@/lib/query/keys'
import {
  AgentSetupStep,
  MemorySetupStep,
  OnboardingProgress,
  agentReady,
  type OnboardingStep,
  onboardingRise,
  onboardingStagger,
} from './OnboardingParts'

const MEMORY_AGENT_PRIORITY = ['codex', 'claude', 'opencode', 'grok']

export function OnboardingGate({ children }: { children: ReactNode }) {
  const onboarding = useQuery(onboardingQuery)

  if (window.jaz?.windowKind === 'board') return <>{children}</>
  if (onboarding.isPending) {
    return (
      <OnboardingShell>
        <SkeletonRows count={4} />
      </OnboardingShell>
    )
  }
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
  const [step, setStep] = useState<OnboardingStep>('agents')
  const [draft, setDraft] = useState(() => draftFromStatus(status))
  const [acpKeysByAgent, setACPKeysByAgent] = useState<Record<string, string>>({})
  const [memoryEnabled, setMemoryEnabled] = useState(status.memory?.enabled ?? true)
  const [memoryAgent, setMemoryAgent] = useState(status.memory?.agent ?? '')
  const onboardingProbes = useMemo(() => status.acp.filter((probe) => probe.agent !== 'jaz'), [status.acp])
  const adapterPreparing = onboardingProbes.some(
    (probe) => probe.managed_adapter?.state === 'missing' || probe.managed_adapter?.state === 'downloading',
  )

  useEffect(() => {
    setDraft(draftFromStatus(status))
  }, [status])

  const { loginJobs, trackLoginJob } = useACPLoginPolling(() => {
    queryClient.invalidateQueries({ queryKey: keys.onboarding })
    queryClient.invalidateQueries({ queryKey: keys.agentSettings })
    queryClient.invalidateQueries({ queryKey: keys.acpAgents })
  })

  const readyAgentNames = useMemo(
    () =>
      orderedMemoryAgents(
        onboardingProbes
          .filter((probe) => agentReady(probe, acpKeysByAgent[probe.agent] ?? ''))
          .map((probe) => probe.agent),
      ),
    [onboardingProbes, acpKeysByAgent],
  )
  const readyAgents = useMemo(() => new Set(readyAgentNames), [readyAgentNames])
  const canContinue = readyAgentNames.length > 0
  const canFinish = !memoryEnabled || memoryAgent.trim() !== ''
  const title = step === 'agents' ? 'Connect your agents' : 'Set up memory'

  useEffect(() => {
    setMemoryEnabled(status.memory?.enabled ?? true)
  }, [status.memory?.enabled])

  useEffect(() => {
    setMemoryAgent((current) => preferredMemoryAgent(current || status.memory?.agent || '', readyAgentNames))
  }, [readyAgentNames, status.memory?.agent])

  useEffect(() => {
    if (!canContinue && step === 'memory') setStep('agents')
  }, [canContinue, step])

  useEffect(() => {
    if (!adapterPreparing) return
    const timer = window.setInterval(onRefresh, 1500)
    return () => window.clearInterval(timer)
  }, [adapterPreparing, onRefresh])

  const login = useMutation({
    mutationFn: ({ agent, auth }: { agent: string; auth?: ACPAgentAuth }) => startACPAuthLogin(agent, auth),
    onSuccess: (job) => {
      trackLoginJob(job)
      toast(`Started ${authProviderLabel(job.agent)} sign-in`)
    },
    onError: (error: Error) => toast(`Couldn't start sign-in: ${error.message}`, 'danger'),
  })

  const save = useMutation({
    mutationFn: () => {
      const next = cloneAgentSettings(draft)
      for (const probe of status.acp) {
        const current = next.acp[probe.agent]
        next.acp[probe.agent] = {
          ...current,
          enabled: probe.agent === 'jaz' ? Boolean(current?.enabled) : readyAgents.has(probe.agent),
        }
      }
      return completeOnboarding({
        settings: next,
        memory: {
          enabled: memoryEnabled,
          agent: memoryAgent.trim() || undefined,
        },
        acp_keys: compactKeys(acpKeysByAgent),
        completed: true,
      })
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(keys.onboarding, saved)
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  return (
    <OnboardingShell>
      <motion.div
        variants={onboardingStagger}
        initial="hidden"
        animate="show"
        className="min-w-0 w-full max-w-[calc(100vw-40px)] md:max-w-[500px]"
      >
        <motion.div
          variants={onboardingRise}
          className="mb-5"
        >
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <BackendChip remote={remote} url={connection.url} />
            <OnboardingProgress step={step} agentsComplete={canContinue} />
          </div>
          <h1 className="mt-3 text-balance text-[20px] font-semibold tracking-tight text-ink">
            {title}
          </h1>
        </motion.div>

        <AnimatePresence mode="wait" initial={false}>
          {step === 'agents' ? (
            <AgentSetupStep
              key="agents"
              probes={onboardingProbes}
              remote={remote}
              acpKeysByAgent={acpKeysByAgent}
              loginJobs={loginJobs}
              loginPending={login.isPending ? login.variables?.agent : undefined}
              canContinue={canContinue}
              onRefresh={onRefresh}
              onStartLogin={(agent) => {
                const auth = onboardingLoginAuth(draft.acp[agent]?.auth)
                setDraft((current) => withAgentAuth(current, agent, auth))
                login.mutate({ agent, auth })
              }}
              onAPIKeyChange={(agent, value) => setACPKeysByAgent((keys) => ({ ...keys, [agent]: value }))}
              onContinue={() => setStep('memory')}
            />
          ) : (
            <MemorySetupStep
              key="memory"
              enabled={memoryEnabled}
              agent={memoryAgent}
              agents={readyAgentNames}
              saving={save.isPending}
              error={save.error?.message ?? ''}
              canFinish={canFinish}
              onEnabledChange={setMemoryEnabled}
              onAgentChange={setMemoryAgent}
              onBack={() => setStep('agents')}
              onFinish={() => save.mutate()}
            />
          )}
        </AnimatePresence>
      </motion.div>
    </OnboardingShell>
  )
}

function BackendChip({ remote, url }: { remote: boolean; url: string }) {
  const deviceLabel = localDeviceLabel()
  const host = (() => {
    try {
      return new URL(url).host
    } catch {
      return url
    }
  })()
  return (
    <span className="inline-flex items-center gap-2 rounded-full bg-surface px-2.5 py-1 text-[12px] text-ink-2">
      <span className="text-ink">{remote ? 'Connected to server' : `Running on ${deviceLabel}`}</span>
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

function orderedMemoryAgents(agents: string[]): string[] {
  const unique = Array.from(new Set(agents.filter((agent) => agent && agent !== 'jaz')))
  const rank = new Map(MEMORY_AGENT_PRIORITY.map((agent, index) => [agent, index]))
  return unique.sort((left, right) => {
    const leftRank = rank.get(left) ?? Number.MAX_SAFE_INTEGER
    const rightRank = rank.get(right) ?? Number.MAX_SAFE_INTEGER
    if (leftRank !== rightRank) return leftRank - rightRank
    return left.localeCompare(right)
  })
}

function preferredMemoryAgent(current: string, agents: string[]): string {
  const value = current.trim()
  if (value && agents.includes(value)) return value
  return agents[0] ?? ''
}

function draftFromStatus(status: OnboardingStatus): AgentSettings {
  const settings = cloneAgentSettings(status.settings)
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

function onboardingLoginAuth(current?: ACPAgentAuth): ACPAgentAuth {
  if (current?.mode === 'jaz_profile') return current
  return { mode: 'jaz_profile' }
}

function withAgentAuth(settings: AgentSettings, agent: string, auth: ACPAgentAuth): AgentSettings {
  const next = cloneAgentSettings(settings)
  next.acp[agent] = { ...next.acp[agent], auth }
  return next
}
