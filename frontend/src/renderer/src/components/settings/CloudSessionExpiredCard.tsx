import { LoaderCircle, LogIn } from 'lucide-react'
import { useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { attachCloudSignIn, type CloudWorkspaceSummary } from '@/lib/cloudAccount'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'
import { startCloudSignIn } from '@/lib/connection'

export function CloudSessionExpiredCard({
  workspace,
  description,
  onRetry,
}: {
  workspace: CloudWorkspaceSummary
  description: string
  onRetry: () => Promise<unknown>
}) {
  const cloud = useCloudSession()
  const isCloudOnly = useIsCloudOnly()
  const toast = useToast()
  const [signingIn, setSigningIn] = useState(false)

  const signIn = async () => {
    if (signingIn) return
    setSigningIn(true)
    try {
      const loginUrl = workspace.oidc_login_url || cloud.oidcLoginUrl || undefined
      const err =
        cloud.source === 'primary' || isCloudOnly
          ? await startCloudSignIn(workspace.url, loginUrl)
          : await attachCloudSignIn(workspace.url, loginUrl)
      if (err) {
        toast(err, 'danger')
      }
    } catch (caught) {
      toast(caught instanceof Error ? caught.message : 'Could not start Jaz Cloud sign-in', 'danger')
    } finally {
      setSigningIn(false)
    }
  }

  return (
    <SettingsCard className="p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h3 className="text-[14px] font-semibold text-ink">Sign in to Jaz Cloud</h3>
          <p className="mt-0.5 text-[12.5px] text-ink-3">
            Your session for {workspaceDisplayName(workspace)} has expired. {description}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" disabled={signingIn} onClick={() => void onRetry()}>
            Retry
          </Button>
          <Button variant="primary" size="sm" disabled={signingIn} onClick={() => void signIn()}>
            {signingIn ? <LoaderCircle size={13} className="animate-spin" /> : <LogIn size={13} />}
            Sign in
          </Button>
        </div>
      </div>
    </SettingsCard>
  )
}
