import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, CircleAlert, LoaderCircle, LogIn } from 'lucide-react'
import { useRef, useState } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Collapse } from '@/components/ui/Collapse'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { agentLabel, authProviderLabel } from '@/lib/agentLabel'
import {
  cloudACPProfilesQuery,
  deleteCloudACPProfile,
  getCloudACPLogin,
  startCloudACPLogin,
  supportsDesktopCloudLogin,
  toACPAuthLogin,
  type CloudACPAgentSpec,
  type CloudACPProfile,
} from '@/lib/api/cloudACP'
import type { ACPAuthLogin } from '@/lib/api/types'
import { useIsCloudOnly } from '@/lib/cloudSession'
import { useACPLoginPolling } from '@/lib/hooks/useACPLoginPolling'
import { keys } from '@/lib/query/keys'

export function CloudACPSettings() {
  const cloudOnly = useIsCloudOnly()
  const queryClient = useQueryClient()
  const toast = useToast()
  const { data, isLoading, error } = useQuery({
    ...cloudACPProfilesQuery,
    refetchInterval: (query) =>
      query.state.data?.profiles.some((p) => p.status === 'refreshing') ? 2_000 : false,
  })

  const viaDesktopByJob = useRef<Record<string, boolean>>({})
  const { loginJobs, trackLoginJob, forgetLoginJob } = useACPLoginPolling(
    (job) => {
      delete viaDesktopByJob.current[job.id]
      if (job.status === 'succeeded') {
        toast(`Connected ${agentLabel(job.agent)}`)
        void queryClient.invalidateQueries({ queryKey: keys.cloudACPProfiles })
        void queryClient.invalidateQueries({ queryKey: keys.cloudModelProfiles })
      }
    },
    async (job) => {
      const updated = await getCloudACPLogin(job.agent, job.id, Boolean(viaDesktopByJob.current[job.id]))
      return toACPAuthLogin(updated) ?? job
    },
  )

  const startLogin = useMutation({
    mutationFn: ({ agent, viaDesktop }: { agent: string; viaDesktop: boolean }) =>
      startCloudACPLogin(agent, viaDesktop),
    onSuccess: (session) => {
      viaDesktopByJob.current[session.id] = Boolean(session.viaDesktop)
      const job = toACPAuthLogin(session)
      if (job) trackLoginJob(job)
      toast(`Started ${authProviderLabel(session.agent)} sign-in`)
    },
    onError: (err) => {
      toast(err instanceof Error ? err.message : 'Could not start sign-in', 'danger')
    },
  })

  const disconnect = useMutation({
    mutationFn: (agent: string) => deleteCloudACPProfile(agent),
    onSuccess: (res) => {
      forgetLoginJob(res.agent)
      toast(`Disconnected ${agentLabel(res.agent)}`)
      void queryClient.invalidateQueries({ queryKey: keys.cloudACPProfiles })
      void queryClient.invalidateQueries({ queryKey: keys.cloudModelProfiles })
    },
    onError: (err) => {
      toast(err instanceof Error ? err.message : 'Could not disconnect agent', 'danger')
    },
  })

  const profilesByAgent = new Map((data?.profiles ?? []).map((p) => [p.agent, p]))
  const specs = data?.supported_agents ?? []

  return (
    <section className="flex flex-col gap-5 py-4">
      <div>
        <h1 className="text-lg font-semibold text-ink">Agents (ACP)</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">
          Configure ACP agents and authentication for this workspace.
        </p>
      </div>

      {isLoading ? (
        <SkeletonRows count={3} />
      ) : error ? (
        <p role="alert" className="py-2 text-[13px] text-danger">
          {error instanceof Error ? error.message : 'Could not load ACP agents.'}
        </p>
      ) : specs.length === 0 ? (
        <p className="py-2 text-[13px] text-ink-3">No personal ACP agents are configured for this workspace.</p>
      ) : (
        <div className="flex flex-col gap-3">
          {specs.map((spec) => {
            const viaDesktop = !spec.supports_web_auth && !cloudOnly && supportsDesktopCloudLogin(spec.agent)
            const canLogin = spec.supports_web_auth || viaDesktop
            const loginReason = canLogin
              ? ''
              : supportsDesktopCloudLogin(spec.agent)
                ? `Sign in from Jaz Desktop to connect ${agentLabel(spec.agent)}.`
                : `${agentLabel(spec.agent)} sign-in isn't available in Jaz Cloud yet.`
            return (
              <CloudACPAgentRow
                key={spec.agent}
                spec={spec}
                profile={profilesByAgent.get(spec.agent)}
                loginJob={loginJobs[spec.agent]}
                canLogin={canLogin}
                loginReason={loginReason}
                loginPending={startLogin.isPending && startLogin.variables?.agent === spec.agent}
                disconnecting={disconnect.isPending && disconnect.variables === spec.agent}
                onStartLogin={() => startLogin.mutate({ agent: spec.agent, viaDesktop })}
                onDisconnect={() => disconnect.mutate(spec.agent)}
              />
            )
          })}
        </div>
      )}
    </section>
  )
}

function CloudACPAgentRow({
  spec,
  profile,
  loginJob,
  canLogin,
  loginReason,
  loginPending,
  disconnecting,
  onStartLogin,
  onDisconnect,
}: {
  spec: CloudACPAgentSpec
  profile?: CloudACPProfile
  loginJob?: ACPAuthLogin
  canLogin: boolean
  loginReason: string
  loginPending: boolean
  disconnecting: boolean
  onStartLogin: () => void
  onDisconnect: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const agent = spec.agent
  const connected = profile?.status === 'connected' || profile?.status === 'refreshing'
  const needsAuth = profile?.status === 'needs_auth' || profile?.status === 'error'
  const running = loginPending || loginJob?.status === 'running'
  const hint = needsAuth
    ? (profile?.last_error || `Sign in with ${authProviderLabel(agent)} again.`)
    : loginReason || `Connect ${authProviderLabel(agent)} to use this agent.`

  return (
    <SettingsCard className="overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        aria-expanded={expanded}
        aria-label={`${expanded ? 'Collapse' : 'Expand'} ${agentLabel(agent)}`}
        className="group flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <ChevronDown
          size={15}
          className={`shrink-0 text-ink-3 transition-transform duration-150 group-hover:text-ink ${expanded ? '' : '-rotate-90'}`}
        />
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
          <AgentAvatar agent={agent} size={18} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium text-ink">{agentLabel(agent)}</span>
          {connected ? null : (
            <span className="mt-0.5 flex min-w-0 items-center gap-1 text-[12px] text-ink-3">
              <CircleAlert size={12} className="shrink-0 text-accent" />
              <span className="truncate">{hint}</span>
            </span>
          )}
        </span>
        {connected ? <CheckCircle2 size={15} className="shrink-0 text-primary" /> : null}
      </button>

      <Collapse open={expanded}>
        <div className="border-t border-border/70 px-3 py-2.5">
          {connected ? (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-control bg-bg px-3 py-2.5">
              <span className="flex min-w-0 items-center gap-2 text-[13px] text-ink">
                <CheckCircle2 size={16} className="shrink-0 text-primary" />
                <span className="truncate">
                  {`Connected with your ${authProviderLabel(agent)} account`}
                  {profile?.account_label ? ` (${profile.account_label})` : ''}
                </span>
              </span>
              <Button variant="ghost" size="sm" disabled={disconnecting} onClick={onDisconnect}>
                {disconnecting ? <LoaderCircle size={13} className="animate-spin" /> : null}
                Disconnect
              </Button>
            </div>
          ) : needsAuth ? (
            <div className="flex flex-col gap-2 rounded-control bg-bg px-3 py-2.5">
              <div className="flex items-start gap-2 text-[13px] text-ink">
                <CircleAlert size={16} className="mt-0.5 shrink-0 text-accent" />
                <div className="min-w-0">
                  <p className="font-medium">Sign-in required</p>
                  <p className="mt-0.5 text-[12px] text-ink-3">
                    {profile?.last_error || `Sign in with ${authProviderLabel(agent)} again.`}
                  </p>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {canLogin && !running ? (
                  <Button variant="primary" size="md" onClick={onStartLogin}>
                    <LogIn size={14} />
                    Sign in again
                  </Button>
                ) : null}
                <Button variant="ghost" size="sm" disabled={disconnecting} onClick={onDisconnect}>
                  {disconnecting ? <LoaderCircle size={13} className="animate-spin" /> : null}
                  Disconnect
                </Button>
              </div>
              {!canLogin && loginReason ? <p className="text-[12px] text-ink-3">{loginReason}</p> : null}
              <AuthLoginStatus job={loginJob} running={running} allowCodeInput={false} />
            </div>
          ) : (
            <div className="flex flex-col items-start gap-2">
              {running ? null : (
                <Button variant="primary" size="md" disabled={!canLogin} onClick={onStartLogin}>
                  <LogIn size={14} />
                  {`Sign in with ${authProviderLabel(agent)}`}
                </Button>
              )}
              {!canLogin && loginReason ? <p className="text-[12px] text-ink-3">{loginReason}</p> : null}
              <div className="w-full">
                <AuthLoginStatus job={loginJob} running={running} allowCodeInput={false} />
              </div>
            </div>
          )}
        </div>
      </Collapse>
    </SettingsCard>
  )
}
