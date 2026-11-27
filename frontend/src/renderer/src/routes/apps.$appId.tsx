import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  AlertTriangle,
  ArrowLeft,
  Download,
  ExternalLink,
  GitFork,
  Pin,
  Share2,
  Shield,
  Trash2,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { AppRenderer } from '@/components/apps/AppRenderer'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { DesktopAppPermissionsModal } from '@/components/apps/DesktopAppPermissionsModal'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
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

  if (detail.isPending) {
    return (
      <div className="mx-auto max-w-4xl p-6">
        <SkeletonRows count={4} />
      </div>
    )
  }

  if (detail.isError || !app) {
    return (
      <EmptyState title="App unavailable">
        <p className="text-[13px] text-ink-3">This app may have been deleted or you no longer have access.</p>
        {!isStandaloneWindow ? (
          <Button className="mt-3" variant="secondary" onClick={() => void navigate({ to: '/apps' })}>
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

  return (
    <div className="flex h-full min-h-0 flex-col bg-bg">
      <header className="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-border bg-surface px-3">
        <div className="flex min-w-0 items-center gap-2">
          {!isStandaloneWindow ? (
            <Link
              to="/apps"
              className="grid size-7 shrink-0 place-items-center rounded-control text-ink-2 hover:bg-surface-2 hover:text-ink"
              aria-label="Back to Apps"
            >
              <ArrowLeft size={15} />
            </Link>
          ) : null}
          <h1 className="truncate text-[13.5px] font-semibold text-ink">{app.title}</h1>
          <button
            type="button"
            onClick={() => (isDesktop ? setPermOpen(true) : setShareOpen(true))}
            title={
              installed?.permissionsPaused
                ? 'Permissions paused after update — click to review'
                : 'Inspect version & permissions'
            }
            className={`inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium transition-colors ${
              installed?.permissionsPaused
                ? 'border-amber-500/50 bg-amber-500/15 text-amber-700 dark:text-amber-300'
                : 'border-border bg-bg text-ink-2 hover:bg-surface-2'
            }`}
          >
            {installed?.permissionsPaused ? <AlertTriangle size={11} /> : <Shield size={11} />}
            <span>v{app.current_version}</span>
            {effectiveSha ? <span className="font-mono text-[10px] opacity-75">{effectiveSha.slice(0, 7)}</span> : null}
            {installed ? (
              <span className="text-[10.5px]">
                · {installed.permissionsPaused ? 'Paused' : `${installed.permissions.length} perms`}
              </span>
            ) : null}
          </button>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {canToggleTop ? (
            <Button
              variant={alwaysOnTop ? 'primary' : 'ghost'}
              size="sm"
              onClick={() => void toggleTop()}
              title="Toggle always on top"
            >
              <Pin size={12} />
            </Button>
          ) : null}
          {!isViewer ? (
            <Button variant="ghost" size="sm" disabled={remixMutation.isPending} onClick={() => remixMutation.mutate()}>
              <GitFork size={12} />
              Remix
            </Button>
          ) : null}
          <Button variant="ghost" size="sm" onClick={() => setShareOpen(true)}>
            <Share2 size={12} />
            Share
          </Button>
          {isDesktop && !isStandaloneWindow ? (
            installed ? (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void clientRuntime.desktopApps?.openWindow(app.id, { title: app.title })}
              >
                <ExternalLink size={12} />
                Open Window
              </Button>
            ) : (
              <Button variant="secondary" size="sm" onClick={() => setPermOpen(true)}>
                <Download size={12} />
                Install as App
              </Button>
            )
          ) : null}
          {!isStandaloneWindow && !isViewer && app.can_edit !== false ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                if (window.confirm(`Delete "${app.title}" permanently?`)) deleteMutation.mutate()
              }}
            >
              <Trash2 size={12} className="text-danger" />
            </Button>
          ) : null}
        </div>
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
