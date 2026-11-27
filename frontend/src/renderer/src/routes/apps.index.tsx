import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  AlertTriangle,
  AppWindow,
  Download,
  ExternalLink,
  FileText,
  Folder,
  GitFork,
  Lock,
  Plug,
  Share2,
  Shield,
  Sparkles,
  Trash2,
  UserCheck,
  Users,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { AppIcon } from '@/components/apps/AppIcon'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { DesktopAppPermissionsModal } from '@/components/apps/DesktopAppPermissionsModal'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Input } from '@/components/ui/Input'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  appsQuery,
  deleteApp,
  orgAppPolicyQuery,
  remixApp,
  type AppKind,
  type JazApp,
} from '@/lib/api/apps'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import { clientRuntime } from '@/lib/clientRuntime'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'
import type { InstalledDesktopApp } from '@shared/desktopApps'

export const Route = createFileRoute('/apps/')({
  component: AppsLibraryPage,
})

type AppsTab = 'mine' | 'team' | 'desktop' | 'connected'

function visibilityBadge(app: JazApp) {
  switch (app.visibility) {
    case 'team':
      return { label: 'Team', Icon: Users }
    case 'space':
      return { label: 'Space', Icon: Folder }
    case 'selected':
      return { label: 'Selected', Icon: UserCheck }
    default:
      return { label: 'Private', Icon: Lock }
  }
}

function AppsLibraryPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const isDesktop = clientRuntime.kind === 'electron' && Boolean(clientRuntime.desktopApps)

  const [tab, setTab] = useState<AppsTab>('mine')
  const [search, setSearch] = useState('')
  const [kindFilter, setKindFilter] = useState<'all' | AppKind>('all')
  const [shareApp, setShareApp] = useState<JazApp | null>(null)
  const [permTarget, setPermTarget] = useState<{ app: JazApp; installed: InstalledDesktopApp | null } | null>(null)

  const allApps = useQuery(appsQuery({ scope: 'all' }))
  const mcpApps = useQuery(mcpEntrypointsQuery)
  const orgPolicy = useQuery({ ...orgAppPolicyQuery, enabled: isDesktop })
  const desktopInstalled = useQuery({
    queryKey: keys.desktopApps,
    queryFn: () => clientRuntime.desktopApps!.list(),
    enabled: isDesktop,
  })

  const installedById = useMemo(() => {
    const map = new Map<string, InstalledDesktopApp>()
    for (const item of desktopInstalled.data ?? []) {
      map.set(item.appId, item)
    }
    return map
  }, [desktopInstalled.data])

  const myApps = useMemo(
    () => (allApps.data ?? []).filter((a) => a.is_owner !== false),
    [allApps.data],
  )
  const teamApps = useMemo(
    () => (allApps.data ?? []).filter((a) => a.visibility !== 'private'),
    [allApps.data],
  )
  const connectedEntrypoints = useMemo(
    () => (mcpApps.data ?? []).filter((e) => e.type === 'global'),
    [mcpApps.data],
  )

  const filterList = (list: JazApp[]) => {
    const needle = search.trim().toLowerCase()
    return list.filter((a) => {
      if (kindFilter !== 'all' && a.kind !== kindFilter) return false
      if (!needle) return true
      return (
        a.title.toLowerCase().includes(needle) ||
        (a.description ?? '').toLowerCase().includes(needle)
      )
    })
  }

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteApp(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast('Deleted app')
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not delete app', 'danger'),
  })

  const remixMutation = useMutation({
    mutationFn: (id: string) => remixApp(id),
    onSuccess: (res) => {
      toast('Remixed into a new thread')
      if (res.thread?.id) {
        void navigate({ to: '/sessions/$sessionId', params: { sessionId: res.thread.id } })
      }
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not remix app', 'danger'),
  })

  const uninstallDesktopMutation = useMutation({
    mutationFn: (appId: string) => clientRuntime.desktopApps!.uninstall(appId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.desktopApps })
      toast('Uninstalled Desktop App')
    },
  })

  const tabs: { id: AppsTab; label: string; count: number; visible: boolean }[] = [
    { id: 'mine', label: 'My Apps', count: myApps.length, visible: true },
    { id: 'team', label: 'Team & Spaces', count: teamApps.length, visible: cloud.active },
    { id: 'desktop', label: 'Installed Desktop Apps', count: (desktopInstalled.data ?? []).length, visible: isDesktop },
    { id: 'connected', label: 'Connected Apps', count: connectedEntrypoints.length, visible: true },
  ]

  const renderAppCard = (app: JazApp) => {
    const badge = visibilityBadge(app)
    const BadgeIcon = badge.Icon
    const installed = installedById.get(app.id) ?? null
    return (
      <div
        key={app.id}
        className="flex flex-col justify-between gap-3 rounded-card border border-border bg-surface p-4 transition-colors hover:border-border/90"
      >
        <div className="space-y-1.5">
          <div className="flex items-start justify-between gap-2">
            <Link
              to="/apps/$appId"
              params={{ appId: app.id }}
              className="flex min-w-0 items-center gap-2 hover:text-primary"
            >
              <span className="grid size-8 shrink-0 place-items-center rounded-control bg-surface-2 text-[15px] text-primary">
                {app.icon?.kind === 'emoji' && app.icon.value ? (
                  app.icon.value
                ) : app.kind === 'document' ? (
                  <FileText size={15} />
                ) : (
                  <Sparkles size={15} />
                )}
              </span>
              <div className="min-w-0">
                <h3 className="truncate text-[14px] font-semibold text-ink">{app.title}</h3>
                <div className="flex items-center gap-1.5 text-[11px] text-ink-3">
                  <span className="inline-flex items-center gap-1">
                    <BadgeIcon size={11} />
                    {badge.label}
                  </span>
                  <span>·</span>
                  <span>v{app.current_version}</span>
                  <span>·</span>
                  <span className="capitalize">{app.kind}</span>
                </div>
              </div>
            </Link>
            {installed?.permissionsPaused ? (
              <span
                title="Permissions paused after version update"
                className="inline-flex shrink-0 items-center gap-1 rounded-full bg-amber-500/15 px-2 py-0.5 text-[10.5px] font-medium text-amber-600 dark:text-amber-400"
              >
                <AlertTriangle size={11} />
                Update
              </span>
            ) : null}
          </div>
          {app.description ? (
            <p className="line-clamp-2 text-[12.5px] text-ink-2">{app.description}</p>
          ) : null}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-1.5 border-t border-border/60 pt-2.5">
          <div className="flex items-center gap-1">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void navigate({ to: '/apps/$appId', params: { appId: app.id } })}
            >
              Open
            </Button>
            {isDesktop ? (
              installed ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void clientRuntime.desktopApps?.openWindow(app.id, { title: app.title })}
                >
                  <ExternalLink size={12} />
                  Launch
                </Button>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setPermTarget({ app, installed: null })}
                >
                  <Download size={12} />
                  Install
                </Button>
              )
            ) : null}
          </div>

          <div className="flex items-center gap-1">
            {!isViewer ? (
              <Button
                variant="ghost"
                size="sm"
                title="Remix into a new chat"
                disabled={remixMutation.isPending}
                onClick={() => remixMutation.mutate(app.id)}
              >
                <GitFork size={12} />
              </Button>
            ) : null}
            <Button
              variant="ghost"
              size="sm"
              title="Share & manage"
              onClick={() => setShareApp(app)}
            >
              <Share2 size={12} />
            </Button>
            {!isViewer && app.can_edit !== false ? (
              <Button
                variant="ghost"
                size="sm"
                title="Delete app"
                onClick={() => {
                  if (window.confirm(`Delete "${app.title}" and all its stored data?`)) {
                    deleteMutation.mutate(app.id)
                  }
                }}
              >
                <Trash2 size={12} className="text-danger" />
              </Button>
            ) : null}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-6 py-8">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <span className="grid size-10 place-items-center rounded-card bg-surface text-primary ring-1 ring-border/70">
            <AppWindow size={20} />
          </span>
          <div>
            <h1 className="text-[20px] font-semibold tracking-tight text-ink">Apps</h1>
            <p className="text-[12.5px] text-ink-3">
              Shareable artifacts, documents, installed Desktop Apps, and connected MCP Apps.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search apps…"
            className="w-52"
          />
          {tab === 'mine' || tab === 'team' ? (
            <select
              value={kindFilter}
              onChange={(e) => setKindFilter(e.target.value as 'all' | AppKind)}
              aria-label="Filter by kind"
              className="rounded-control border border-border bg-bg px-2.5 py-1.5 text-[12.5px] text-ink"
            >
              <option value="all">All kinds</option>
              <option value="artifact">Interactive</option>
              <option value="document">Documents</option>
            </select>
          ) : null}
        </div>
      </header>

      <div role="tablist" aria-label="App categories" className="flex items-center gap-1 border-b border-border/70">
        {tabs
          .filter((t) => t.visible)
          .map((t) => {
            const active = tab === t.id
            return (
              <button
                key={t.id}
                type="button"
                role="tab"
                aria-selected={active}
                onClick={() => setTab(t.id)}
                className={`-mb-px flex cursor-pointer items-center gap-1.5 border-b-2 px-3 py-2 text-[13px] font-medium transition-colors ${
                  active ? 'border-primary text-ink' : 'border-transparent text-ink-3 hover:text-ink'
                }`}
              >
                <span>{t.label}</span>
                <span className="rounded-full bg-surface-2 px-1.5 py-0.5 text-[11px] text-ink-3 tabular-nums">
                  {t.count}
                </span>
              </button>
            )
          })}
      </div>

      {tab === 'mine' || tab === 'team' ? (
        allApps.isPending ? (
          <SkeletonRows count={4} />
        ) : (
          (() => {
            const list = filterList(tab === 'mine' ? myApps : teamApps)
            if (list.length === 0) {
              return (
                <EmptyState
                  title={tab === 'mine' ? 'No published apps yet' : 'No team or Space apps yet'}
                >
                  <p className="text-[13px] text-ink-3">
                    Publish any inline artifact or Markdown document from a chat thread to see it here.
                  </p>
                </EmptyState>
              )
            }
            return <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{list.map(renderAppCard)}</div>
          })()
        )
      ) : tab === 'desktop' ? (
        (desktopInstalled.data ?? []).length === 0 ? (
          <EmptyState title="No Desktop Apps installed">
            <p className="text-[13px] text-ink-3">
              Click Install on any Jaz App to create a standalone window and OS launcher shim.
            </p>
          </EmptyState>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {(desktopInstalled.data ?? []).map((inst) => {
              const live = (allApps.data ?? []).find((a) => a.id === inst.appId)
              const appModel: JazApp = live ?? {
                id: inst.appId,
                owner_user_id: 'local',
                title: inst.title,
                description: inst.description,
                kind: inst.kind,
                visibility: 'private',
                current_version: inst.installedVersion,
                status: 'active',
                bundle_sha256: inst.latestBundleSha256,
                capabilities: inst.declaredCapabilities,
                created_at: inst.installedAt,
                updated_at: inst.updatedAt,
              }
              return (
                <div
                  key={inst.appId}
                  className="flex flex-col justify-between gap-3 rounded-card border border-border bg-surface p-4"
                >
                  <div className="space-y-1.5">
                    <div className="flex items-start justify-between gap-2">
                      <div>
                        <h3 className="text-[14px] font-semibold text-ink">{inst.title}</h3>
                        <p className="font-mono text-[11px] text-ink-3">
                          v{inst.installedVersion} · {inst.approvedBundleSha256.slice(0, 12)}
                        </p>
                      </div>
                      {inst.permissionsPaused ? (
                        <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[10.5px] font-medium text-amber-600 dark:text-amber-400">
                          Paused
                        </span>
                      ) : null}
                    </div>
                    {inst.launcherPath ? (
                      <p className="truncate font-mono text-[11px] text-ink-3" title={inst.launcherPath}>
                        {inst.launcherPath}
                      </p>
                    ) : null}
                    <p className="text-[12px] text-ink-2">
                      {inst.permissions.length > 0
                        ? `Permissions: ${inst.permissions.join(', ')}`
                        : 'Sandbox only (Tier 0/1)'}
                    </p>
                  </div>
                  <div className="flex items-center justify-between gap-1.5 border-t border-border/60 pt-2.5">
                    <div className="flex items-center gap-1">
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => void clientRuntime.desktopApps?.openWindow(inst.appId, { title: inst.title })}
                      >
                        <ExternalLink size={12} />
                        Launch
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => setPermTarget({ app: appModel, installed: inst })}
                      >
                        <Shield size={12} />
                        Permissions
                      </Button>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => uninstallDesktopMutation.mutate(inst.appId)}
                    >
                      <Trash2 size={12} className="text-danger" />
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>
        )
      ) : connectedEntrypoints.length === 0 ? (
        <EmptyState title="No Connected MCP Apps">
          <p className="text-[13px] text-ink-3">
            Connect an MCP server that declares interactive UI resources to open its apps here.
          </p>
        </EmptyState>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {connectedEntrypoints.map((entry) => (
            <div
              key={`${entry.server_id}:${entry.tool}`}
              className="flex flex-col justify-between gap-3 rounded-card border border-border bg-surface p-4"
            >
              <div className="flex items-center gap-2.5">
                <span className="grid size-8 place-items-center rounded-control bg-surface-2 text-primary">
                  {entry.icon ? <AppIcon app={entry} /> : <Plug size={15} />}
                </span>
                <div className="min-w-0">
                  <h3 className="truncate text-[14px] font-semibold text-ink">{entry.title}</h3>
                  <p className="truncate font-mono text-[11px] text-ink-3">{entry.tool}</p>
                </div>
              </div>
              <div className="flex justify-end border-t border-border/60 pt-2.5">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() =>
                    void navigate({
                      to: '/apps/$serverId/$tool',
                      params: { serverId: entry.server_id, tool: entry.tool },
                    })
                  }
                >
                  Open Connected App
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {shareApp ? <AppShareModal open onClose={() => setShareApp(null)} app={shareApp} /> : null}
      {permTarget ? (
        <DesktopAppPermissionsModal
          open
          onClose={() => setPermTarget(null)}
          app={permTarget.app}
          installed={permTarget.installed}
          orgPolicy={orgPolicy.data}
          onSaved={() => {
            void queryClient.invalidateQueries({ queryKey: keys.desktopApps })
            toast('Updated Desktop App installation')
          }}
        />
      ) : null}
    </div>
  )
}
