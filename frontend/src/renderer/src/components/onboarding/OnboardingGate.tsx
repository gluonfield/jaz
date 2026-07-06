import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { DitherArt, DitherTerrain } from '@/components/launch/DitherArt'
import { useToast } from '@/components/ui/toast'
import { authProviderLabel } from '@/lib/agentLabel'
import { connectionPluginsQuery } from '@/lib/api/connections'
import { completeOnboarding, onboardingQuery } from '@/lib/api/onboarding'
import { cloneAgentSettings, compactKeys, prepareACPAgent, startACPAuthLogin } from '@/lib/api/settings'
import type { ACPAgentAuth, AgentSettings, OnboardingStatus } from '@/lib/api/types'
import { disconnectBackend, isLocalBackendUrl, useConnection } from '@/lib/connection'
import { clientRuntime } from '@/lib/clientRuntime'
import { devPreview } from '@/lib/devPreview'
import { useACPLoginPolling } from '@/lib/hooks/useACPLoginPolling'
import { selectableACPAgent } from '@/lib/agentRuntimes'
import { keys } from '@/lib/query/keys'
import { AgentList, agentReady } from './OnboardingAgents'
import { ConnectionsList } from './OnboardingConnections'
import {
  MemoryCard,
  OnboardingFooter,
  type OnboardingStep,
  slideRise,
  slideStagger,
  slideExit,
} from './OnboardingParts'
import { LoopsBoardsShowcase, SLIDES, WelcomeStep, slideFooter } from './OnboardingSlides'

const MEMORY_AGENT_PRIORITY = ['codex', 'claude', 'opencode', 'antigravity', 'grok']

// `?onboarding` pins the gate open so the flow can be iterated in a browser
// against a live, already-onboarded backend.
const onboardingPreview = devPreview('onboarding') !== null

export function OnboardingGate({ children }: { children: ReactNode }) {
  const onboarding = useQuery(onboardingQuery)
  // The preview pin is one-shot: finishing the flow releases it so "Launch
  // jaz" hands over to the app exactly like a real first run.
  const [preview, setPreview] = useState(onboardingPreview)
  const completed = onboarding.data?.completed === true && !preview

  if (clientRuntime.windowKind === 'board') return <>{children}</>
  if (completed) return <>{children}</>
  // One shell across loading → content, so the terrain and titlebar never
  // remount (a remount replays the boot art and reads as a blink).
  return (
    <OnboardingShell onDisconnect={disconnectBackend}>
      {onboarding.isPending ? null : onboarding.isError ? (
        <StatusBlock
          icon={<AlertCircle size={16} />}
          title="Couldn't load onboarding"
          text={onboarding.error.message}
        />
      ) : (
        <OnboardingScreen
          status={onboarding.data}
          onRefresh={() => void onboarding.refetch()}
          onFinished={() => setPreview(false)}
        />
      )}
    </OnboardingShell>
  )
}

function OnboardingScreen({
  status,
  onRefresh,
  onFinished,
}: {
  status: OnboardingStatus
  onRefresh: () => void
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
    (probe) =>
      probe.managed_adapter?.state === 'downloading' || probe.managed_tool?.state === 'downloading',
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

  const prepare = useMutation({
    mutationFn: (agent: string) => prepareACPAgent(agent),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.onboarding })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
    onError: (error: Error) => toast(`Couldn't download agent: ${error.message}`, 'danger'),
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

  const slide = step === 'welcome' ? null : SLIDES[step]
  const footer = step === 'welcome' ? null : slideFooter(step, { canContinue, memoryReady, anyConnected })

  return (
    <AnimatePresence mode="wait">
      {!slide || !footer ? (
        <WelcomeStep key="welcome" onStart={() => setStep('agents')} />
      ) : (
        <motion.div
          key={step}
          variants={slideStagger}
          initial="hidden"
          animate="show"
          exit={slideExit}
          className="flex w-full max-w-[460px] flex-col items-center"
        >
        <motion.div variants={slideRise}>
          <DitherArt draw={slide.motif} cols={104} rows={34} delay={0.1} />
        </motion.div>
        <motion.h1
          variants={slideRise}
          className="mt-6 text-balance text-center text-[22px] font-semibold tracking-tight text-ink"
        >
          {slide.title}
        </motion.h1>
        <motion.p
          variants={slideRise}
          className="mt-2 max-w-[360px] text-center text-pretty text-[13px] leading-relaxed text-ink-2"
        >
          {slide.subtitle}
        </motion.p>

        <motion.div variants={slideRise} className="mt-7 w-full">
          {step === 'agents' ? (
            <AgentList
              probes={onboardingProbes}
              remote={remote}
              acpKeysByAgent={acpKeysByAgent}
              loginJobs={loginJobs}
              loginPending={login.isPending ? login.variables?.agent : undefined}
              preparePending={prepare.isPending ? prepare.variables : undefined}
              onPrepare={(agent) => prepare.mutate(agent)}
              onStartLogin={(agent) => {
                const auth = onboardingLoginAuth(agent, draft.acp[agent]?.auth)
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
            nextLabel={footer.nextLabel}
            nextDisabled={footer.nextDisabled}
            busy={!slide.next && save.isPending}
            error={slide.next ? undefined : save.error?.message}
            onBack={() => setStep(slide.back)}
            onNext={() => (slide.next ? setStep(slide.next) : save.mutate())}
          />
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
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

function OnboardingShell({ children, onDisconnect }: { children: ReactNode; onDisconnect?: () => void }) {
  return (
    <div className="relative flex h-full flex-col bg-bg">
      <DitherTerrain className="absolute inset-x-0 bottom-0" delay={0.3} />
      {/* Always an escape back to the connect chooser, so onboarding a backend
          you can't finish never traps the app. Right-aligned to clear the macOS
          traffic lights. */}
      <div className="titlebar-drag relative flex h-[52px] shrink-0 items-center justify-end px-3">
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
      <main className="relative min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-5 pb-[52px]">
        <div className="flex min-h-full w-full items-center justify-center py-6 md:py-10">{children}</div>
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

function onboardingLoginAuth(agent: string, current?: ACPAgentAuth): ACPAgentAuth {
  if (agent === 'antigravity') return { mode: 'existing_cli' }
  if (current?.mode === 'jaz_profile') return current
  return { mode: 'jaz_profile' }
}

function withAgentAuth(settings: AgentSettings, agent: string, auth: ACPAgentAuth): AgentSettings {
  const next = cloneAgentSettings(settings)
  next.acp[agent] = { ...next.acp[agent], auth }
  return next
}
