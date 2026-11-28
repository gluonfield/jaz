import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ListFilter, Plus, Users } from 'lucide-react'
import { useMemo, useState, type ComponentProps, type ReactNode } from 'react'
import { AppCard } from '@/components/apps/AppCard'
import { AppIcon } from '@/components/apps/AppIcon'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { AppThumbnail, ThumbnailSheet } from '@/components/apps/AppThumbnail'
import { useDesktopApps } from '@/components/apps/useDesktopApps'
import { BoardModal } from '@/components/boards/BoardModal'
import { deleteBoardWarning } from '@/components/boards/BoardPage'
import { BoardThumbnail } from '@/components/boards/BoardThumbnail'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { SearchField } from '@/components/ui/SearchField'
import { Segmented } from '@/components/ui/Segmented'
import { Skeleton } from '@/components/ui/Skeleton'
import type { MenuAction } from '@/components/ui/ActionMenu'
import { useToast } from '@/components/ui/toast'
import {
  appsQuery,
  deleteApp,
  hasBundle,
  remixApp,
  type AppKind,
  type BundledApp,
  type JazApp,
} from '@/lib/api/apps'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import { popOutBoard } from '@/lib/clientRuntime'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { installFolder } from '@/lib/desktopApps'
import { shortDate } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'
import type { InstalledDesktopApp } from '@shared/desktopApps'

export const Route = createFileRoute('/apps/')({
  component: AppsLibraryPage,
})

type AppsTab = 'mine' | 'team' | 'desktop' | 'connected'
type KindFilter = 'all' | AppKind

const KINDS: { value: KindFilter; label: string }[] = [
  { value: 'all', label: 'All kinds' },
  { value: 'artifact', label: 'Interactive' },
  { value: 'document', label: 'Documents' },
  { value: 'board', label: 'Boards' },
]

const KIND_LABEL: Record<AppKind, string> = { artifact: 'Interactive', document: 'Document', board: 'Board' }

const EMPTY: Record<AppsTab, { title: string; text: string }> = {
  mine: { title: 'No apps yet', text: 'Publish an artifact or document from a chat, or gather apps on a board.' },
  team: { title: 'Nothing shared yet', text: 'Apps shared with your team or a space appear here.' },
  desktop: { title: 'No installed apps', text: 'Install an app to give it its own window and icon.' },
  connected: { title: 'No connected apps', text: 'Apps from your connected MCP servers appear here.' },
}

// An installed app whose published record is gone still shows, from what the
// desktop kept at install time.
function installedAsApp(installed: InstalledDesktopApp): BundledApp {
  return {
    id: installed.appId,
    owner_user_id: 'local',
    title: installed.title,
    description: installed.description,
    kind: installed.kind,
    visibility: 'private',
    current_version: installed.installedVersion,
    status: 'active',
    bundle_sha256: installed.latestBundleSha256,
    capabilities: installed.declaredCapabilities,
    created_at: installed.installedAt,
    updated_at: installed.updatedAt,
  }
}

function Grid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-5">{children}</div>
}

function AppsLibraryPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const desktop = useDesktopApps()

  const [tab, setTab] = useState<AppsTab>('mine')
  const [search, setSearch] = useState('')
  const [kindFilter, setKindFilter] = useState<KindFilter>('all')
  const [kindMenuOpen, setKindMenuOpen] = useState(false)
  const [shareApp, setShareApp] = useState<JazApp | null>(null)
  const [creatingBoard, setCreatingBoard] = useState(false)

  const allApps = useQuery(appsQuery({ scope: 'all' }))
  const mcpApps = useQuery(mcpEntrypointsQuery)

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteApp(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast('Deleted')
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not delete', 'danger'),
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

  const needle = search.trim().toLowerCase()
  const apps = useMemo(() => {
    const published = allApps.data ?? []
    const list = tab === 'mine' ? published.filter((app) => app.is_owner !== false)
      : tab === 'team' ? published.filter((app) => app.visibility !== 'private')
        : tab === 'desktop' ? [...desktop.records.values()].map((installed) => published.find((app) => app.id === installed.appId) ?? installedAsApp(installed))
          : []
    return list.filter((app) =>
      (kindFilter === 'all' || app.kind === kindFilter) &&
      (!needle || app.title.toLowerCase().includes(needle) || (app.description ?? '').toLowerCase().includes(needle)))
  }, [allApps.data, desktop.records, kindFilter, needle, tab])

  const tabs = [
    { value: 'mine' as const, label: 'Yours' },
    ...(cloud.active ? [{ value: 'team' as const, label: 'Shared' }] : []),
    ...(desktop.available ? [{ value: 'desktop' as const, label: 'Installed' }] : []),
    { value: 'connected' as const, label: 'Connected' },
  ]

  const deleteAction = (app: JazApp, warning: string): MenuAction[] => !isViewer && app.can_edit !== false ? [{
    label: 'Delete',
    onSelect: () => {
      if (window.confirm(warning)) deleteMutation.mutate(app.id)
    },
  }] : []

  // A board has a window of its own and nothing to install, remix or permit.
  const boardActions = (board: JazApp): MenuAction[] => [
    { label: 'Open in window', onSelect: () => popOutBoard(board.id) },
    { label: cloud.active ? 'Share' : 'Manage', onSelect: () => setShareApp(board) },
    ...deleteAction(board, deleteBoardWarning(board.title)),
  ]

  const appActions = (app: BundledApp): MenuAction[] => [
    ...[desktop.primary(app)].filter((action) => action !== null),
    { label: cloud.active ? 'Share' : 'Manage', onSelect: () => setShareApp(app) },
    ...(isViewer ? [] : [{ label: 'Remix in a new chat', onSelect: () => remixMutation.mutate(app.id) }]),
    ...desktop.actions(app),
    ...deleteAction(app, `Delete "${app.title}" and all its stored data?`),
  ]

  const cards: (ComponentProps<typeof AppCard> & { key: string })[] = tab === 'connected'
    ? (mcpApps.data ?? []).filter((entry) => entry.type === 'global' && (!needle || entry.title.toLowerCase().includes(needle))).map((entry) => ({
      key: `${entry.server_id}:${entry.tool}`,
      title: entry.title,
      detail: 'Connected',
      preview: (
        <ThumbnailSheet>
          <span className="grid size-full place-items-center pb-[6%] text-ink-2">
            <AppIcon app={entry} size={40} />
          </span>
        </ThumbnailSheet>
      ),
      onOpen: () => void navigate({ to: '/apps/$serverId/$tool', params: { serverId: entry.server_id, tool: entry.tool } }),
    }))
    : apps.map((app) => {
      const installed = desktop.records.get(app.id)
      return {
        key: app.id,
        title: app.title,
        detail: tab === 'desktop' && installed?.launcherPath
          ? `Installed in ${installFolder(installed.launcherPath)}`
          : `${KIND_LABEL[app.kind]} · ${shortDate(app.updated_at)}`,
        mark: installed?.permissionsPaused ? (
          <span title="Changed since you allowed its permissions" className="size-2 shrink-0 rounded-full bg-running" />
        ) : app.visibility !== 'private' ? (
          <span title={app.visibility === 'team' ? 'Shared with your team' : app.visibility === 'space' ? 'Shared with a space' : 'Shared with selected people'} className="shrink-0 text-ink-3">
            <Users size={15} aria-hidden />
          </span>
        ) : null,
        preview: hasBundle(app) ? <AppThumbnail app={app} /> : <BoardThumbnail boardId={app.id} />,
        actions: hasBundle(app) ? appActions(app) : boardActions(app),
        onOpen: () => void navigate({ to: '/apps/$appId', params: { appId: app.id } }),
      }
    })

  const filtered = Boolean(needle) || kindFilter !== 'all'
  const loading = tab === 'connected' ? mcpApps.isPending : allApps.isPending

  return (
    <div className="mx-auto flex w-full max-w-[1120px] flex-col px-8 pb-12 pt-10 max-sm:px-4 max-sm:pt-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-[26px] font-semibold tracking-[-0.02em] text-ink">Apps</h1>
        {isViewer ? null : (
          <Button variant="primary" onClick={() => setCreatingBoard(true)}>
            <Plus size={14} />
            New board
          </Button>
        )}
      </div>

      <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
        <Segmented value={tab} options={tabs} onChange={setTab} layoutId="apps-library-tab" />
        <div className="flex items-center gap-1">
          <SearchField value={search} onChange={setSearch} placeholder="Search apps…" className="h-8 w-52 max-sm:w-40" />
          {tab !== 'connected' ? (
            <Popover open={kindMenuOpen} onClose={() => setKindMenuOpen(false)} placement="below" align="end" trigger={
              <IconButton aria-label="Filter by kind" title="Filter by kind" aria-haspopup="menu" aria-expanded={kindMenuOpen} onClick={() => setKindMenuOpen((open) => !open)} className={kindFilter === 'all' ? '' : 'bg-ink/10 text-ink'}>
                <ListFilter size={15} />
              </IconButton>
            }>
              <div role="menu" aria-label="Filter by kind">
                {KINDS.map((kind) => (
                  <MenuRow key={kind.value} role="menuitemradio" selected={kindFilter === kind.value} onClick={() => {
                    setKindFilter(kind.value)
                    setKindMenuOpen(false)
                  }}>
                    {kind.label}
                  </MenuRow>
                ))}
              </div>
            </Popover>
          ) : null}
        </div>
      </div>

      <div className="mt-7">
        {loading ? (
          <Grid>
            {Array.from({ length: 6 }, (_, index) => (
              <div key={index} className="overflow-hidden rounded-[14px] border border-border">
                <Skeleton className="aspect-[16/10] rounded-none" />
                <div className="space-y-2 px-3.5 py-3">
                  <Skeleton className="h-3.5 w-2/3" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
              </div>
            ))}
          </Grid>
        ) : tab !== 'connected' && allApps.isError ? (
          <EmptyState title="Could not load apps">
            <Button className="mt-2" variant="primary" onClick={() => void allApps.refetch()}>Try again</Button>
          </EmptyState>
        ) : cards.length === 0 ? (
          filtered ? (
            <EmptyState title="No matches">Try a different search or filter.</EmptyState>
          ) : (
            <EmptyState title={EMPTY[tab].title}>
              <p>{EMPTY[tab].text}</p>
              {tab === 'mine' && !isViewer ? (
                <Button className="mt-4" variant="primary" onClick={() => void navigate({ to: '/new' })}>Start a chat</Button>
              ) : null}
            </EmptyState>
          )
        ) : (
          <Grid>
            {cards.map((card) => <AppCard {...card} key={card.key} />)}
          </Grid>
        )}
      </div>

      {creatingBoard ? <BoardModal open onClose={() => setCreatingBoard(false)} /> : null}
      {shareApp ? <AppShareModal open onClose={() => setShareApp(null)} app={shareApp} /> : null}
      {desktop.dialog}
    </div>
  )
}
