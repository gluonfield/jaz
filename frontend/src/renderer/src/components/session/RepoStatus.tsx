import type { RepoInfo } from '@/lib/api/types'

export function RepoStatus({ info }: { info: RepoInfo }) {
  const mergePending = info.is_worktree && (!info.main_branch || info.ahead !== 0)
  if (info.dirty === false && !mergePending) return null

  return (
    <div className="space-y-1 px-2.5 py-1 text-[12px] leading-snug break-words text-ink-3">
      {info.dirty !== false ? (
        <p className={info.dirty ? 'text-running' : ''}>{info.dirty ? 'Uncommitted changes' : 'Change status unavailable'}</p>
      ) : null}
      {mergePending ? (
        <p className={(info.ahead ?? 0) > 0 ? 'text-running' : ''}>
          {!info.main_branch || info.ahead === undefined ? 'Merge status unavailable'
            : `${info.ahead} ${info.ahead === 1 ? 'commit' : 'commits'} to merge into ${info.main_branch}`}
        </p>
      ) : null}
    </div>
  )
}
