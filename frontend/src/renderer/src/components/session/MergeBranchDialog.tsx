import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { sessionRepoMergeBranches } from '@/lib/api/sessions'
import type { RepoMergeSelection } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export function MergeBranchDialog({
  sessionId,
  busy,
  onClose,
  onMerge,
}: {
  sessionId: string
  busy: boolean
  onClose: () => void
  onMerge: (selection: RepoMergeSelection) => Promise<boolean>
}) {
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<RepoMergeSelection | null>(null)
  const branches = useQuery({
    queryKey: [...keys.sessionRepo(sessionId), 'merge-branches'],
    queryFn: () => sessionRepoMergeBranches(sessionId),
    staleTime: 0,
  })
  const target = branches.data?.target
  const sources = branches.data?.branches.filter(({ branch }) => branch.toLowerCase().includes(filter.toLowerCase())) ?? []
  const selection = sources.find(({ branch, head }) => branch === selected?.source.branch && head === selected.source.head && target === selected.target)
  const canMerge = !branches.isError && !branches.isFetching && Boolean(selection)

  return (
    <Modal
      open
      title={target ? `Merge into ${target}` : 'Merge another branch'}
      description="Choose a branch from another worktree. Only committed changes are merged; uncommitted edits stay in that worktree."
      onClose={() => {
        if (!busy) onClose()
      }}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>Cancel</Button>
          <Button
            variant="primary"
            disabled={busy || !canMerge}
            onClick={() => {
              if (selected && canMerge) void onMerge(selected).then((ok) => {
                if (ok) onClose()
              })
            }}
          >
            {busy ? 'Merging…' : target ? `Merge into ${target}` : 'Merge'}
          </Button>
        </>
      }
    >
      <input
        aria-label="Filter worktree branches"
        placeholder="Find a branch…"
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
        className="mb-3 h-9 w-full rounded-control bg-surface-1 px-3 text-[13px] text-ink outline-none focus:ring-2 focus:ring-primary"
      />
      {branches.isError ? (
        <div className="space-y-2 text-[13px] text-danger">
          <p>Could not check worktree branches.</p>
          <Button onClick={() => void branches.refetch()}>Retry branches</Button>
        </div>
      ) : branches.isPending ? (
        <p className="text-[13px] text-ink-3">Checking worktree branches…</p>
      ) : sources.length ? (
        <fieldset className="flex max-h-72 flex-col gap-1 overflow-y-auto" disabled={busy}>
          <legend className="sr-only">Branch to merge</legend>
          {sources.map(({ branch, head, ahead }) => (
            <label key={branch} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-control px-2 py-2 hover:bg-surface-2">
              <input type="radio" name="merge-branch" value={branch} checked={selection?.branch === branch} onChange={() => {
                if (target) setSelected({ source: { branch, head }, target })
              }} />
              <span className="min-w-0 flex-1 break-words font-mono text-[12px] text-ink">{branch}</span>
              <span className="shrink-0 text-[12px] text-ink-3 tabular-nums">{ahead} {ahead === 1 ? 'commit' : 'commits'}</span>
            </label>
          ))}
        </fieldset>
      ) : (
        <p className="text-[13px] text-ink-3">{filter ? 'No matching branches.' : `No other worktree branches have commits to merge into ${target}.`}</p>
      )}
    </Modal>
  )
}
