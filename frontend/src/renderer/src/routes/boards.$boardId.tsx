import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ExternalLink, Plus, ZoomIn, ZoomOut } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { AddWidgetModal } from '@/components/boards/AddWidgetModal'
import { BoardGrid } from '@/components/boards/BoardGrid'
import { LoopModal } from '@/components/loops/LoopModal'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { SkeletonRows } from '@/components/ui/Skeleton'
import {
  type BoardDetail,
  type BoardLayoutEntry,
  boardDetailQuery,
  boardsQuery,
  patchBoard,
  removeWidgetFromBoard,
} from '@/lib/api/boards'
import { clientRuntime } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import { useTheme } from '@/lib/theme'

export const Route = createFileRoute('/boards/$boardId')({
  // ?add=1 opens the widget picker on arrival (set by BoardModal after create).
  validateSearch: (search): { add?: boolean } => (search.add ? { add: true } : {}),
  component: BoardPage,
})

function BoardPage() {
  const { boardId } = Route.useParams()
  const { add } = Route.useSearch()
  const navigate = Route.useNavigate()
  const detail = useQuery(boardDetailQuery(boardId))
  const queryClient = useQueryClient()
  const { resolved } = useTheme()
  const scaleTimer = useRef<number | null>(null)
  const isBoardWindow = clientRuntime.windowKind === 'board'
  const boards = useQuery({ ...boardsQuery, enabled: isBoardWindow })
  // "Add widget" opens a picker of existing loops first; "New loop" inside it
  // hands off to the loop modal. Either way the board stays put and scrolls
  // the fresh tile into view once it appears.
  const [modal, setModal] = useState<'add' | 'new-loop' | null>(null)
  const pendingLoopId = useRef<string | null>(null)
  // null = displaying; a string = the draft name being edited inline.
  const [draftName, setDraftName] = useState<string | null>(null)

  // Consume ?add=1 once: open the picker and strip the param so a reload
  // doesn't reopen it.
  useEffect(() => {
    if (!add) return
    setModal('add')
    void navigate({ search: {}, replace: true })
  }, [add, navigate])

  useEffect(() => {
    const loopId = pendingLoopId.current
    if (!loopId || !detail.data) return
    const item = detail.data.items.find((it) => it.loop_id === loopId)
    if (!item) return
    pendingLoopId.current = null
    requestAnimationFrame(() => {
      document
        .querySelector(`[data-widget-id="${item.widget_id}"]`)
        ?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
  }, [detail.data])

  useEffect(() => {
    if (isBoardWindow && detail.data?.board.name) {
      document.title = `${detail.data.board.name} — Jaz`
    }
  }, [isBoardWindow, detail.data?.board.name])

  const layoutMutation = useMutation({
    mutationFn: (entry: BoardLayoutEntry) => patchBoard(boardId, { layout: [entry] }),
    onMutate: async (entry) => {
      await queryClient.cancelQueries({ queryKey: keys.boardDetail(boardId) })
      queryClient.setQueryData<BoardDetail>(keys.boardDetail(boardId), (prev) =>
        prev
          ? {
              ...prev,
              items: prev.items.map((item) =>
                item.widget_id === entry.widget_id
                  ? { ...item, x: entry.x, y: entry.y, w: entry.w, h: entry.h, placed_by: 'user' }
                  : item,
              ),
            }
          : prev,
      )
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.boardDetail(boardId) }),
  })

  const removeMutation = useMutation({
    mutationFn: (widgetId: string) => removeWidgetFromBoard(boardId, widgetId),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.boardDetail(boardId) }),
  })

  const scaleMutation = useMutation({
    mutationFn: (fontScale: number) => patchBoard(boardId, { font_scale: fontScale }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.boardDetail(boardId) }),
  })

  const renameMutation = useMutation({
    mutationFn: (name: string) => patchBoard(boardId, { name }),
    onMutate: async (name) => {
      await queryClient.cancelQueries({ queryKey: keys.boardDetail(boardId) })
      queryClient.setQueryData<BoardDetail>(keys.boardDetail(boardId), (prev) =>
        prev ? { ...prev, board: { ...prev.board, name } } : prev,
      )
    },
    // keys.boards is a prefix of boardDetail, so this refreshes the sidebar
    // list and this page in one invalidation.
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.boards }),
  })

  if (detail.isPending) {
    return (
      <div className="mx-auto max-w-5xl p-6">
        <SkeletonRows count={4} />
      </div>
    )
  }
  if (detail.isError) {
    return <EmptyState title="Board unavailable">Could not load this board.</EmptyState>
  }

  const { board, items } = detail.data
  const boardTabs =
    isBoardWindow && boards.data
      ? boards.data.some((candidate) => candidate.id === board.id)
        ? boards.data
        : [board, ...boards.data]
      : [board]
  // Single commit path: Enter blurs the input, blur commits. Escape unmounts
  // the input without blurring, so a cancel never saves.
  const commitRename = () => {
    if (draftName === null) return
    const name = draftName.trim()
    setDraftName(null)
    if (name && name !== board.name) renameMutation.mutate(name)
  }
  const scale = board.font_scale > 0 ? board.font_scale : 1
  // Steps apply to the cache immediately; the PATCH is debounced so rapid
  // clicks settle on one absolute value instead of racing over HTTP.
  const applyScale = (next: number) => {
    queryClient.setQueryData<BoardDetail>(keys.boardDetail(boardId), (prev) =>
      prev ? { ...prev, board: { ...prev.board, font_scale: next } } : prev,
    )
    if (scaleTimer.current) window.clearTimeout(scaleTimer.current)
    scaleTimer.current = window.setTimeout(() => scaleMutation.mutate(next), 400)
  }
  const stepScale = (delta: number) => {
    const cached = queryClient.getQueryData<BoardDetail>(keys.boardDetail(boardId))
    const current = cached?.board.font_scale || scale
    applyScale(Math.min(2, Math.max(0.7, Math.round((current + delta) * 10) / 10)))
  }
  const popOut = () => {
    if (clientRuntime.openBoardWindow) {
      clientRuntime.openBoardWindow(boardId)
      return
    }
    window.open(`/boards/${boardId}`, '_blank', 'noopener,noreferrer')
  }

  // Board windows stay chrome-light; embedded boards carry their name and the
  // explicit window escape hatch in the app page header.
  return (
    <div className="flex h-full flex-col bg-surface">
      <div
        className={`flex shrink-0 items-center gap-3 px-6 pb-1 ${
          isBoardWindow ? 'justify-between pt-0' : 'justify-between pt-2'
        }`}
      >
        {isBoardWindow ? (
          <nav aria-label="Boards" className="min-w-0 flex-1 overflow-hidden">
            <div className="flex min-w-0 items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
              {boardTabs.map((candidate) => {
                const active = candidate.id === boardId
                return (
                  <button
                    key={candidate.id}
                    type="button"
                    title={candidate.name}
                    onClick={() => {
                      if (!active) {
                        void navigate({
                          to: '/boards/$boardId',
                          params: { boardId: candidate.id },
                          search: {},
                        })
                      }
                    }}
                    className={`flex h-6 max-w-44 shrink-0 cursor-pointer items-center rounded-full px-2.5 text-[12px] font-medium transition-[background-color,color,transform] duration-150 active:scale-[0.96] ${
                      active
                        ? 'bg-primary-soft text-ink'
                        : 'text-ink-3 hover:bg-surface-2 hover:text-ink'
                    }`}
                  >
                    <span className="min-w-0 truncate">{candidate.name}</span>
                  </button>
                )
              })}
            </div>
          </nav>
        ) : (
          <h1 className="min-w-0 flex-1 text-lg font-semibold text-ink">
            {draftName === null ? (
              <button
                type="button"
                title="Rename board"
                onClick={() => setDraftName(board.name)}
                className="-mx-1.5 block max-w-full cursor-text truncate rounded-md px-1.5 text-left transition-colors duration-150 hover:bg-surface-2"
              >
                {board.name}
              </button>
            ) : (
              <input
                autoFocus
                value={draftName}
                aria-label="Board name"
                onFocus={(e) => e.currentTarget.select()}
                onChange={(e) => setDraftName(e.target.value)}
                onBlur={commitRename}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') e.currentTarget.blur()
                  if (e.key === 'Escape') setDraftName(null)
                }}
                className="-mx-1.5 w-full rounded-md bg-surface-2 px-1.5 text-lg font-semibold text-ink outline-none"
              />
            )}
          </h1>
        )}
        <div className="flex shrink-0 items-center gap-1">
          {isBoardWindow ? null : (
            <Button variant="secondary" size="sm" onClick={popOut}>
              <ExternalLink size={13} />
              Pop Out
            </Button>
          )}
          <IconButton
            variant="ghost"
            size="xs"
            aria-label="Smaller widget text"
            title="Smaller widget text"
            disabled={scale <= 0.7}
            onClick={() => stepScale(-0.1)}
          >
            <ZoomOut size={13} />
          </IconButton>
          {scale !== 1 ? (
            <button
              type="button"
              title="Reset widget text size"
              onClick={() => applyScale(1)}
              className="rounded-full px-1 text-[11px] tabular-nums text-ink-3 transition-colors duration-150 hover:text-ink"
            >
              {Math.round(scale * 100)}%
            </button>
          ) : null}
          <IconButton
            variant="ghost"
            size="xs"
            aria-label="Larger widget text"
            title="Larger widget text"
            disabled={scale >= 2}
            onClick={() => stepScale(0.1)}
          >
            <ZoomIn size={13} />
          </IconButton>
        </div>
      </div>
      {items.length === 0 ? (
        <EmptyState title="Nothing on this board yet">
          <p>Widgets are live tiles your loops keep up to date. Add one to get started.</p>
          <div className="mt-3 flex justify-center">
            <Button variant="primary" size="sm" onClick={() => setModal('add')}>
              <Plus size={13} />
              Add widget
            </Button>
          </div>
        </EmptyState>
      ) : (
        // pt-1: a dragged tile's lift shadow is drawn outside the box; with no
        // top padding the overflow clip shaves it off row-0 tiles.
        <div
          className={`min-h-0 flex-1 overflow-y-auto px-6 pt-1 ${
            isBoardWindow ? 'pb-0' : 'pb-6'
          }`}
        >
          <BoardGrid
            board={board}
            items={items}
            theme={resolved}
            scale={scale}
            onLayoutChange={(entry) => layoutMutation.mutate(entry)}
            onRemove={(widgetId) => removeMutation.mutate(widgetId)}
            onNewWidget={() => setModal('add')}
          />
        </div>
      )}
      <AddWidgetModal
        open={modal === 'add'}
        onClose={() => setModal(null)}
        boardId={boardId}
        assignedLoopIds={items.map((item) => item.loop_id)}
        onCreateNew={() => setModal('new-loop')}
        onAssigned={(loopIds) => {
          pendingLoopId.current = loopIds[0] ?? null
        }}
      />
      <LoopModal
        open={modal === 'new-loop'}
        onClose={() => setModal(null)}
        onCreated={(created) => {
          pendingLoopId.current = created.id
        }}
      />
    </div>
  )
}
