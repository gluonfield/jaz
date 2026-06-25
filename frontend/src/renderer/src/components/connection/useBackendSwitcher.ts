import { useState } from 'react'
import { useToast } from '@/components/ui/toast'
import { useKnownBackends } from '@/lib/backends'
import { connectRemote, startLocal, useConnection } from '@/lib/connection'

// State and orchestration shared by the two "Run jaz on" switchers — the sidebar
// footer and the settings panel. Owns the open popover, the current backend and
// saved remotes, and switching between them (toasting on failure). Each switcher
// renders its own chrome and per-row actions on top of this.
export function useBackendSwitcher() {
  const { status, url } = useConnection()
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
    remotes,
    switchLocal: () => switchTo(startLocal),
    switchRemote: (remoteUrl: string) => switchTo(() => connectRemote(remoteUrl)),
  }
}
