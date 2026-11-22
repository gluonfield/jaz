import { useState } from 'react'
import { useToast } from '@/components/ui/toast'
import { useKnownBackends } from '@/lib/backends'
import { signOutCloudAccount } from '@/lib/cloudAccount'
import { useCloudSession } from '@/lib/cloudSession'
import { connectRemote, rememberConnection, startCloudSignIn, startLocal, useConnection } from '@/lib/connection'

// State and orchestration shared by the two "Run jaz on" switchers — the sidebar
// footer and the settings panel. Owns the open popover, the current backend, cloud
// user identity, saved remotes, and switching between them (toasting on failure).
export function useBackendSwitcher() {
  const { status, url, mode, auth, oidcLoginUrl, user } = useConnection()
  const cloud = useCloudSession()
  const remotes = useKnownBackends()
  const toast = useToast()
  const [open, setOpen] = useState(false)

  const switchTo = async (action: () => Promise<string | null>) => {
    setOpen(false)
    const err = await action()
    if (err) toast(err, 'danger')
  }

  return {
    open,
    setOpen,
    status,
    url,
    mode,
    auth,
    oidcLoginUrl,
    user,
    cloud,
    remotes,
    switchLocal: () => switchTo(startLocal),
    switchRemote: (remoteUrl: string) => switchTo(() => connectRemote(remoteUrl)),
    signInWithSSO: (targetUrl: string, loginUrl?: string) => switchTo(() => startCloudSignIn(targetUrl, loginUrl)),
    signOutCloud: () => {
      setOpen(false)
      signOutCloudAccount()
      toast('Signed out of Jaz Cloud')
    },
    remember: () => {
      const error = rememberConnection()
      toast(error || 'Connection remembered', error ? 'danger' : 'ok')
    },
  }
}
