import {
  ArrowLeftRight,
  ChartNoAxesColumn,
  Cloud,
  Laptop,
  LoaderCircle,
  LogIn,
  LogOut,
  Plus,
  RefreshCw,
  Sparkles,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useToast } from '@/components/ui/toast'
import { clientRuntime } from '@/lib/clientRuntime'
import {
  attachCloudSignIn,
  LOCAL_WORKSPACE_ID,
  signOutCloudAccount,
  switchWorkspace,
  syncCloudAccount,
  type CloudWorkspaceSummary,
} from '@/lib/cloudAccount'
import { DEFAULT_CLOUD_URL, useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { activeCloudWorkspace, workspaceDisplayName, workspaceInitials } from '@/lib/cloudWorkspaceDisplay'
import { disconnectBackend, startCloudSignIn } from '@/lib/connection'
import type { SettingsSection } from './sections'

function defaultAddFormUrl(workspaces: CloudWorkspaceSummary[]): string {
  const candidate = clientRuntime.cloudUrl || DEFAULT_CLOUD_URL
  return workspaces.some((ws) => ws.url === candidate) ? '' : candidate
}

export function CloudSettings({
  addCloudRequestId = 0,
  onNavigate,
}: {
  addCloudRequestId?: number
  onNavigate: (section: SettingsSection) => void
}) {
  const cloud = useCloudSession()
  const isPrimaryOnly = useIsCloudOnly()
  const toast = useToast()

  const workspaces = cloud.workspaces
  const activeCloud = activeCloudWorkspace(cloud)
  const isLocalActive = !isPrimaryOnly && !activeCloud

  const [showAddForm, setShowAddForm] = useState(addCloudRequestId > 0)
  const [url, setUrl] = useState(() => defaultAddFormUrl(workspaces))
  const [busyTarget, setBusyTarget] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [awaitingBrowser, setAwaitingBrowser] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const prevWorkspaceCount = useRef(workspaces.length)
  const lastAddRequestId = useRef(addCloudRequestId)

  useEffect(() => {
    if (addCloudRequestId > 0 && addCloudRequestId !== lastAddRequestId.current) {
      lastAddRequestId.current = addCloudRequestId
      setShowAddForm(true)
      setUrl(defaultAddFormUrl(workspaces))
      setFormError(null)
      setAwaitingBrowser(false)
    }
  }, [addCloudRequestId, workspaces])

  useEffect(() => {
    if (workspaces.length > prevWorkspaceCount.current) {
      setAwaitingBrowser(false)
      setFormError(null)
      setShowAddForm(false)
    }
    prevWorkspaceCount.current = workspaces.length
  }, [workspaces.length])

  useEffect(() => {
    if (cloud.active && awaitingBrowser && isPrimaryOnly) {
      setAwaitingBrowser(false)
      setFormError(null)
    }
  }, [awaitingBrowser, cloud.active, isPrimaryOnly])

  const startSignIn = async (targetUrl: string, loginPath?: string, targetKey = 'add') => {
    const trimmed = targetUrl.trim()
    if (!trimmed) return
    setBusyTarget(targetKey)
    setFormError(null)
    try {
      const err = isPrimaryOnly
        ? await startCloudSignIn(trimmed, loginPath)
        : await attachCloudSignIn(trimmed, loginPath)
      if (err) {
        setFormError(err)
        return
      }
      if (clientRuntime.kind === 'electron') {
        setAwaitingBrowser(true)
      }
    } catch (error) {
      setFormError(error instanceof Error ? error.message : 'Could not start Jaz Cloud sign-in')
    } finally {
      setBusyTarget(null)
    }
  }

  const handleSwitch = async (workspaceId: string, label: string) => {
    setBusyTarget(`switch:${workspaceId}`)
    setFormError(null)
    try {
      const err = await switchWorkspace(workspaceId)
      if (err) {
        setFormError(err)
        return
      }
      toast(`Switched to ${label}`)
    } finally {
      setBusyTarget(null)
    }
  }

  const handleSync = async () => {
    setSyncing(true)
    setFormError(null)
    try {
      const err = await syncCloudAccount()
      if (err) {
        setFormError(err)
      } else {
        toast('Synced Jaz Cloud')
      }
    } finally {
      setSyncing(false)
    }
  }

  const handleSignOut = async (workspace: CloudWorkspaceSummary) => {
    setAwaitingBrowser(false)
    setFormError(null)
    if (isPrimaryOnly) {
      disconnectBackend()
      return
    }
    setBusyTarget(`signout:${workspace.id}`)
    try {
      await signOutCloudAccount(workspace.id)
      toast(`Signed out of ${workspaceDisplayName(workspace)}`)
    } finally {
      setBusyTarget(null)
    }
  }

  const openAddForm = () => {
    setShowAddForm(true)
    setUrl(defaultAddFormUrl(workspaces))
    setFormError(null)
    setAwaitingBrowser(false)
  }

  const orderedCloudWorkspaces = activeCloud
    ? [activeCloud, ...workspaces.filter((ws) => ws.id !== activeCloud.id)]
    : workspaces
  const showConnectCard = workspaces.length === 0 || (!isPrimaryOnly && showAddForm)
  const busy = busyTarget !== null

  return (
    <section className="flex flex-col gap-5 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-ink">Workspace</h1>
          <p className="mt-0.5 text-[13px] text-ink-2">
            {isPrimaryOnly
              ? 'Your active Jaz Cloud workspace and organization identity.'
              : activeCloud
                ? 'Manage your active workspace, sync, and connected Jaz Cloud workspaces.'
                : 'Manage your workspace or connect Jaz Cloud workspaces.'}
          </p>
        </div>
        {!isPrimaryOnly && workspaces.length > 0 && !showAddForm ? (
          <Button size="sm" variant="secondary" disabled={busy} onClick={openAddForm}>
            <Plus size={13} />
            Add Cloud workspace
          </Button>
        ) : null}
      </div>

      {isLocalActive ? (
        <SettingsCard className="overflow-hidden">
          <div className="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3">
            <LocalWorkspaceIdentity />
            <ActiveBadge label="Active" />
          </div>
          <SettingsRow
            title="Storage"
            description="Threads, bots, boards, loops, AGENTS, plugins, and MCP servers live on this computer."
          >
            <span className="text-[12px] text-ink-3">On device</span>
          </SettingsRow>
        </SettingsCard>
      ) : null}

      {showConnectCard ? (
        <SettingsCard className="p-4">
          <form
            onSubmit={(event) => {
              event.preventDefault()
              void startSignIn(url)
            }}
            className="flex flex-col gap-3"
          >
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Cloud size={15} className="shrink-0 text-primary" />
                <p className="text-[13.5px] font-medium text-ink">
                  {workspaces.length > 0 ? 'Add Cloud workspace' : 'Connect Jaz Cloud'}
                </p>
              </div>
              {!isPrimaryOnly && workspaces.length > 0 ? (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  onClick={() => {
                    setShowAddForm(false)
                    setAwaitingBrowser(false)
                    setFormError(null)
                  }}
                >
                  Cancel
                </Button>
              ) : null}
            </div>
            <p className="text-[12.5px] text-ink-2">
              {isPrimaryOnly
                ? 'Enter your Jaz Cloud HTTPS address and sign in with SSO.'
                : 'Enter a Jaz Cloud HTTPS address and sign in with SSO. Each workspace keeps its own threads, bots, boards, Spaces, and settings.'}
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <Input
                autoFocus={showAddForm && workspaces.length > 0}
                value={url}
                onChange={(event) => {
                  setUrl(event.target.value)
                  if (formError) setFormError(null)
                }}
                placeholder={DEFAULT_CLOUD_URL}
                aria-label="Jaz Cloud server URL"
                spellCheck={false}
                disabled={busy}
                className="h-9 min-w-[240px] flex-1 font-mono text-[12.5px]"
              />
              <Button type="submit" variant="primary" disabled={busy || !url.trim()}>
                {busyTarget === 'add' || (workspaces.length === 0 && cloud.status === 'checking') ? (
                  <LoaderCircle size={14} className="animate-spin" />
                ) : (
                  <LogIn size={14} />
                )}
                {awaitingBrowser ? 'Reopen browser sign-in' : 'Sign in with SSO'}
              </Button>
            </div>
            {awaitingBrowser ? (
              <p className="text-[12px] text-ink-3">
                Complete sign-in in your browser — Jaz will connect the workspace automatically.
              </p>
            ) : null}
            {formError || (workspaces.length === 0 && cloud.error) ? (
              <p role="alert" className="text-[12px] text-danger">
                {formError || cloud.error}
              </p>
            ) : null}
          </form>
        </SettingsCard>
      ) : null}

      {orderedCloudWorkspaces.length > 0 ? (
        <div className="flex flex-col gap-3">
          {!isPrimaryOnly && isLocalActive ? (
            <p className="text-[12px] font-medium text-ink-3">Cloud workspaces</p>
          ) : null}
          {orderedCloudWorkspaces.map((ws) => {
            const isActive = isPrimaryOnly
              ? cloud.active
              : cloud.active && ws.id === cloud.activeWorkspaceId
            const syncTime = ws.last_sync_at || (isActive ? cloud.lastSyncAt : '')
            const rowError = ws.last_error || (isActive ? cloud.error : null)
            const name = workspaceDisplayName(ws)
            const reauthKey = `reauth:${ws.id}`
            const switchKey = `switch:${ws.id}`
            const signoutKey = `signout:${ws.id}`
            return (
              <SettingsCard key={ws.id} className="overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3">
                  <WorkspaceIdentity workspace={ws} name={name} />
                  {isActive ? (
                    <ActiveBadge label={isPrimaryOnly ? 'Connected' : 'Active'} />
                  ) : (
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={() => void handleSwitch(ws.id, name)}
                    >
                      {busyTarget === switchKey ? (
                        <LoaderCircle size={13} className="animate-spin" />
                      ) : (
                        <ArrowLeftRight size={13} />
                      )}
                      Switch
                    </Button>
                  )}
                </div>

                <SettingsRow
                  title="Cloud server"
                  description={
                    isPrimaryOnly
                      ? 'Primary workspace server.'
                      : syncTime
                        ? `Last synced ${new Date(syncTime).toLocaleTimeString()}.`
                        : 'Synced with Jaz Cloud.'
                  }
                >
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-[12px] text-ink-2 select-all">{ws.url}</span>
                    {!isPrimaryOnly && isActive ? (
                      <Button size="sm" variant="ghost" disabled={syncing || busy} onClick={() => void handleSync()}>
                        <RefreshCw size={12} className={syncing ? 'animate-spin' : ''} />
                        Sync
                      </Button>
                    ) : null}
                  </div>
                </SettingsRow>

                {rowError ? (
                  <div className="border-t border-border px-3.5 py-2">
                    <p role="alert" className="text-[12px] text-danger">
                      {rowError}
                    </p>
                  </div>
                ) : null}

                <SettingsRow
                  title="Session"
                  description={
                    isPrimaryOnly
                      ? 'Revoke this cloud session and return to the sign-in screen.'
                      : 'Refresh SSO credentials or sign out of this workspace.'
                  }
                >
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={() => void startSignIn(ws.url, ws.oidc_login_url || undefined, reauthKey)}
                    >
                      {busyTarget === reauthKey ? (
                        <LoaderCircle size={13} className="animate-spin" />
                      ) : (
                        <LogIn size={13} />
                      )}
                      Re-authenticate SSO
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => void handleSignOut(ws)}
                    >
                      {busyTarget === signoutKey ? (
                        <LoaderCircle size={13} className="animate-spin" />
                      ) : (
                        <LogOut size={13} />
                      )}
                      Sign out
                    </Button>
                  </div>
                </SettingsRow>
              </SettingsCard>
            )
          })}

          {!isPrimaryOnly && !isLocalActive ? (
            <SettingsCard className="overflow-hidden">
              <div className="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3">
                <LocalWorkspaceIdentity />
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={busy}
                  onClick={() => void handleSwitch(LOCAL_WORKSPACE_ID, 'Local')}
                >
                  {busyTarget === `switch:${LOCAL_WORKSPACE_ID}` ? (
                    <LoaderCircle size={13} className="animate-spin" />
                  ) : (
                    <ArrowLeftRight size={13} />
                  )}
                  Switch
                </Button>
              </div>
            </SettingsCard>
          ) : null}

          {!showConnectCard && formError ? (
            <p role="alert" className="text-[12px] text-danger">
              {formError}
            </p>
          ) : null}
        </div>
      ) : null}

      {cloud.active && activeCloud ? (
        <div>
          <p className="mb-2 text-[12px] font-medium text-ink-3">{workspaceDisplayName(activeCloud)}</p>
          <SettingsCard className="overflow-hidden">
            <SettingsRow
              title="Connected Machines"
              description="Pair Mac, Windows, or Linux workstations for folder staging and local execution."
            >
              <Button size="sm" variant="secondary" onClick={() => onNavigate('machines')}>
                <Laptop size={13} />
                Manage machines
              </Button>
            </SettingsRow>
            <SettingsRow
              title="Personalization"
              description="Edit your standing AGENTS instructions for this workspace."
            >
              <Button size="sm" variant="secondary" onClick={() => onNavigate('personalization')}>
                <Sparkles size={13} />
                Open personalization
              </Button>
            </SettingsRow>
            <SettingsRow
              title="Usage"
              description="View monthly organization and personal cloud spend caps."
            >
              <Button size="sm" variant="secondary" onClick={() => onNavigate('usage')}>
                <ChartNoAxesColumn size={13} />
                View usage
              </Button>
            </SettingsRow>
          </SettingsCard>
        </div>
      ) : null}
    </section>
  )
}

function ActiveBadge({ label }: { label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full bg-primary-soft px-2.5 py-0.5 text-[11px] font-medium text-primary">
      <span className="size-1.5 rounded-full bg-ok" />
      {label}
    </span>
  )
}

function LocalWorkspaceIdentity() {
  return (
    <div className="flex min-w-0 flex-1 items-center gap-2.5 px-1 py-0.5">
      <span className="grid size-7 shrink-0 place-items-center rounded-[7px] bg-surface-2 text-ink-2">
        <Laptop size={14} />
      </span>
      <div className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">Local</span>
        <p className="truncate text-[11.5px] text-ink-3">This computer</p>
      </div>
    </div>
  )
}

function WorkspaceIdentity({
  workspace,
  name,
}: {
  workspace: CloudWorkspaceSummary
  name: string
}) {
  const details = [workspace.display_name, workspace.email].filter(Boolean).join(' · ')
  return (
    <div className="flex min-w-0 flex-1 items-center gap-2.5 px-1 py-0.5">
      <span className="grid size-7 shrink-0 place-items-center rounded-[7px] bg-primary-soft text-[11px] font-semibold text-primary">
        {workspaceInitials(name)}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-[13px] font-medium text-ink">{name}</span>
          {workspace.role ? (
            <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-2">
              {workspace.role}
            </span>
          ) : null}
        </div>
        {details ? (
          <p className="truncate text-[11.5px] text-ink-3">{details}</p>
        ) : null}
      </div>
    </div>
  )
}
