import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, LoaderCircle } from 'lucide-react'
import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Collapse } from '@/components/ui/Collapse'
import { Input } from '@/components/ui/Input'
import { SkeletonRows } from '@/components/ui/Skeleton'
import {
  cloudACPProfilesQuery,
  completeCloudACPLogin,
  deleteCloudACPProfile,
  getCloudACPLogin,
  refreshCloudACPProfile,
  saveCloudACPKey,
  startCloudACPLogin,
  toACPAuthLogin,
  type CloudACPAgentSpec,
  type CloudACPProfile,
} from '@/lib/api/cloudACP'
import type { ACPAuthLoginChallenge } from '@/lib/api/types'
import { clientRuntime } from '@/lib/clientRuntime'
import { useCloudCatalogScope } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

export function CloudACPSettings() {
  const { account, workspaceID } = useCloudCatalogScope()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({
    ...cloudACPProfilesQuery(account, workspaceID),
    refetchOnMount: 'always',
    refetchInterval: (query) =>
      query.state.data?.profiles.some((profile) => profile.status === 'refreshing') ? 2_000 : false,
  })
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: keys.cloudACPProfiles(account) }),
      queryClient.invalidateQueries({ queryKey: keys.cloudModelProfiles(account) }),
    ])
  }
  const specs = data?.supported_agents.filter((spec) => spec.executable !== false) ?? []

  return (
    <section className="flex flex-col gap-5 py-4">
      <div>
        <h1 className="text-lg font-semibold text-ink">Agents (ACP)</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">
          Connect your agent accounts for this Cloud workspace.
        </p>
      </div>
      {isLoading && <SkeletonRows count={4} />}
      {error && <p role="alert" className="text-[13px] text-danger">{error.message}</p>}
      {data && specs.length === 0 && <p className="text-[13px] text-ink-3">No personal agents are available.</p>}
      <div key={account} className="flex flex-col gap-3">
        {specs.map((spec) => (
          <CloudACPAgentRow
            key={spec.agent}
            spec={spec}
            profile={data?.profiles.find((profile) => profile.agent === spec.agent)}
            workspaceID={workspaceID}
            refresh={refresh}
          />
        ))}
      </div>
    </section>
  )
}

const signInLabels = {
  device_code: 'Sign in with ChatGPT',
  oauth_code: 'Sign in with Claude',
  oauth_redirect: 'Sign in with Google',
}

function CloudACPAgentRow({ spec, profile, workspaceID, refresh }: {
  spec: CloudACPAgentSpec
  profile?: CloudACPProfile
  workspaceID: string
  refresh: () => Promise<void>
}) {
  const [expanded, setExpanded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [login, setLogin] = useState<ACPAuthLoginChallenge>()
  const operation = useRef<AbortController | undefined>(undefined)
  const keyId = useId()
  const projectId = useId()
  const locationId = useId()
  useEffect(() => () => operation.current?.abort(), [])

  const connectKey = spec.agent === 'grok' && spec.credential_kind === 'api_key'
  const authKind = spec.supports_web_auth ? spec.web_auth_kind : undefined
  const connected = profile?.credential_kind === spec.credential_kind &&
    (profile.status === 'connected' || profile.status === 'refreshing')
  const running = login?.status === 'running'
  const status = connected ? (profile?.status === 'refreshing' ? 'Refreshing' : 'Connected') : profile ? 'Needs authentication' : 'Disconnected'

  async function run(action: (signal: AbortSignal) => Promise<void>) {
    if (operation.current) return
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setError('')
    try {
      await action(controller.signal)
    } catch (cause) {
      if (!controller.signal.aborted) throw cause
    } finally {
      if (operation.current === controller) {
        operation.current = undefined
        setBusy(false)
      }
    }
  }

  function perform(action: (signal: AbortSignal) => Promise<void>) {
    void run(action).catch((cause: unknown) => {
      setError(cause instanceof Error ? cause.message : 'The request failed. Try again.')
    })
  }

  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const key = String(new FormData(event.currentTarget).get('apiKey') ?? '').trim()
    if (!key) return
    event.currentTarget.reset()
    perform(async (signal) => {
      await saveCloudACPKey(spec.agent, key, workspaceID, signal)
      await refresh()
    })
  }

  function signIn(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const fields = new FormData(event.currentTarget)
    const options = authKind === 'oauth_redirect' ? {
      gcp_project: String(fields.get('gcpProject') ?? '').trim() || undefined,
      gcp_location: String(fields.get('gcpLocation') ?? '').trim() || undefined,
    } : undefined
    setLogin(undefined)
    perform(async (signal) => {
      try {
        let session = await startCloudACPLogin(spec.agent, workspaceID, signal, options)
        while (!signal.aborted) {
          const next = toACPAuthLogin(session)
          setLogin(next)
          if (next.status === 'succeeded') await refresh()
          if (next.status !== 'running' || authKind === 'oauth_code') return
          await new Promise((resolve) => window.setTimeout(resolve, 1_000))
          if (signal.aborted) return
          session = await getCloudACPLogin(spec.agent, session.id, workspaceID, signal)
        }
      } catch (cause) {
        if (!signal.aborted) setLogin(undefined)
        throw cause
      }
    })
  }

  function cancelSignIn() {
    operation.current?.abort()
    operation.current = undefined
    setLogin(undefined)
    setBusy(false)
  }

  function completeSignIn(id: string, code: string) {
    return run(async (signal) => {
      const session = await completeCloudACPLogin(spec.agent, id, code, workspaceID, signal)
      if (signal.aborted) return
      setLogin(toACPAuthLogin(session))
      await refresh()
    })
  }

  function refreshCredentials() {
    perform(async (signal) => {
      try {
        await refreshCloudACPProfile(spec.agent, workspaceID, signal)
      } finally {
        if (!signal.aborted) await refresh()
      }
    })
  }

  function disconnect() {
    perform(async (signal) => {
      await deleteCloudACPProfile(spec.agent, workspaceID, signal)
      if (!signal.aborted) setLogin(undefined)
      await refresh()
    })
  }

  return (
    <SettingsCard className="overflow-hidden">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <ChevronDown size={15} className={`shrink-0 text-ink-3 transition-transform ${expanded ? '' : '-rotate-90'}`} />
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
          <AgentAvatar agent={spec.agent} size={18} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[13px] font-medium text-ink">{spec.display_name}</span>
          <span className="block text-[12px] text-ink-3">
            {status}{profile?.account_label && connected ? ` · ${profile.account_label}` : ''}
          </span>
        </span>
        {connected && <CheckCircle2 size={15} className="shrink-0 text-primary" />}
      </button>
      <Collapse open={expanded}>
        <div className="flex flex-col gap-2.5 border-t border-border/70 px-3 py-2.5">
          {connectKey ? (
            <form onSubmit={save} className="flex flex-col gap-1.5">
              <label htmlFor={keyId} className="text-[13px] font-medium text-ink">xAI API key</label>
              <div className="flex flex-wrap items-center gap-2">
                <input
                  id={keyId}
                  name="apiKey"
                  type="password"
                  autoComplete="off"
                  required
                  disabled={busy}
                  className="min-w-0 flex-1 rounded-control border border-border bg-bg px-2.5 py-1.5 text-[13px] text-ink"
                />
                <Button type="submit" variant="primary" size="sm" disabled={busy}>
                  {connected ? 'Replace key' : 'Save key'}
                </Button>
              </div>
            </form>
          ) : !connected && authKind && (
            <>
              {!running && (
                <form onSubmit={signIn} className="grid gap-2">
                  {authKind === 'oauth_redirect' && (
                    <div className="grid gap-2 sm:grid-cols-2">
                      <div className="grid gap-1">
                        <label htmlFor={projectId} className="text-[13px] text-ink-2">GCP project ID (optional)</label>
                        <Input id={projectId} name="gcpProject" spellCheck={false} disabled={busy} />
                      </div>
                      <div className="grid gap-1">
                        <label htmlFor={locationId} className="text-[13px] text-ink-2">GCP location (optional)</label>
                        <Input id={locationId} name="gcpLocation" spellCheck={false} disabled={busy} />
                      </div>
                    </div>
                  )}
                  <Button type="submit" variant="primary" size="sm" className="justify-self-start" disabled={busy}>
                    {signInLabels[authKind]}
                  </Button>
                </form>
              )}
              {running && authKind === 'oauth_redirect' && clientRuntime.kind === 'electron' && (
                <p className="text-[12px] text-ink-2">Use a browser where you’re signed in to this Jaz Cloud account.</p>
              )}
              <AuthLoginStatus key={login?.id} job={login} running={busy} submitCode={authKind === 'oauth_code' && completeSignIn} />
            </>
          )}
          <div className="flex flex-wrap items-center gap-2">
            {running && <Button variant="ghost" size="sm" disabled={authKind === 'oauth_code' && busy} onClick={cancelSignIn}>{authKind === 'oauth_code' ? 'Cancel sign-in' : 'Stop waiting'}</Button>}
            {connected && authKind && <Button variant="ghost" size="sm" disabled={busy || profile?.status === 'refreshing'} onClick={refreshCredentials}>Refresh credentials</Button>}
            {profile && <Button variant="ghost" size="sm" disabled={busy || running} onClick={disconnect}>Disconnect</Button>}
            {busy && (connected || connectKey) && <LoaderCircle size={14} aria-label="Working" className="animate-spin text-ink-3" />}
          </div>
          {error && <p role="alert" className="text-[12px] text-danger">{error}</p>}
        </div>
      </Collapse>
    </SettingsCard>
  )
}
