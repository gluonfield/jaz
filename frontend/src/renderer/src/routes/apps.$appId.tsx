import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Pin } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { AppPageHeader } from '@/components/apps/AppPageHeader'
import { AppRenderer } from '@/components/apps/AppRenderer'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { useDeleteApp } from '@/components/apps/useDeleteApp'
import { DesktopAppButton, useDesktopApps } from '@/components/apps/useDesktopApps'
import { BoardPage } from '@/components/boards/BoardPage'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { ActionMenu, type MenuAction } from '@/components/ui/ActionMenu'
import { useToast } from '@/components/ui/toast'
import { appDetailQuery, detailApp, hasBundle, remixApp, type BundledApp } from '@/lib/api/apps'
import { clientRuntime } from '@/lib/clientRuntime'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { desktopApps } from '@/lib/desktopApps'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/apps/$appId')({
  // ?add=1 opens a new board's tile picker on arrival.
  validateSearch: (search): { add?: boolean } => (search.add ? { add: true } : {}),
  component: AppDetailPage,
})

function AppDetailPage() {
  const { appId } = Route.useParams()
  const navigate = useNavigate()
  const detail = useQuery(appDetailQuery(appId))
  const app = useMemo(() => detail.data && detailApp(detail.data), [detail.data])

  if (detail.isPending) return null

  if (!app) {
    return (
      <EmptyState title="App unavailable">
        <p>It may have been deleted, or you no longer have access.</p>
        {clientRuntime.windowKind === 'main' ? (
          <Button className="mt-4" variant="primary" onClick={() => void navigate({ to: '/apps' })}>
            Back to Apps
          </Button>
        ) : null}
      </EmptyState>
    )
  }

  return hasBundle(app) ? <PublishedAppPage app={app} /> : <BoardPage board={app} />
}

function PublishedAppPage({ app }: { app: BundledApp }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const isStandaloneWindow = clientRuntime.windowKind === 'app'
  const desktop = useDesktopApps()

  const [alwaysOnTop, setAlwaysOnTop] = useState(false)
  const [shareOpen, setShareOpen] = useState(false)
  const installed = desktop.records.get(app.id)

  // An installed app's permissions pause when the app changed since they were allowed.
  useEffect(() => {
    if (!desktopApps || !installed) return
    void desktopApps
      .syncVersion(app.id, app.current_version, app.bundle_sha256 ?? '', app.capabilities ?? [])
      .then((synced) => synced && queryClient.invalidateQueries({ queryKey: keys.desktopApps }))
      .catch(() => {})
  }, [app, installed, queryClient])

  useEffect(() => {
    if (isStandaloneWindow) document.title = app.title
  }, [isStandaloneWindow, app.title])

  const remixMutation = useMutation({
    mutationFn: () => remixApp(app.id),
    onSuccess: (res) => {
      toast('Remixed into a new thread')
      if (res.thread?.id) {
        if (isStandaloneWindow) {
          clientRuntime.openInMain?.(`/sessions/${res.thread.id}`)
        } else {
          void navigate({ to: '/sessions/$sessionId', params: { sessionId: res.thread.id } })
        }
      }
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not remix app', 'danger'),
  })

  const deleteAction = useDeleteApp(() => void navigate({ to: '/apps' }))

  const canToggleTop = isStandaloneWindow && installed && !installed.permissionsPaused && installed.permissions.includes('always_on_top')

  const toggleTop = async () => {
    const next = !alwaysOnTop
    const ok = await clientRuntime.desktopApps?.setAlwaysOnTop(app.id, next)
    if (ok) setAlwaysOnTop(next)
  }

  const paused = installed?.permissionsPaused ?? false
  const actions: MenuAction[] = [
    ...(isViewer ? [] : [{ label: 'Remix in a new chat', onSelect: () => remixMutation.mutate() }]),
    ...desktop.actions(app),
    ...(isStandaloneWindow ? [] : deleteAction(app)),
  ]

  return (
    <div className="flex h-full min-h-0 flex-col bg-bg">
      <AppPageHeader title={<h1 className="truncate">{app.title}</h1>}>
        {paused ? (
          <Button size="sm" onClick={() => desktop.review(app)}>
            <span className="size-1.5 rounded-full bg-running" aria-hidden />
            Review permissions
          </Button>
        ) : null}
        {canToggleTop ? (
          <IconButton size="sm" aria-label="Keep on top" title="Keep on top" aria-pressed={alwaysOnTop} onClick={() => void toggleTop()} className={alwaysOnTop ? 'bg-ink/15 text-ink' : ''}>
            <Pin size={14} />
          </IconButton>
        ) : null}
        {isStandaloneWindow ? null : <DesktopAppButton desktop={desktop} app={app} />}
        <Button size="sm" onClick={() => setShareOpen(true)}>{cloud.active ? 'Share' : 'Manage'}</Button>
        {actions.length ? <ActionMenu label="More actions" actions={actions} /> : null}
      </AppPageHeader>

      <div className="flex min-h-0 flex-1 flex-col">
        <AppRenderer
          app={app}
          desktopPermissions={installed?.permissions}
          desktopPermissionsPaused={installed?.permissionsPaused ?? false}
        />
      </div>

      {shareOpen ? <AppShareModal open onClose={() => setShareOpen(false)} app={app} /> : null}
      {desktop.dialog}
    </div>
  )
}
