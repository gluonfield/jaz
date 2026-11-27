import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ArrowLeft, MoreHorizontal, Pin } from 'lucide-react'
import { useEffect, useState } from 'react'
import { AppRenderer } from '@/components/apps/AppRenderer'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { DesktopAppPermissionsModal } from '@/components/apps/DesktopAppPermissionsModal'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import {
  appDetailQuery,
  deleteApp,
  orgAppPolicyQuery,
  remixApp,
} from '@/lib/api/apps'
import { clientRuntime } from '@/lib/clientRuntime'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'
import type { InstalledDesktopApp } from '@shared/desktopApps'

export const Route = createFileRoute('/apps/$appId')({
  component: AppDetailPage,
})

function AppDetailPage() {
  const { appId } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isViewer = useIsCloudViewer()
  const isStandaloneWindow = clientRuntime.windowKind === 'app'
  const isDesktop = clientRuntime.kind === 'electron' && Boolean(clientRuntime.desktopApps)

  const detail = useQuery(appDetailQuery(appId))
  const orgPolicy = useQuery({ ...orgAppPolicyQuery, enabled: isDesktop })
  const [installed, setInstalled] = useState<InstalledDesktopApp | null>(null)
  const [alwaysOnTop, setAlwaysOnTop] = useState(false)
  const [shareOpen, setShareOpen] = useState(false)
  const [permOpen, setPermOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)

  const app = detail.data?.app

  // Automatic version check on open: sync latest version & bundle_sha256 so any
  // update automatically pauses version-locked Tier 2/3 Desktop permissions.
  useEffect(() => {
    if (!isDesktop || !app) return
    let active = true
    const sha = app.bundle_sha256 ?? detail.data?.current_version?.bundle_sha256 ?? ''
    void clientRuntime.desktopApps!
      .syncVersion(app.id, app.current_version, sha, app.capabilities ?? [])
      .then((synced) => {
        if (!active) return
        setInstalled(synced)
        if (synced?.permissionsPaused) {
          toast('App updated — elevated permissions paused until re-approved', 'danger')
        }
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [isDesktop, app?.id, app?.current_version, app?.bundle_sha256, detail.data?.current_version?.bundle_sha256])

  useEffect(() => {
    if (isStandaloneWindow && app?.title) {
      document.title = app.title
    }
  }, [isStandaloneWindow, app?.title])

  const remixMutation = useMutation({
    mutationFn: () => remixApp(appId),
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

  const deleteMutation = useMutation({
    mutationFn: () => deleteApp(appId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast('Deleted app')
      void navigate({ to: '/apps' })
    },
  })

  if (detail.isPending) return null

  if (detail.isError || !app) {
    return (
      <EmptyState title="App unavailable">
        <p>It may have been deleted, or you no longer have access.</p>
        {!isStandaloneWindow ? (
          <Button className="mt-4" variant="primary" onClick={() => void navigate({ to: '/apps' })}>
            Back to Apps
          </Button>
        ) : null}
      </EmptyState>
    )
  }

  const effectiveSha = app.bundle_sha256 ?? detail.data?.current_version?.bundle_sha256 ?? ''
  const appWithSha = effectiveSha ? { ...app, bundle_sha256: effectiveSha } : app
  const canToggleTop =
    isStandaloneWindow &&
    Boolean(installed) &&
    !installed!.permissionsPaused &&
    installed!.permissions.includes('always_on_top')

  const toggleTop = async () => {
    const next = !alwaysOnTop
    const ok = await clientRuntime.desktopApps?.setAlwaysOnTop(app.id, next)
    if (ok) setAlwaysOnTop(next)
  }

  const paused = installed?.permissionsPaused ?? false
  const menuRow = (label: string, action: () => void) => (
    <MenuRow key={label} role="menuitem" onClick={() => {
      setMenuOpen(false)
      action()
    }}>
      {label}
    </MenuRow>
  )

  return (
    <div className="flex h-full min-h-0 flex-col bg-bg">
      <header className="flex h-11 shrink-0 items-center gap-1 border-b border-border/70 px-2">
        {!isStandaloneWindow ? (
          <IconButton size="sm" aria-label="Back to Apps" title="Back to Apps" onClick={() => void navigate({ to: '/apps' })}>
            <ArrowLeft size={15} />
          </IconButton>
        ) : null}
        <h1 className="min-w-0 flex-1 truncate px-1.5 text-[13.5px] font-medium text-ink">{app.title}</h1>
        {paused ? (
          <Button size="sm" onClick={() => setPermOpen(true)}>
            <span className="size-1.5 rounded-full bg-running" aria-hidden />
            Review permissions
          </Button>
        ) : null}
        {canToggleTop ? (
          <IconButton size="sm" aria-label="Keep on top" title="Keep on top" aria-pressed={alwaysOnTop} onClick={() => void toggleTop()} className={alwaysOnTop ? 'bg-ink/15 text-ink' : ''}>
            <Pin size={14} />
          </IconButton>
        ) : null}
        <Button size="sm" onClick={() => setShareOpen(true)}>Share</Button>
        <Popover open={menuOpen} onClose={() => setMenuOpen(false)} placement="below" align="end" trigger={
          <IconButton size="sm" aria-label="More actions" aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => setMenuOpen((open) => !open)}>
            <MoreHorizontal size={15} />
          </IconButton>
        }>
          <div role="menu" aria-label="More actions">
            {!isViewer ? menuRow('Remix in a new chat', () => remixMutation.mutate()) : null}
            {isDesktop && !isStandaloneWindow
              ? installed
                ? menuRow('Open in window', () => void clientRuntime.desktopApps?.openWindow(app.id, { title: app.title }))
                : menuRow('Install as desktop app', () => setPermOpen(true))
              : null}
            {isDesktop && installed ? menuRow('Permissions', () => setPermOpen(true)) : null}
            {!isStandaloneWindow && !isViewer && app.can_edit !== false ? menuRow('Delete', () => {
              if (window.confirm(`Delete "${app.title}" permanently?`)) deleteMutation.mutate()
            }) : null}
          </div>
        </Popover>
      </header>

      <div className="flex min-h-0 flex-1 flex-col">
        <AppRenderer
          app={appWithSha}
          fillHeight
          desktopPermissions={installed?.permissions}
          desktopPermissionsPaused={installed?.permissionsPaused ?? false}
        />
      </div>

      {shareOpen ? <AppShareModal open onClose={() => setShareOpen(false)} app={appWithSha} /> : null}
      {permOpen && isDesktop ? (
        <DesktopAppPermissionsModal
          open
          onClose={() => setPermOpen(false)}
          app={appWithSha}
          installed={installed}
          orgPolicy={orgPolicy.data}
          onSaved={(next) => {
            setInstalled(next)
            void queryClient.invalidateQueries({ queryKey: keys.desktopApps })
            toast('Updated Desktop App permissions')
          }}
        />
      ) : null}
    </div>
  )
}
