import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { DesktopAppDialog } from '@/components/apps/DesktopAppDialog'
import { Button } from '@/components/ui/Button'
import type { MenuAction } from '@/components/ui/ActionMenu'
import { useToast } from '@/components/ui/toast'
import { appDetailQuery, detailApp, hasBundle, type BundledApp } from '@/lib/api/apps'
import { useCloudSession } from '@/lib/cloudSession'
import { useConnection } from '@/lib/connection'
import { revealInFolderLabel } from '@/lib/deviceLabel'
import { activeDesktopAppScope, desktopApps, installDesktopApp, installFolder } from '@/lib/desktopApps'
import { keys } from '@/lib/query/keys'
import { declaredDesktopPermissions, matchesDesktopAppScope } from '@shared/desktopApps'

// Only an app with a bundle installs; a board opens in a window of its own.
type AppRef = BundledApp | string

const refId = (app: AppRef): string => (typeof app === 'string' ? app : app.id)

// Installing apps as programs of their own on this computer, opening them, and
// the actions on the installed ones. Render `dialog` where the hook is used.
export function useDesktopApps() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const connection = useConnection()
  const { active, activeWorkspaceId, url: cloudUrl, user: cloudUser } = cloud
  const scope = useMemo(
    () => activeDesktopAppScope({ active, activeWorkspaceId, url: cloudUrl, user: cloudUser }, connection.url),
    [active, activeWorkspaceId, cloudUrl, cloudUser, connection.url],
  )
  const list = useQuery({ queryKey: keys.desktopApps, queryFn: () => desktopApps!.list(), enabled: Boolean(desktopApps) })
  const records = useMemo(
    () =>
      new Map(
        (list.data ?? [])
          .filter((record) => matchesDesktopAppScope(record, scope))
          .map((record) => [record.appId, record]),
      ),
    [list.data, scope],
  )
  const [dialog, setDialog] = useState<BundledApp | null>(null)
  const [installing, setInstalling] = useState<string | null>(null)
  const refresh = () => void queryClient.invalidateQueries({ queryKey: keys.desktopApps })
  const fail = (error: unknown) => toast(error instanceof Error ? error.message : 'Something went wrong', 'danger')
  const resolve = async (app: AppRef): Promise<BundledApp> => {
    if (typeof app !== 'string') return app
    const fetched = detailApp(await queryClient.fetchQuery(appDetailQuery(app)))
    if (!hasBundle(fetched)) throw new Error('A board cannot be installed as an app')
    return fetched
  }

  const install = async (app: AppRef) => {
    setInstalling(refId(app))
    try {
      const target = await resolve(app)
      if (declaredDesktopPermissions(target.capabilities).length) {
        setDialog(target)
      } else {
        await installDesktopApp(target, [], [], false, scope)
        refresh()
      }
    } catch (error) {
      fail(error)
    } finally {
      setInstalling(null)
    }
  }

  // Open App once installed, Install App until then.
  const primary = (app: AppRef): MenuAction | null => {
    if (!desktopApps) return null
    const id = refId(app)
    return records.has(id)
      ? { label: 'Open App', onSelect: () => void desktopApps!.open(id).catch(fail) }
      : { label: installing === id ? 'Installing…' : 'Install App', onSelect: () => void install(app) }
  }

  const actions = (app: AppRef): MenuAction[] => {
    const id = refId(app)
    const record = records.get(id)
    if (!record) return []
    return [
      ...(declaredDesktopPermissions(record.declaredCapabilities).length
        ? [{ label: 'Permissions…', onSelect: () => void resolve(app).then(setDialog, fail) }]
        : []),
      {
        label: revealInFolderLabel(),
        hint: record.launcherPath && installFolder(record.launcherPath),
        onSelect: () => void desktopApps!.reveal(id).catch(fail),
      },
      { label: 'Uninstall', onSelect: () => void desktopApps!.uninstall(id).then(refresh, fail) },
    ]
  }

  return {
    available: Boolean(desktopApps),
    records,
    installing,
    primary,
    review: (app: AppRef) => void resolve(app).then(setDialog, fail),
    actions,
    dialog: dialog ? (
      <DesktopAppDialog app={dialog} installed={records.get(dialog.id) ?? null} scope={scope} onClose={() => setDialog(null)} onSaved={refresh} />
    ) : null,
  }
}

export type DesktopApps = ReturnType<typeof useDesktopApps>

export function DesktopAppButton({ desktop, app }: { desktop: DesktopApps; app: AppRef }) {
  const action = desktop.primary(app)
  if (!action) return null
  return <Button size="sm" disabled={desktop.installing === refId(app)} onClick={action.onSelect}>{action.label}</Button>
}
