import { useQuery } from '@tanstack/react-query'
import { Cloud, Folder } from 'lucide-react'
import { projectsQuery } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { useIsCloud } from '@/lib/connection'
import { fullTime, relativeTime } from '@/lib/format/time'

export function SessionHoverDetails({ session, title }: { session: Session; title: string }) {
  const isPrimaryCloud = useIsCloud()
  const isCloudSession = session.backend === 'cloud'
  const projects = useQuery({ ...projectsQuery, enabled: !isPrimaryCloud && !isCloudSession })
  const projectPath = session.runtime_ref?.project_path || session.runtime_ref?.cwd
  const project = projects.data?.find((item) => item.path === projectPath)?.name
    ?? projectPath?.split(/[\\/]+/).filter(Boolean).at(-1)
  const edited = relativeTime(session.updated_at)

  return (
    <>
      <div className="flex items-start gap-3">
        <span className="min-w-0 flex-1 break-words font-medium">{title}</span>
        {edited && (
          <time
            dateTime={session.updated_at}
            aria-label={`Last edited ${fullTime(session.updated_at)}`}
            className="shrink-0 text-[11px] leading-5 tabular-nums text-ink-3"
          >
            {edited}
          </time>
        )}
      </div>
      {isCloudSession ? (
        <div className="mt-1 flex items-center gap-1.5 text-ink-2">
          <Cloud size={13} className="shrink-0 text-primary" aria-hidden />
          <span className="min-w-0 truncate">Jaz Cloud</span>
        </div>
      ) : project ? (
        <div className="mt-1 flex items-start gap-2 text-ink-2">
          <Folder size={14} className="mt-0.5 shrink-0" aria-hidden />
          <span className="min-w-0 break-words">{project}</span>
        </div>
      ) : null}
    </>
  )
}
