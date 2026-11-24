import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  CircleAlert,
  ExternalLink,
  LoaderCircle,
  LogIn,
  RefreshCw,
  Trash2,
  Upload,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  cloudACPProfilesQuery,
  deleteCloudACPProfile,
  getCloudACPLogin,
  refreshCloudACPProfile,
  startCloudACPLogin,
  syncLocalACPProfileToCloud,
  type CloudACPAgentSpec,
  type CloudACPLoginSession,
  type CloudACPProfile,
} from '@/lib/api/cloudACP'
import { useIsCloudOnly } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'
import { useCopyAction } from '@/lib/useCopyAction'

const LOGIN_POLL_MS = 1_500

function formatTimestamp(iso?: string | null): string {
  if (!iso) return ''
  const ms = Date.parse(iso)
  if (Number.isNaN(ms)) return ''
  return new Date(ms).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function statusPill(profile?: CloudACPProfile): { label: string; tone: string } {
  if (!profile) return { label: 'Not connected', tone: 'text-ink-3' }
  switch (profile.status) {
    case 'connected':
      return { label: 'Connected', tone: 'text-ok' }
    case 'refreshing':
      return { label: 'Refreshing…', tone: 'text-ink-2' }
    case 'needs_auth':
      return { label: 'Sign-in required', tone: 'text-danger' }
    case 'error':
      return { label: 'Error', tone: 'text-danger' }
  }
}

export function CloudACPSettings() {
  const cloudOnly = useIsCloudOnly()
  const queryClient = useQueryClient()
  const toast = useToast()
  const { data, isLoading, error } = useQuery(cloudACPProfilesQuery)
  const [clearLocalOnSync, setClearLocalOnSync] = useState(false)
  const [activeLogins, setActiveLogins] = useState<Record<string, CloudACPLoginSession>>({})

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: keys.cloudACPProfiles })
    void queryClient.invalidateQueries({ queryKey: keys.cloudModelProfiles })
  }

  // Poll any active device-code login sessions until terminal status.
  const activeKey = Object.values(activeLogins)
    .filter((s) => s.status === 'pending' || s.status === 'awaiting_user')
    .map((s) => `${s.agent}:${s.id}`)
    .join(',')

  useEffect(() => {
    if (!activeKey) return
    let cancelled = false
    const entries = activeKey.split(',').map((pair) => {
      const [agent, id] = pair.split(':')
      return { agent, id }
    })

    const poll = async () => {
      for (const { agent, id } of entries) {
        try {
          const updated = await getCloudACPLogin(agent, id)
          if (cancelled) return
          setActiveLogins((prev) => ({ ...prev, [agent]: updated }))
          if (updated.status === 'completed') {
            toast(`Connected ${agent} to Jaz Cloud`)
            void queryClient.invalidateQueries({ queryKey: keys.cloudACPProfiles })
            void queryClient.invalidateQueries({ queryKey: keys.cloudModelProfiles })
          }
        } catch {
          // Transient poll failure; next tick retries.
        }
      }
    }

    const timer = window.setInterval(() => {
      void poll()
    }, LOGIN_POLL_MS)
    void poll()
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [activeKey, queryClient, toast])

  const startLogin = useMutation({
    mutationFn: (agent: string) => startCloudACPLogin(agent),
    onSuccess: (session) => {
      setActiveLogins((prev) => ({ ...prev, [session.agent]: session }))
    },
    onError: (err) => {
      toast(err instanceof Error ? err.message : 'Could not start sign-in', 'danger')
    },
  })

  const syncLocal = useMutation({
    mutationFn: (agent: string) => syncLocalACPProfileToCloud(agent, { clearLocal: clearLocalOnSync }),
    onSuccess: (res) => {
      const suffix = res.cleared_local ? ' (local Jaz copy cleared)' : ''
      toast(`Synced ${res.profile.display_name} to Jaz Cloud${suffix}`)
      invalidate()
    },
    onError: (err) => {
      toast(err instanceof Error ? err.message : 'Could not sync local credentials', 'danger')
    },
  })

  const refresh = useMutation({
    mutationFn: (agent: string) => refreshCloudACPProfile(agent, true),
    onSuccess: (res) => {
      toast(`Refreshing ${res.profile.display_name}…`)
      invalidate()
    },
    onError: (err) => {
      toast(err instanceof Error ? err.message : 'Could not queue refresh', 'danger')
    },
  })

  const disconnect = useMutation({
    mutationFn: (agent: string) => deleteCloudACPProfile(agent),
    onSuccess: (res) => {
      setActiveLogins((prev) => {
        const next = { ...prev }
        delete next[res.agent]
        return next
      })
      toast(`Disconnected ${res.agent} from Jaz Cloud`)
      invalidate()
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
          Connect your personal coding-agent subscriptions to Jaz Cloud. Credentials are encrypted per user, never
          shared with teammates, and kept fresh automatically so threads continue from web and mobile.
        </p>
      </div>

      {!cloudOnly ? (
        <label className="flex cursor-pointer items-center gap-2 text-[12.5px] text-ink-2 select-none">
          <input
            type="checkbox"
            checked={clearLocalOnSync}
            onChange={(e) => setClearLocalOnSync(e.target.checked)}
            className="size-3.5 accent-accent"
          />
          <span>Clear Jaz&apos;s local copy after syncing so Jaz Cloud is the sole token refresher</span>
        </label>
      ) : null}

      {isLoading ? (
        <SettingsCard className="p-3">
          <SkeletonRows count={3} />
        </SettingsCard>
      ) : error ? (
        <SettingsCard className="p-3">
          <p role="alert" className="text-[13px] text-danger">
            {error instanceof Error ? error.message : 'Could not load Cloud ACP profiles.'}
          </p>
        </SettingsCard>
      ) : specs.length === 0 ? (
        <SettingsCard className="p-3">
          <p className="text-[13px] text-ink-2">
            This Jaz Cloud workspace does not expose personal ACP profiles yet.
          </p>
        </SettingsCard>
      ) : (
        <div className="flex flex-col gap-3">
          {specs.map((spec) => (
            <CloudACPAgentCard
              key={spec.agent}
              spec={spec}
              profile={profilesByAgent.get(spec.agent)}
              loginSession={activeLogins[spec.agent]}
              cloudOnly={cloudOnly}
              busy={
                (startLogin.isPending && startLogin.variables === spec.agent) ||
                (syncLocal.isPending && syncLocal.variables === spec.agent) ||
                (refresh.isPending && refresh.variables === spec.agent) ||
                (disconnect.isPending && disconnect.variables === spec.agent)
              }
              onStartLogin={() => startLogin.mutate(spec.agent)}
              onSyncLocal={() => syncLocal.mutate(spec.agent)}
              onRefresh={() => refresh.mutate(spec.agent)}
              onDisconnect={() => disconnect.mutate(spec.agent)}
            />
          ))}
        </div>
      )}
    </section>
  )
}

function CloudACPAgentCard({
  spec,
  profile,
  loginSession,
  cloudOnly,
  busy,
  onStartLogin,
  onSyncLocal,
  onRefresh,
  onDisconnect,
}: {
  spec: CloudACPAgentSpec
  profile?: CloudACPProfile
  loginSession?: CloudACPLoginSession
  cloudOnly: boolean
  busy: boolean
  onStartLogin: () => void
  onSyncLocal: () => void
  onRefresh: () => void
  onDisconnect: () => void
}) {
  const { copied, copy } = useCopyAction(loginSession?.user_code ?? '')
  const pill = statusPill(profile)
  const isLoggingIn = loginSession?.status === 'pending' || loginSession?.status === 'awaiting_user'
  const expiresLabel = formatTimestamp(profile?.token_expires_at)
  const refreshedLabel = formatTimestamp(profile?.last_refreshed_at)

  return (
    <SettingsCard className="overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <AgentAvatar agent={spec.agent} size={28} />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate text-[13.5px] font-medium text-ink">{spec.display_name}</span>
              <span className={`inline-flex items-center gap-1 text-[11.5px] font-medium ${pill.tone}`}>
                {profile?.status === 'connected' ? <CheckCircle2 size={12} /> : null}
                {profile?.status === 'refreshing' ? <LoaderCircle size={12} className="animate-spin" /> : null}
                {profile?.status === 'needs_auth' || profile?.status === 'error' ? <CircleAlert size={12} /> : null}
                {pill.label}
              </span>
            </div>
            <p className="mt-0.5 truncate text-[12px] text-ink-3">
              {[
                profile?.account_label,
                expiresLabel ? `Expires ${expiresLabel}` : '',
                refreshedLabel ? `Refreshed ${refreshedLabel}` : '',
              ]
                .filter(Boolean)
                .join(' · ') ||
                (spec.supports_web_auth
                  ? 'Supports direct browser sign-in or Desktop sync'
                  : 'Sign in on Jaz Desktop once, then sync to Jaz Cloud')}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          {spec.supports_web_auth ? (
            <Button size="sm" variant={profile ? 'ghost' : 'primary'} disabled={busy || isLoggingIn} onClick={onStartLogin}>
              {isLoggingIn ? <LoaderCircle size={13} className="animate-spin" /> : <LogIn size={13} />}
              {profile ? 'Re-authenticate' : 'Sign in'}
            </Button>
          ) : null}

          {!cloudOnly ? (
            <Button size="sm" variant={profile || spec.supports_web_auth ? 'ghost' : 'primary'} disabled={busy} onClick={onSyncLocal}>
              <Upload size={13} />
              Sync local
            </Button>
          ) : null}

          {profile ? (
            <>
              <Button size="sm" variant="ghost" disabled={busy || profile.status === 'refreshing'} onClick={onRefresh}>
                <RefreshCw size={13} className={profile.status === 'refreshing' ? 'animate-spin' : ''} />
                Refresh
              </Button>
              <Button size="sm" variant="ghost" disabled={busy} onClick={onDisconnect} aria-label={`Disconnect ${spec.display_name}`}>
                <Trash2 size={13} />
              </Button>
            </>
          ) : null}
        </div>
      </div>

      {profile?.last_error ? (
        <div className="border-t border-border bg-surface-1 px-3.5 py-2">
          <p role="alert" className="text-[12px] text-danger">
            {profile.last_error}
          </p>
        </div>
      ) : null}

      {loginSession && loginSession.status !== 'completed' ? (
        <div className="border-t border-border bg-surface-1 px-3.5 py-3">
          {loginSession.status === 'failed' || loginSession.status === 'expired' ? (
            <p role="alert" className="text-[12px] text-danger">
              {loginSession.error || `Sign-in ${loginSession.status}. Try again.`}
            </p>
          ) : (
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2 text-[12.5px] text-ink-2">
                <LoaderCircle size={13} className="animate-spin text-ink-3" />
                <span>
                  {loginSession.user_code
                    ? loginSession.instructions || 'Open the verification link and enter this code:'
                    : 'Starting device sign-in on Jaz Cloud…'}
                </span>
              </div>
              {loginSession.user_code || loginSession.verification_url ? (
                <div className="flex flex-wrap items-center gap-2">
                  {loginSession.user_code ? (
                    <button
                      type="button"
                      onClick={copy}
                      className="inline-flex items-center gap-1.5 rounded-md border border-border bg-bg px-2.5 py-1 font-mono text-[13px] font-semibold tracking-wider text-ink hover:bg-surface-2"
                    >
                      <span>{loginSession.user_code}</span>
                      <CopyToggleIcon copied={copied} />
                    </button>
                  ) : null}
                  {loginSession.verification_url ? (
                    <a
                      href={loginSession.verification_url}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 rounded-md bg-accent px-2.5 py-1 text-[12.5px] font-medium text-accent-fg hover:opacity-90"
                    >
                      <span>Open sign-in page</span>
                      <ExternalLink size={12} />
                    </a>
                  ) : null}
                </div>
              ) : null}
            </div>
          )}
        </div>
      ) : null}
    </SettingsCard>
  )
}
