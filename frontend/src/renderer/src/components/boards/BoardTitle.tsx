import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useToast } from '@/components/ui/toast'
import { patchApp, type AppDetail, type JazApp } from '@/lib/api/apps'
import { boardsQuery } from '@/lib/api/boards'
import { clientRuntime } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'

// What a board's header calls it: its name, renamed in place, or in a board
// window a strip of every board to switch between.
export function BoardTitle({ board, canEdit }: { board: JazApp; canEdit: boolean }) {
  if (clientRuntime.windowKind === 'board') return <BoardTabs board={board} />
  return canEdit ? <BoardName board={board} /> : <h1 className="truncate">{board.title}</h1>
}

function BoardTabs({ board }: { board: JazApp }) {
  const navigate = useNavigate()
  const boards = useQuery(boardsQuery)
  return (
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
  )
}

function BoardName({ board }: { board: JazApp }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  // null while the name is shown; the draft while it is edited in place.
  const [draft, setDraft] = useState<string | null>(null)

  const rename = useMutation({
    mutationFn: (title: string) => patchApp(board.id, { title }),
    onMutate: async (title) => {
      await queryClient.cancelQueries({ queryKey: keys.appDetail(board.id) })
      queryClient.setQueryData<AppDetail>(keys.appDetail(board.id), (prev) => prev && { ...prev, app: { ...prev.app, title } })
    },
    onError: (err) => toast(err.message, 'danger'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.appsRoot }),
  })

  if (draft === null) {
    return (
      <button
        type="button"
        title="Rename board"
        onClick={() => setDraft(board.title)}
        className="-mx-1.5 block h-7 max-w-full cursor-text truncate rounded-md px-1.5 text-left transition-colors duration-150 hover:bg-ink/[0.06]"
      >
        {board.title}
      </button>
    )
  }

  // Enter blurs the input and blur commits; Escape unmounts it without a blur.
  const commit = () => {
    const title = draft.trim()
    setDraft(null)
    if (title && title !== board.title) rename.mutate(title)
  }

  return (
    <input
      autoFocus
      value={draft}
      aria-label="Board name"
      onFocus={(e) => e.currentTarget.select()}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur()
        if (e.key === 'Escape') setDraft(null)
      }}
      className="-mx-1.5 h-7 w-full max-w-80 rounded-md bg-ink/[0.06] px-1.5 outline-none"
    />
  )
}
