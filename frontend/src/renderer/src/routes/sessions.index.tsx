import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { SessionRow } from '@/components/sidebar/SessionRow'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { allSessionsQuery } from '@/lib/api/sessions'

export const Route = createFileRoute('/sessions/')({
  component: SessionsPage,
})

function SessionsPage() {
  const sessions = useQuery(allSessionsQuery)

  return (
    <div className="mx-auto max-w-[640px] px-10 pb-12">
      <header className="pb-6">
        <h1 className="text-lg font-semibold text-ink">All sessions</h1>
        {sessions.data ? (
          <p className="mt-0.5 text-[13px] text-ink-3">
            {sessions.data.length} session{sessions.data.length === 1 ? '' : 's'}
          </p>
        ) : null}
      </header>

      {sessions.isPending ? (
        <SkeletonRows count={8} />
      ) : sessions.isError ? (
        <EmptyState title="Couldn't load sessions">
          <p>{sessions.error.message}</p>
        </EmptyState>
      ) : sessions.data.length === 0 ? (
        <EmptyState title="No sessions yet">
          <p>
            Start one from a terminal with{' '}
            <code className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[12px]">jaz chat</code>{' '}
            and it will appear here.
          </p>
        </EmptyState>
      ) : (
        <div className="flex flex-col gap-px">
          {sessions.data.map((session) => (
            <SessionRow key={session.id} session={session} />
          ))}
        </div>
      )}
    </div>
  )
}
