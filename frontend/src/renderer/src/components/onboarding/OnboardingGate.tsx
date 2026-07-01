import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { DitherArt, type Silhouette } from '@/components/launch/DitherArt'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { authProviderLabel } from '@/lib/agentLabel'
import { connectionPluginsQuery } from '@/lib/api/connections'
import { completeOnboarding, onboardingQuery } from '@/lib/api/onboarding'
import { cloneAgentSettings, compactKeys, startACPAuthLogin } from '@/lib/api/settings'
import type { ACPAgentAuth, AgentSettings, OnboardingStatus } from '@/lib/api/types'
import { disconnectBackend, isLocalBackendUrl, useConnection } from '@/lib/connection'
import { clientRuntime } from '@/lib/clientRuntime'
import { useACPLoginPolling } from '@/lib/hooks/useACPLoginPolling'
import { selectableACPAgent } from '@/lib/agentRuntimes'
import { keys } from '@/lib/query/keys'
import {
  AgentList,
  MemoryCard,
  OnboardingFooter,
  agentReady,
  type OnboardingStep,
  onboardingEase,
} from './OnboardingParts'
import { ConnectionsList } from './OnboardingConnections'
import { LoopsBoardsShowcase, WelcomeStep } from './OnboardingSlides'

const MEMORY_AGENT_PRIORITY = ['codex', 'claude', 'opencode', 'grok']

// Dev-only escape hatch: `?onboarding` pins the gate open so the flow can be
// iterated in a browser against a live, already-onboarded backend.
const onboardingPreview =
  import.meta.env.DEV && new URLSearchParams(window.location.search).has('onboarding')

type SetupStep = Exclude<OnboardingStep, 'welcome'>

// Each slide opens on a small mark dissolved in the same dither grain as the
// wordmark: a prompt chevron, memory layers, linked rings, a loop orbit.
const MOTIFS: Record<SetupStep, Silhouette> = {
  agents: (g, w, h) => {
    g.lineWidth = h * 0.17
    g.lineCap = 'round'
    g.lineJoin = 'round'
    g.beginPath()
    g.moveTo(w * 0.26, h * 0.2)
    g.lineTo(w * 0.5, h * 0.5)
    g.lineTo(w * 0.26, h * 0.8)
    g.stroke()
    g.beginPath()
    g.roundRect(w * 0.58, h * 0.7, w * 0.18, h * 0.13, h * 0.06)
    g.fill()
  },
  memory: (g, w, h) => {
    for (const y of [0.14, 0.42, 0.7]) {
      g.beginPath()
      g.roundRect(w * 0.24, h * y, w * 0.52, h * 0.16, h * 0.08)
      g.fill()
    }
  },
  connections: (g, w, h) => {
    g.lineWidth = h * 0.14
    for (const x of [0.38, 0.62]) {
      g.beginPath()
      g.arc(w * x, h * 0.5, h * 0.3, 0, Math.PI * 2)
      g.stroke()
    }
  },
  loops: (g, w, h) => {
    g.lineWidth = h * 0.14
    g.beginPath()
    g.arc(w * 0.5, h * 0.5, h * 0.32, 0, Math.PI * 2)
    g.stroke()
    g.beginPath()
    g.arc(w * 0.5 + h * 0.32 * Math.cos(-Math.PI / 4), h * 0.5 + h * 0.32 * Math.sin(-Math.PI / 4), h * 0.13, 0, Math.PI * 2)
    g.fill()
  },
}

const TITLES: Record<SetupStep, string> = {
  agents: 'Connect your agents',
  memory: 'Give jaz a memory',
  connections: 'Connect your world',
  loops: 'Always working',
}

const SUBTITLES: Record<SetupStep, string> = {
  agents: 'jaz runs on the coding agents you already use.',
  memory: 'Preferences, decisions, projects — agents start warm, not cold.',
  connections: 'Your email and chats become context agents can use.',
  loops: 'Loops run agents on a schedule. Boards show their results live.',
}

const NEXT: Record<SetupStep, OnboardingStep> = {
  agents: 'memory',
  memory: 'connections',
  connections: 'loops',
  loops: 'loops',
}

const BACK: Record<SetupStep, OnboardingStep> = {
  agents: 'welcome',
  memory: 'agents',
  connections: 'memory',
  loops: 'connections',
}

const slideStagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
}

const slideRise = {
  hidden: { opacity: 0, y: 12, filter: 'blur(5px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.42, ease: onboardingEase } },
}

export function OnboardingGate({ children }: { children: ReactNode }) {
  const onboarding = useQuery(onboardingQuery)
  // The preview pin is one-shot: finishing the flow releases it so "Launch
  // jaz" hands over to the app exactly like a real first run.
  const [preview, setPreview] = useState(onboardingPreview)
  const completed = onboarding.data?.completed === true && !preview

  if (clientRuntime.windowKind === 'board') return <>{children}</>
  if (onboarding.isPending) {
    return (
      <OnboardingShell onDisconnect={disconnectBackend}>
        <SkeletonRows count={4} />
      </OnboardingShell>
    )
  }
  if (onboarding.isError) {
    return (
      <OnboardingShell onDisconnect={disconnectBackend}>
        <StatusBlock
          icon={<AlertCircle size={16} />}
          title="Couldn't load onboarding"
          text={onboarding.error.message}
        />
      </OnboardingShell>
    )
  }
  if (completed) return <>{children}</>
  return (
    <OnboardingScreen
      status={onboarding.data}
      onRefresh={() => void onboarding.refetch()}
      onDisconnect={disconnectBackend}
      onFinished={() => setPreview(false)}
    />
  )
}

function OnboardingScreen({
  status,
  onRefresh,
  onDisconnect,
  onFinished,
}: {
  status: OnboardingStatus
  onRefresh: () => void
  onDisconnect: () => void
  onFinished: () => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const connection = useConnection()
  const remote = !isLocalBackendUrl(connection.url)
  const [step, setStep] = useState<OnboardingStep>('welcome')
  const [draft, setDraft] = useState(() => draftFromStatus(status))
  const [acpKeysByAgent, setACPKeysByAgent] = useState<Record<string, string>>({})
  const [memoryEnabled, setMemoryEnabled] = useState(status.memory?.enabled ?? true)
  const [memoryAgent, setMemoryAgent] = useState(status.memory?.agent ?? '')
  const onboardingProbes = useMemo(() => status.acp.filter((probe) => selectableACPAgent(probe.agent)), [status.acp])
  const adapterPreparing = onboardingProbes.some(
    (probe) => probe.managed_adapter?.state === 'missing' || probe.managed_adapter?.state === 'downloading',
  )
  // Only for the connections slide's action label; shares the list's cache.
  const plugins = useQuery(connectionPluginsQuery)
  const anyConnected = (plugins.data ?? []).some((plugin) => plugin.connection?.status === 'connected')

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
  const memoryReady = !memoryEnabled || memoryAgent.trim() !== ''

  useEffect(() => {
    setMemoryEnabled(status.memory?.enabled ?? true)
  }, [status.memory?.enabled])

  useEffect(() => {
    setMemoryAgent((current) => preferredMemoryAgent(current || status.memory?.agent || '', readyAgentNames))
  }, [readyAgentNames, status.memory?.agent])

  useEffect(() => {
    if (!canContinue && step !== 'welcome' && step !== 'agents') setStep('agents')
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
      for (const probe of onboardingProbes) {
        const current = next.acp[probe.agent]
        next.acp[probe.agent] = {
          ...current,
          enabled: readyAgents.has(probe.agent),
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
      onFinished()
    },
  })

  return (
    <OnboardingShell onDisconnect={onDisconnect} center>
      <AnimatePresence mode="wait">
        {step === 'welcome' ? (
          <WelcomeStep key="welcome" onStart={() => setStep('agents')} />
        ) : (
          <motion.div
            key={step}
            variants={slideStagger}
            initial="hidden"
            animate="show"
            exit={{ opacity: 0, y: -8, filter: 'blur(4px)', transition: { duration: 0.16, ease: onboardingEase } }}
            className="flex w-full max-w-[460px] flex-col items-center"
          >
            <motion.div variants={slideRise}>
              <DitherArt draw={MOTIFS[step]} cols={40} rows={24} delay={0.15} />
            </motion.div>
            <motion.h1
              variants={slideRise}
              className="mt-6 text-balance text-center text-[22px] font-semibold tracking-tight text-ink"
            >
              {TITLES[step]}
            </motion.h1>
            <motion.p
              variants={slideRise}
              className="mt-2 max-w-[360px] text-center text-pretty text-[13px] leading-relaxed text-ink-2"
            >
              {SUBTITLES[step]}
            </motion.p>

            <motion.div variants={slideRise} className="mt-7 w-full">
              {step === 'agents' ? (
                <AgentList
                  probes={onboardingProbes}
                  remote={remote}
                  acpKeysByAgent={acpKeysByAgent}
                  loginJobs={loginJobs}
                  loginPending={login.isPending ? login.variables?.agent : undefined}
                  onStartLogin={(agent) => {
                    const auth = onboardingLoginAuth(draft.acp[agent]?.auth)
                    setDraft((current) => withAgentAuth(current, agent, auth))
                    login.mutate({ agent, auth })
                  }}
                  onAPIKeyChange={(agent, value) => setACPKeysByAgent((keys) => ({ ...keys, [agent]: value }))}
                />
              ) : step === 'memory' ? (
                <MemoryCard
                  enabled={memoryEnabled}
                  agent={memoryAgent}
                  agents={readyAgentNames}
                  onEnabledChange={setMemoryEnabled}
                  onAgentChange={setMemoryAgent}
                />
              ) : step === 'connections' ? (
                <ConnectionsList />
              ) : (
                <LoopsBoardsShowcase />
              )}
            </motion.div>

            <motion.div variants={slideRise} className="w-full">
              <OnboardingFooter
                step={step}
                nextLabel={step === 'loops' ? 'Launch jaz' : step === 'connections' && !anyConnected ? 'Skip for now' : 'Continue'}
                nextDisabled={step === 'agents' ? !canContinue : step === 'memory' ? !memoryReady : false}
                busy={step === 'loops' && save.isPending}
                error={step === 'loops' ? save.error?.message : undefined}
                onBack={() => setStep(BACK[step])}
                onNext={() => (step === 'loops' ? save.mutate() : setStep(NEXT[step]))}
              />
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </OnboardingShell>
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

function OnboardingShell({
  children,
  onDisconnect,
  center = false,
}: {
  children: ReactNode
  onDisconnect?: () => void
  center?: boolean
}) {
  return (
    <div className="flex h-full flex-col bg-bg">
      {/* Always an escape back to the connect chooser, so onboarding a backend
          you can't finish never traps the app. Right-aligned to clear the macOS
          traffic lights. */}
      <div className="titlebar-drag flex h-[52px] shrink-0 items-center justify-end px-3">
        {onDisconnect ? (
          <button
            type="button"
            onClick={onDisconnect}
            className="rounded-full px-2.5 py-1.5 text-[12px] font-medium text-ink-2 transition-colors duration-150 [-webkit-app-region:no-drag] hover:bg-surface-2 hover:text-ink"
          >
            Use a different backend
          </button>
        ) : null}
      </div>
      <main className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-5 pb-[52px]">
        <div className={`flex min-h-full w-full ${center ? 'items-center' : 'items-start'} justify-center py-6 md:py-10`}>
          {children}
        </div>
      </main>
    </div>
  )
}

function orderedMemoryAgents(agents: string[]): string[] {
  const unique = Array.from(new Set(agents.filter(selectableACPAgent)))
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
  for (const probe of status.acp.filter((item) => selectableACPAgent(item.agent))) {
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
