import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { Plus, ZoomIn, ZoomOut } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { AppPageHeader } from '@/components/apps/AppPageHeader'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { AddTileModal } from '@/components/boards/AddTileModal'
import { BoardGrid } from '@/components/boards/BoardGrid'
import { LoopModal } from '@/components/loops/LoopModal'
import { ActionMenu, type MenuAction } from '@/components/ui/ActionMenu'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { Modal } from '@/components/ui/Modal'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { deleteApp, patchApp, type AppVersion, type JazApp } from '@/lib/api/apps'
import {
  type BoardLayoutEntry,
  type BoardView,
  boardViewQuery,
  boardsQuery,
  patchBoard,
  removeBoardTile,
} from '@/lib/api/boards'
import { loopDetailQuery } from '@/lib/api/loops'
import { clientRuntime, popOutBoard } from '@/lib/clientRuntime'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const route = getRouteApi('/apps/$appId')

const MIN_SCALE = 0.7
const MAX_SCALE = 2

// A board opened as the app it is: the app page's header over a grid of the
// apps it holds. Its own window swaps the name for a strip of every board.
export function BoardPage({ board }: { board: JazApp }) {
  const { add } = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const isBoardWindow = clientRuntime.windowKind === 'board'
  const view = useQuery(boardViewQuery(board.id))
  const boards = useQuery({ ...boardsQuery, enabled: isBoardWindow })
  const canEdit = !isViewer && board.can_edit !== false
  // "Add tile" picks from loops and apps; "New loop" in the picker hands off
  // to the loop modal. Either way the board scrolls the new tile into view.
  const [modal, setModal] = useState<'add' | 'new-loop' | null>(null)
  const [sharing, setSharing] = useState(false)
  // null while the name is shown; the draft while it is edited in place.
  const [draftTitle, setDraftTitle] = useState<string | null>(null)
  const pendingLoopId = useRef<string | null>(null)
  const scaleTimer = useRef<number | null>(null)
  const viewKey = keys.appBoard(board.id)
  const refreshView = () => queryClient.invalidateQueries({ queryKey: viewKey })

  const editingLoop = useMutation({
    mutationFn: (loopId: string) => queryClient.fetchQuery({ ...loopDetailQuery(loopId), staleTime: 0 }),
  })

  // ?add=1 opens the picker once, on arrival from creating the board.
  useEffect(() => {
    if (!add || !canEdit) return
    setModal('add')
    void navigate({ search: {}, replace: true })
  }, [add, canEdit, navigate])

  useEffect(() => {
    const loopId = pendingLoopId.current
    const tile = loopId ? view.data?.tiles.find((candidate) => candidate.loop?.id === loopId) : undefined
    if (!tile) return
    pendingLoopId.current = null
    requestAnimationFrame(() => {
      document.querySelector(`[data-tile="${tile.app.id}"]`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
  }, [view.data])

  useEffect(() => {
    if (isBoardWindow) document.title = `${board.title} — Jaz`
  }, [isBoardWindow, board.title])

  useEffect(() => () => {
    if (scaleTimer.current) window.clearTimeout(scaleTimer.current)
  }, [])

  const layoutMutation = useMutation({
    mutationFn: (entry: BoardLayoutEntry) => patchBoard(board.id, { layout: [entry] }),
    onMutate: async (entry) => {
      await queryClient.cancelQueries({ queryKey: viewKey })
      queryClient.setQueryData<BoardView>(viewKey, (prev) => prev && {
        ...prev,
        tiles: prev.tiles.map((tile) => (tile.app.id === entry.app_id ? { ...tile, ...entry } : tile)),
      })
    },
    onSettled: refreshView,
  })

  const removeMutation = useMutation({
    mutationFn: (appId: string) => removeBoardTile(board.id, appId),
    onSettled: refreshView,
  })

  const scaleMutation = useMutation({
    mutationFn: (fontScale: number) => patchBoard(board.id, { font_scale: fontScale }),
    onSettled: refreshView,
  })

  const renameMutation = useMutation({
    mutationFn: (title: string) => patchApp(board.id, { title }),
    onMutate: async (title) => {
      await queryClient.cancelQueries({ queryKey: keys.appDetail(board.id) })
      queryClient.setQueryData<{ app: JazApp; current_version: AppVersion | null }>(
        keys.appDetail(board.id),
        (prev) => prev && { ...prev, app: { ...prev.app, title } },
      )
    },
    onError: (err) => toast(err.message, 'danger'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.appsRoot }),
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteApp(board.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast('Deleted board')
      void navigate({ to: '/apps' })
    },
    onError: (err) => toast(err.message, 'danger'),
  })

  // Enter blurs the input and blur commits; Escape unmounts it without a blur.
  const commitRename = () => {
    const title = draftTitle?.trim()
    setDraftTitle(null)
    if (title && title !== board.title) renameMutation.mutate(title)
  }

  const scale = view.data?.font_scale ?? 1
  // A step shows at once; the save waits so quick steps settle on one value.
  const applyScale = (next: number) => {
    queryClient.setQueryData<BoardView>(viewKey, (prev) => prev && { ...prev, font_scale: next })
    if (!canEdit) return
    if (scaleTimer.current) window.clearTimeout(scaleTimer.current)
    scaleTimer.current = window.setTimeout(() => scaleMutation.mutate(next), 400)
  }
  const stepScale = (delta: number) =>
    applyScale(Math.min(MAX_SCALE, Math.max(MIN_SCALE, Math.round((scale + delta) * 10) / 10)))

  const actions: MenuAction[] = isBoardWindow ? [] : [
    { label: 'Open in window', onSelect: () => popOutBoard(board.id) },
    ...(canEdit ? [{
      label: 'Delete',
      onSelect: () => {
        if (window.confirm(deleteBoardWarning(board.title))) deleteMutation.mutate()
      },
    }] : []),
  ]

  const title = isBoardWindow ? (
    <nav aria-label="Boards" className="-mx-1.5 flex items-center gap-0.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      {(boards.data ?? [board]).map((candidate) => {
        const active = candidate.id === board.id
        return (
          <button
            key={candidate.id}
            type="button"
            title={candidate.title}
            aria-current={active ? 'page' : undefined}
            onClick={() => void navigate({ to: '/apps/$appId', params: { appId: candidate.id }, search: {} })}
            className={`h-7 max-w-44 shrink-0 truncate rounded-full px-2.5 text-[12.5px] font-medium transition-[background-color,color,transform] duration-150 active:scale-[0.96] ${
              active ? 'bg-ink/[0.07] text-ink' : 'cursor-pointer text-ink-3 hover:bg-ink/[0.05] hover:text-ink'
            }`}
          >
            {candidate.title}
          </button>
        )
      })}
    </nav>
  ) : draftTitle !== null ? (
    <input
      autoFocus
      value={draftTitle}
      aria-label="Board name"
      onFocus={(e) => e.currentTarget.select()}
      onChange={(e) => setDraftTitle(e.target.value)}
      onBlur={commitRename}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur()
        if (e.key === 'Escape') setDraftTitle(null)
      }}
      className="-mx-1.5 h-7 w-full max-w-80 rounded-md bg-ink/[0.06] px-1.5 outline-none"
    />
  ) : canEdit ? (
    <button
      type="button"
      title="Rename board"
      onClick={() => setDraftTitle(board.title)}
      className="-mx-1.5 block h-7 max-w-full cursor-text truncate rounded-md px-1.5 text-left transition-colors duration-150 hover:bg-ink/[0.06]"
    >
      {board.title}
    </button>
  ) : (
    <h1 className="truncate">{board.title}</h1>
  )

  return (
    <div className="flex h-full min-h-0 flex-col bg-surface">
      <AppPageHeader title={title}>
        <div className="flex items-center">
          <IconButton size="sm" aria-label="Smaller tile text" title="Smaller tile text" disabled={scale <= MIN_SCALE} onClick={() => stepScale(-0.1)}>
            <ZoomOut size={14} />
          </IconButton>
          {scale !== 1 ? (
            <button
              type="button"
              title="Reset tile text size"
              onClick={() => applyScale(1)}
              className="cursor-pointer rounded-full px-1 text-[11.5px] tabular-nums text-ink-3 transition-colors duration-150 hover:text-ink"
            >
              {Math.round(scale * 100)}%
            </button>
          ) : null}
          <IconButton size="sm" aria-label="Larger tile text" title="Larger tile text" disabled={scale >= MAX_SCALE} onClick={() => stepScale(0.1)}>
            <ZoomIn size={14} />
          </IconButton>
        </div>
        {canEdit ? (
          <Button size="sm" onClick={() => setModal('add')}>
            <Plus size={13} />
            Add tile
          </Button>
        ) : null}
        {cloud.active ? <Button size="sm" onClick={() => setSharing(true)}>Share</Button> : null}
        {actions.length ? <ActionMenu label="More actions" actions={actions} /> : null}
      </AppPageHeader>

      {view.isPending ? (
        <div className="mx-auto w-full max-w-5xl p-6">
          <SkeletonRows count={4} />
        </div>
      ) : view.isError ? (
        <EmptyState title="Board unavailable">Could not load this board's tiles.</EmptyState>
      ) : view.data.tiles.length === 0 ? (
        <EmptyState title="Nothing on this board yet">
          {canEdit ? (
            <>
              <p>Tiles show the apps your loops keep up to date, and any app you pin.</p>
              <Button className="mt-4" variant="primary" onClick={() => setModal('add')}>
                <Plus size={14} />
                Add tile
              </Button>
            </>
          ) : (
            <p>{`Shared by ${board.owner_display_name || board.owner_email || 'a teammate'}.`}</p>
          )}
        </EmptyState>
      ) : (
        // The top padding keeps a dragged tile's lift shadow inside the scroll clip.
        <div className={`scrollbar-quiet min-h-0 flex-1 overflow-y-auto px-5 pt-4 ${isBoardWindow ? 'pb-0' : 'pb-6'}`}>
          <BoardGrid
            view={view.data}
            onLayoutChange={canEdit ? layoutMutation.mutate : undefined}
            onRemove={canEdit ? removeMutation.mutate : undefined}
            onEdit={canEdit ? editingLoop.mutate : undefined}
            onAddTile={canEdit ? () => setModal('add') : undefined}
          />
        </div>
      )}

      {sharing ? <AppShareModal open onClose={() => setSharing(false)} app={board} /> : null}
      <AddTileModal
        open={modal === 'add'}
        onClose={() => setModal(null)}
        boardId={board.id}
        tiles={view.data?.tiles ?? []}
        onCreateNew={() => setModal('new-loop')}
        onAdded={(loopIds) => {
          pendingLoopId.current = loopIds[0] ?? null
        }}
      />
      <LoopModal
        key={board.id}
        open={modal === 'new-loop' || editingLoop.isSuccess}
        onClose={() => {
          setModal(null)
          editingLoop.reset()
        }}
        loop={editingLoop.data?.loop}
        boardIds={editingLoop.data?.boardIds}
        onCreated={(created) => {
          pendingLoopId.current = created.id
        }}
      />
      {editingLoop.isPending || editingLoop.isError ? (
        <Modal open onClose={editingLoop.reset} title="Edit loop" size="md">
          {editingLoop.isError ? (
            <div className="space-y-3">
              <p role="alert" className="text-[13px] text-danger">{editingLoop.error.message}</p>
              <Button variant="secondary" size="sm" onClick={() => editingLoop.mutate(editingLoop.variables)}>
                Retry
              </Button>
            </div>
          ) : (
            <SkeletonRows count={4} />
          )}
        </Modal>
      ) : null}
    </div>
  )
}

// Deleting a board keeps what was on it.
export function deleteBoardWarning(title: string): string {
  return `Delete board "${title}"? Its apps are kept. A loop on no other board stops publishing until you add it to one.`
}
