import { useState } from 'react'
import { AddCloudWorkspaceModal } from '@/components/connection/AddCloudWorkspaceModal'
import { TeamMembersCard } from '@/components/settings/TeamMembersCard'
import { WorkspaceMark } from '@/components/sidebar/WorkspaceMark'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { attachCloudSignIn, signOutCloudAccount, syncCloudAccount } from '@/lib/cloudAccount'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { activeCloudWorkspace, workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'
import { disconnectBackend } from '@/lib/connection'

function host(url: string): string {
  return URL.parse(url)?.host ?? url
}

// The workspace the user is in. Switching lives in the workspace switcher, so
// this page never lists the others.
export function CloudSettings() {
  const cloud = useCloudSession()
  const isPrimaryOnly = useIsCloudOnly()
  const toast = useToast()
  const [adding, setAdding] = useState(false)
  const [running, setRunning] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const workspace = activeCloudWorkspace(cloud)
  const error = actionError || workspace?.last_error || (workspace ? cloud.error : null)

  const run = async (task: () => Promise<string | null | void>) => {
    setRunning(true)
    setActionError(null)
    try {
      setActionError((await task()) || null)
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : 'Something went wrong')
    } finally {
      setRunning(false)
    }
  }

  const signOut = () => {
    if (!workspace) return
    if (isPrimaryOnly) {
      disconnectBackend()
      return
    }
    void run(async () => {
      await signOutCloudAccount(workspace.id)
      toast(`Signed out of ${workspaceDisplayName(workspace)}`)
    })
  }

  return (
    <section className="flex flex-col gap-7 py-4">
      <header className="flex items-center gap-3">
        <WorkspaceMark workspace={workspace} className="size-10 rounded-[11px] text-[17px]" />
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-lg font-semibold text-ink">{workspace ? workspaceDisplayName(workspace) : 'Local'}</h1>
          <p className="truncate text-[13px] text-ink-3">
            {workspace ? [workspace.email, host(workspace.url)].filter(Boolean).join(' · ') : 'Stored on this computer'}
          </p>
        </div>
        {workspace ? (
          <ActionMenu
            label="Workspace actions"
            actions={[
              ...(isPrimaryOnly ? [] : [
                { label: 'Sync now', onSelect: () => void run(syncCloudAccount) },
                { label: 'Add Cloud Workspace', onSelect: () => setAdding(true) },
              ]),
              { label: 'Sign out', onSelect: signOut },
            ]}
          />
        ) : isPrimaryOnly ? null : (
          <Button variant="primary" onClick={() => setAdding(true)}>Add Cloud Workspace</Button>
        )}
      </header>

      {workspace && error ? (
        <div className="flex items-center justify-between gap-3">
          <p role="alert" className="min-w-0 text-[13px] text-danger">{error}</p>
          {isPrimaryOnly ? null : (
            <Button size="sm" disabled={running} onClick={() => void run(() => attachCloudSignIn(workspace.url, workspace.oidc_login_url))}>
              Sign in
            </Button>
          )}
        </div>
      ) : null}

      {workspace ? <TeamMembersCard workspace={workspace} /> : null}

      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </section>
  )
}
