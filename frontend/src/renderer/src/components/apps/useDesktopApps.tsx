import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { DesktopAppDialog } from '@/components/apps/DesktopAppDialog'
import { Button } from '@/components/ui/Button'
import type { MenuAction } from '@/components/ui/ActionMenu'
import { useToast } from '@/components/ui/toast'
import { appDetailQuery, detailApp, type JazApp } from '@/lib/api/apps'
import { revealInFolderLabel } from '@/lib/deviceLabel'
import { desktopApps, installDesktopApp, installFolder } from '@/lib/desktopApps'
import { keys } from '@/lib/query/keys'
import { declaredDesktopPermissions } from '@shared/desktopApps'

type AppRef = JazApp | string

const refId = (app: AppRef): string => (typeof app === 'string' ? app : app.id)

// Installing apps as programs of their own on this computer, opening them, and
// the actions on the installed ones. Render `dialog` where the hook is used.
export function useDesktopApps() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const list = useQuery({ queryKey: keys.desktopApps, queryFn: () => desktopApps!.list(), enabled: Boolean(desktopApps) })
  const records = useMemo(() => new Map((list.data ?? []).map((record) => [record.appId, record])), [list.data])
  const [dialog, setDialog] = useState<JazApp | null>(null)
  const [installing, setInstalling] = useState<string | null>(null)
  const refresh = () => void queryClient.invalidateQueries({ queryKey: keys.desktopApps })
  const fail = (error: unknown) => toast(error instanceof Error ? error.message : 'Something went wrong', 'danger')
  const resolve = async (app: AppRef): Promise<JazApp> =>
    typeof app === 'string' ? detailApp(await queryClient.fetchQuery(appDetailQuery(app))) : app

  const install = async (app: AppRef) => {
    setInstalling(refId(app))
    try {
      const target = await resolve(app)
      if (declaredDesktopPermissions(target.capabilities).length) {
        setDialog(target)
      } else {
        await installDesktopApp(target)
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
      <DesktopAppDialog app={dialog} installed={records.get(dialog.id) ?? null} onClose={() => setDialog(null)} onSaved={refresh} />
    ) : null,
  }
}

export type DesktopApps = ReturnType<typeof useDesktopApps>

export function DesktopAppButton({ desktop, app }: { desktop: DesktopApps; app: AppRef }) {
  const action = desktop.primary(app)
  if (!action) return null
  return <Button size="sm" disabled={desktop.installing === refId(app)} onClick={action.onSelect}>{action.label}</Button>
}
