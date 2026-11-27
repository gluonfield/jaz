import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { DesktopAppPermissionsModal } from '@/components/apps/DesktopAppPermissionsModal'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { appDetailQuery, detailApp, orgAppPolicyQuery } from '@/lib/api/apps'
import { clientRuntime } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'

// Installs a saved app as a desktop app, behind the install approval. Once
// installed it opens the app's window. Renders nothing outside the desktop app.
export function InstallAppButton({ appId }: { appId: string }) {
  const desktopApps = clientRuntime.kind === 'electron' ? clientRuntime.desktopApps : undefined
  const queryClient = useQueryClient()
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const installed = useQuery({
    queryKey: keys.desktopApps,
    queryFn: () => desktopApps!.list(),
    enabled: Boolean(desktopApps),
  })
  const detail = useQuery({ ...appDetailQuery(appId), enabled: open })
  const orgPolicy = useQuery({ ...orgAppPolicyQuery, enabled: open })

  if (!desktopApps) return null
  const current = installed.data?.find((app) => app.appId === appId)
  if (current) {
    return (
      <Button size="sm" onClick={() => void desktopApps.openWindow(appId, { title: current.title })}>
        Open Window
      </Button>
    )
  }
  return (
    <>
      <Button size="sm" disabled={open && detail.isPending} onClick={() => setOpen(true)}>
        Install App
      </Button>
      {open && detail.data ? (
        <DesktopAppPermissionsModal
          open
          onClose={() => setOpen(false)}
          app={detailApp(detail.data)}
          installed={null}
          orgPolicy={orgPolicy.data}
          onSaved={() => {
            void queryClient.invalidateQueries({ queryKey: keys.desktopApps })
            toast('Installed Desktop App')
          }}
        />
      ) : null}
    </>
  )
}
