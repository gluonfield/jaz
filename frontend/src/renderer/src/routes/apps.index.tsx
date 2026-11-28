import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ListFilter, Plus, Users } from 'lucide-react'
import { useMemo, useState, type ComponentProps, type ReactNode } from 'react'
import { AppCard } from '@/components/apps/AppCard'
import { AppIcon } from '@/components/apps/AppIcon'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { AppThumbnail, ThumbnailSheet } from '@/components/apps/AppThumbnail'
import { useDeleteApp } from '@/components/apps/useDeleteApp'
import { useDesktopApps } from '@/components/apps/useDesktopApps'
import { BoardModal } from '@/components/boards/BoardModal'
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
  APP_KIND_LABEL,
  appsQuery,
  hasBundle,
  remixApp,
  type BundleKind,
  type BundledApp,
  type JazApp,
} from '@/lib/api/apps'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import { APPS_TABS, libraryApps, type AppsTab } from '@/lib/appsLibrary'
import { popOutBoard } from '@/lib/clientRuntime'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { installFolder } from '@/lib/desktopApps'
import { shortDate } from '@/lib/format/time'

export const Route = createFileRoute('/apps/')({
  // The tab rides in the URL so the way back from a board lands on Boards.
  validateSearch: (search): { tab?: AppsTab } => {
    const tab = APPS_TABS.find((value) => value === search.tab)
    return tab ? { tab } : {}
  },
  component: AppsLibraryPage,
})

type KindFilter = 'all' | BundleKind

const KINDS: { value: KindFilter; label: string }[] = [
  { value: 'all', label: 'All kinds' },
  { value: 'artifact', label: 'Interactive' },
  { value: 'document', label: 'Documents' },
]

const EMPTY: Record<AppsTab, { title: string; text: string }> = {
  mine: { title: 'No apps yet', text: 'Publish an artifact or document from a chat.' },
  team: { title: 'Nothing shared yet', text: 'Apps shared with your team or a space appear here.' },
  desktop: { title: 'No installed apps', text: 'Install an app to give it its own window and icon.' },
  connected: { title: 'No connected apps', text: 'Apps from your connected MCP servers appear here.' },
  boards: { title: 'No boards yet', text: 'A board holds apps as tiles, and keeps the apps your loops publish up to date.' },
}

function Grid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-5">{children}</div>
}

function AppsLibraryPage() {
  const navigate = useNavigate()
  const toast = useToast()
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const desktop = useDesktopApps()

  const tabs = [
    { value: 'mine' as const, label: 'Yours' },
    ...(cloud.active ? [{ value: 'team' as const, label: 'Shared' }] : []),
    ...(desktop.available ? [{ value: 'desktop' as const, label: 'Installed' }] : []),
    { value: 'connected' as const, label: 'Connected' },
    { value: 'boards' as const, label: 'Boards' },
  ]
  const requested = Route.useSearch().tab
  const tab = tabs.find((option) => option.value === requested)?.value ?? 'mine'
  const boards = tab === 'boards'
  const [search, setSearch] = useState('')
  const [kindFilter, setKindFilter] = useState<KindFilter>('all')
  const [kindMenuOpen, setKindMenuOpen] = useState(false)
  const [shareApp, setShareApp] = useState<JazApp | null>(null)
  const [creatingBoard, setCreatingBoard] = useState(false)

  const allApps = useQuery(appsQuery({ scope: 'all' }))
  const mcpApps = useQuery(mcpEntrypointsQuery)

  const deleteAction = useDeleteApp()

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
  // Boards are one kind and connected apps have none, so neither is filtered by it.
  const kinds = !boards && tab !== 'connected'
  const kind = kinds ? kindFilter : 'all'
  const apps = useMemo(
    () => libraryApps(tab, allApps.data ?? [], [...desktop.records.values()]).filter((app) =>
      (kind === 'all' || app.kind === kind) &&
      (!needle || app.title.toLowerCase().includes(needle) || (app.description ?? '').toLowerCase().includes(needle))),
    [allApps.data, desktop.records, kind, needle, tab],
  )

  // A board has a window of its own and nothing to install, remix or permit.
  const boardActions = (board: JazApp): MenuAction[] => [
    { label: 'Open in window', onSelect: () => popOutBoard(board.id) },
    { label: cloud.active ? 'Share' : 'Manage', onSelect: () => setShareApp(board) },
    ...deleteAction(board),
  ]

  const appActions = (app: BundledApp): MenuAction[] => [
    ...[desktop.primary(app)].filter((action) => action !== null),
    { label: cloud.active ? 'Share' : 'Manage', onSelect: () => setShareApp(app) },
    ...(isViewer ? [] : [{ label: 'Remix in a new chat', onSelect: () => remixMutation.mutate(app.id) }]),
    ...desktop.actions(app),
    ...deleteAction(app),
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
          : hasBundle(app) ? `${APP_KIND_LABEL[app.kind]} · ${shortDate(app.updated_at)}` : shortDate(app.updated_at),
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

  const filtered = Boolean(needle) || kind !== 'all'
  const loading = tab === 'connected' ? mcpApps.isPending : allApps.isPending

  return (
    <div className="mx-auto flex w-full max-w-[1120px] flex-col px-8 pb-12 pt-10 max-sm:px-4 max-sm:pt-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-[26px] font-semibold tracking-[-0.02em] text-ink">Apps</h1>
        {boards && !isViewer ? (
          <Button variant="primary" onClick={() => setCreatingBoard(true)}>
            <Plus size={14} />
            New board
          </Button>
        ) : null}
      </div>

      <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
        <Segmented value={tab} options={tabs} onChange={(next) => void navigate({ to: '/apps', search: { tab: next }, replace: true })} layoutId="apps-library-tab" />
        <div className="flex items-center gap-1">
          <SearchField value={search} onChange={setSearch} placeholder={boards ? 'Search boards…' : 'Search apps…'} className="h-8 w-52 max-sm:w-40" />
          {kinds ? (
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
