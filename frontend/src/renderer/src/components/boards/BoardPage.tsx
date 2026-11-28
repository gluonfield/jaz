import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { AppPageHeader } from '@/components/apps/AppPageHeader'
import { AppShareModal } from '@/components/apps/AppShareModal'
import { useDeleteApp } from '@/components/apps/useDeleteApp'
import { AddTileModal } from '@/components/boards/AddTileModal'
import { BoardGrid } from '@/components/boards/BoardGrid'
import { BoardTitle } from '@/components/boards/BoardTitle'
import { BoardZoom } from '@/components/boards/BoardZoom'
import { LoopModal } from '@/components/loops/LoopModal'
import { ActionMenu, type MenuAction } from '@/components/ui/ActionMenu'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { SkeletonRows } from '@/components/ui/Skeleton'
import type { JazApp } from '@/lib/api/apps'
import {
  type BoardLayoutEntry,
  type BoardView,
  boardViewQuery,
  patchBoard,
  removeBoardTile,
} from '@/lib/api/boards'
import { loopDetailQuery } from '@/lib/api/loops'
import { clientRuntime, popOutBoard } from '@/lib/clientRuntime'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const route = getRouteApi('/apps/$appId')

// A board opened as the app it is: the app page's header over a grid of the
// apps it holds.
export function BoardPage({ board }: { board: JazApp }) {
  const { add } = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const isBoardWindow = clientRuntime.windowKind === 'board'
  const view = useQuery(boardViewQuery(board.id))
  const canEdit = !isViewer && board.can_edit !== false
  // "Add tile" picks from loops and apps; "New loop" in the picker hands off
  // to the loop modal. Either way the board scrolls the new tile into view.
  const [modal, setModal] = useState<'add' | 'new-loop' | null>(null)
  const [sharing, setSharing] = useState(false)
  const pendingLoopId = useRef<string | null>(null)
  const viewKey = keys.appBoard(board.id)
  const refreshView = () => queryClient.invalidateQueries({ queryKey: viewKey })
  const deleteAction = useDeleteApp(() => void navigate({ to: '/apps' }))

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

  const actions: MenuAction[] = isBoardWindow ? [] : [
    { label: 'Open in window', onSelect: () => popOutBoard(board.id) },
    ...deleteAction(board),
  ]

  return (
    <div className="flex h-full min-h-0 flex-col bg-surface">
      <AppPageHeader title={<BoardTitle board={board} canEdit={canEdit} />}>
        <BoardZoom boardId={board.id} scale={view.data?.font_scale ?? 1} save={canEdit} />
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
