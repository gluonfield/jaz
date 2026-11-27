import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, LoaderCircle } from 'lucide-react'
import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Collapse } from '@/components/ui/Collapse'
import { SkeletonRows } from '@/components/ui/Skeleton'
import {
  cloudACPProfilesQuery,
  deleteCloudACPProfile,
  getCloudACPLogin,
  saveCloudACPKey,
  startCloudACPLogin,
  toACPAuthLogin,
  type CloudACPAgentSpec,
  type CloudACPProfile,
} from '@/lib/api/cloudACP'
import type { ACPAuthLogin } from '@/lib/api/types'
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
          Connect an agent for Cloud runs and native runs on this workspace’s paired Desktop.
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

const keyLabels: Record<string, string> = {
  claude: 'Anthropic Console API key',
  antigravity: 'Gemini Developer API key',
  grok: 'xAI API key',
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
  const [login, setLogin] = useState<ACPAuthLogin>()
  const operation = useRef<AbortController | undefined>(undefined)
  const keyId = useId()
  useEffect(() => () => operation.current?.abort(), [])

  const connectKey = spec.credential_kind === 'api_key'
  const connected = profile?.status === 'connected' || profile?.status === 'refreshing'
  const running = login?.status === 'running'
  const status = connected ? 'Connected' : profile ? 'Needs authentication' : 'Disconnected'

  async function perform(action: (signal: AbortSignal) => Promise<void>, errorMessage: string) {
    if (operation.current) return
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setError('')
    try {
      await action(controller.signal)
    } catch {
      if (!controller.signal.aborted) setError(errorMessage)
    } finally {
      operation.current = undefined
      if (!controller.signal.aborted) setBusy(false)
    }
  }

  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const key = String(new FormData(event.currentTarget).get('apiKey') ?? '').trim()
    if (!key) return
    event.currentTarget.reset()
    void perform(async (signal) => {
      await saveCloudACPKey(spec.agent, key, workspaceID, signal)
      await refresh()
    }, 'Could not save the API key. Try again.')
  }

  function signIn() {
    void perform(async (signal) => {
      const session = await startCloudACPLogin(workspaceID, signal)
      let next = toACPAuthLogin(session)
      while (!signal.aborted) {
        setLogin(next)
        if (next.status !== 'running') break
        await new Promise((resolve) => window.setTimeout(resolve, 1_000))
        if (signal.aborted) return
        const polled = await getCloudACPLogin(session.id, workspaceID, signal)
        next = toACPAuthLogin(polled)
      }
      if (next.status === 'succeeded') await refresh()
    }, 'Could not complete Codex sign-in. Try again.')
  }

  function disconnect() {
    void perform(async (signal) => {
      await deleteCloudACPProfile(spec.agent, workspaceID, signal)
      if (!signal.aborted) setLogin(undefined)
      await refresh()
    }, 'Could not disconnect the agent. Try again.')
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
              <label htmlFor={keyId} className="text-[13px] font-medium text-ink">{keyLabels[spec.agent] ?? 'API key'}</label>
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
          ) : !connected && spec.supports_web_auth && (
            <div>
              <Button variant="primary" size="sm" disabled={busy} onClick={signIn}>Sign in with ChatGPT</Button>
              <AuthLoginStatus job={login} running={Boolean(running && busy)} allowCodeInput={false} />
            </div>
          )}
          <div className="flex items-center gap-2">
            {profile && <Button variant="ghost" size="sm" disabled={busy} onClick={disconnect}>Disconnect</Button>}
            {busy && <LoaderCircle size={14} aria-label="Working" className="animate-spin text-ink-3" />}
            {error && <p role="alert" className="text-[12px] text-danger">{error}</p>}
          </div>
        </div>
      </Collapse>
    </SettingsCard>
  )
}
