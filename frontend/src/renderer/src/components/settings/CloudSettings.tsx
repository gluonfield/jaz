import { LoaderCircle, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { AddCloudWorkspaceModal } from '@/components/connection/AddCloudWorkspaceModal'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { attachCloudSignIn, signOutCloudAccount, syncCloudAccount } from '@/lib/cloudAccount'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { activeCloudWorkspace, workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'
import { disconnectBackend } from '@/lib/connection'

type Action = 'sync' | 'sign-in' | 'sign-out'

// The workspace the user is in. Switching lives in the sidebar's workspace
// switcher, so this page never lists the others.
export function CloudSettings() {
  const cloud = useCloudSession()
  const isPrimaryOnly = useIsCloudOnly()
  const toast = useToast()
  const [adding, setAdding] = useState(false)
  const [running, setRunning] = useState<Action | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const workspace = activeCloudWorkspace(cloud)
  const syncedAt = workspace?.last_sync_at || cloud.lastSyncAt
  const error = actionError || workspace?.last_error || (workspace ? cloud.error : null)

  const run = async (action: Action, task: () => Promise<string | null | void>) => {
    setRunning(action)
    setActionError(null)
    try {
      setActionError((await task()) || null)
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : 'Something went wrong')
    } finally {
      setRunning(null)
    }
  }

  const signOut = () => {
    if (!workspace) return
    if (isPrimaryOnly) {
      disconnectBackend()
      return
    }
    void run('sign-out', async () => {
      await signOutCloudAccount(workspace.id)
      toast(`Signed out of ${workspaceDisplayName(workspace)}`)
    })
  }

  return (
    <section className="flex flex-col gap-5 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-ink">Workspace</h1>
          {isPrimaryOnly ? null : (
            <p className="mt-0.5 text-[13px] text-ink-2">
              {cloud.workspaces.length > 0
                ? 'Switch workspaces from the top of the sidebar.'
                : 'Add a Jaz Cloud workspace to run threads in the cloud.'}
            </p>
          )}
        </div>
        {isPrimaryOnly ? null : (
          <Button size="sm" onClick={() => setAdding(true)}>
            <Plus size={13} />
            Add Cloud Workspace
          </Button>
        )}
      </div>

      {workspace ? (
        <SettingsCard className="overflow-hidden">
          <SettingsRow
            title={workspaceDisplayName(workspace)}
            description={[...new Set([workspace.display_name, workspace.email].filter(Boolean))].join(' · ') || undefined}
          >
            <Button size="sm" disabled={running !== null} onClick={signOut}>
              {running === 'sign-out' ? <LoaderCircle size={13} className="animate-spin" /> : null}
              Sign out
            </Button>
          </SettingsRow>
          <SettingsRow
            title="Server"
            description={
              syncedAt
                ? `Synced ${new Date(syncedAt).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })}`
                : undefined
            }
          >
            <div className="flex min-w-0 items-center gap-1">
              <span className="min-w-0 truncate font-mono text-[12px] text-ink-2 select-all">{workspace.url}</span>
              {isPrimaryOnly ? null : (
                <IconButton
                  variant="ghost"
                  size="sm"
                  aria-label="Sync now"
                  title="Sync now"
                  disabled={running !== null}
                  onClick={() => void run('sync', syncCloudAccount)}
                >
                  <RefreshCw size={13} className={running === 'sync' ? 'animate-spin' : ''} />
                </IconButton>
              )}
            </div>
          </SettingsRow>
          {error ? (
            <div className="flex items-center justify-between gap-3 border-t border-border px-3 py-2.5">
              <p role="alert" className="min-w-0 text-[12px] text-danger">
                {error}
              </p>
              {isPrimaryOnly ? null : (
                <Button
                  size="sm"
                  disabled={running !== null}
                  onClick={() =>
                    void run('sign-in', () => attachCloudSignIn(workspace.url, workspace.oidc_login_url))
                  }
                >
                  {running === 'sign-in' ? <LoaderCircle size={13} className="animate-spin" /> : null}
                  Sign in
                </Button>
              )}
            </div>
          ) : null}
        </SettingsCard>
      ) : isPrimaryOnly ? null : (
        <SettingsCard>
          <SettingsRow title="Local" description="Threads and settings are stored on this computer.">
            {null}
          </SettingsRow>
        </SettingsCard>
      )}

      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </section>
  )
}
