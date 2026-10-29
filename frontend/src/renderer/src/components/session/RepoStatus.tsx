import type { RepoInfo } from '@/lib/api/types'

export function RepoStatus({ info, cwd }: { info: RepoInfo; cwd?: string }) {
  return (
    <dl className="space-y-1 px-2.5 py-1 text-[12px] leading-snug text-ink-3">
      <div className="flex items-baseline justify-between gap-3" title={cwd}>
        <dt className="shrink-0">This workspace</dt>
        <dd className="min-w-0 truncate text-ink-2">{info.is_worktree ? 'Worktree' : 'Local checkout'}{info.head ? ` · ${info.head.slice(0, 7)}` : ''}</dd>
      </div>
      <div className="flex items-baseline justify-between gap-3">
        <dt>Uncommitted</dt>
        <dd className={info.dirty ? 'text-running' : ''}>{info.dirty === undefined ? 'Unknown' : info.dirty ? 'Changes' : 'None'}</dd>
      </div>
      {info.is_worktree ? (
        <div className="flex items-baseline justify-between gap-3">
          <dt className="min-w-0 truncate" title={info.main_branch}>Local {info.main_branch || 'target'}</dt>
          <dd className={`shrink-0 ${(info.ahead ?? 0) > 0 ? 'text-running' : ''}`}>
            {!info.main_branch || info.ahead === undefined ? 'Unknown'
              : info.ahead > 0 ? `${info.ahead} ${info.ahead === 1 ? 'commit' : 'commits'} outside`
                : 'Commits included'}
          </dd>
        </div>
      ) : null}
      <div className="flex items-baseline justify-between gap-3" title="Compared with locally recorded tracking refs; this check does not fetch.">
        <dt className="shrink-0">Cached upstream</dt>
        <dd className="flex min-w-0 items-baseline gap-1.5">
          {info.upstream ? <><span className="min-w-0 truncate" title={info.upstream}>{info.upstream}</span><span className="shrink-0">{info.push_ahead === undefined ? 'unknown' : `${info.push_ahead} ahead`}</span></> : 'Unknown'}
        </dd>
      </div>
    </dl>
  )
}
