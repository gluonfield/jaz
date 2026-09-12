import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArchiveRestore, CornerDownRight } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { sessionLabel } from '@/components/sidebar/SessionRow'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { SearchField } from '@/components/ui/SearchField'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { archivedSessionsQuery } from '@/lib/api/archivedSessions'
import { setSessionArchived } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { useDebouncedValue } from '@/lib/hooks/useDebouncedValue'
import { keys } from '@/lib/query/keys'

export function ArchivedThreadsSettings() {
  const [query, setQuery] = useState('')
  const search = useDebouncedValue(query.trim(), 200)

  return (
    <section className="pb-4">
      <div className="sticky top-0 z-10 bg-bg py-4">
        <p className="text-sm font-medium text-ink">Archived threads</p>
        <p className="mt-0.5 text-[13px] text-ink-2">Threads archived from the sidebar. Search by title.</p>
        <div className="mt-4">
          <SearchField
            value={query}
            onChange={setQuery}
            placeholder="Search archived threads…"
            className="h-10"
          />
        </div>
      </div>
      <ArchivedList key={search} query={search} />
    </section>
  )
}

function ArchivedList({ query }: { query: string }) {
  const archived = useInfiniteQuery(archivedSessionsQuery(query))
  const more = useRef<HTMLDivElement>(null)
  const { fetchNextPage, hasNextPage, isFetching, isError } = archived

  useEffect(() => {
    const target = more.current
    if (!target || !hasNextPage || isFetching || isError) return
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) void fetchNextPage()
    }, { rootMargin: '200px' })
    observer.observe(target)
    return () => observer.disconnect()
  }, [fetchNextPage, hasNextPage, isFetching, isError])

  if (archived.isPending) return <SkeletonRows count={3} />
  const sessions = archived.data?.pages.flatMap((page) => page.sessions) ?? []

  return (
    <div className="flex flex-col gap-px pb-2" aria-busy={isFetching}>
      {sessions.map((session) => <ArchivedRow key={session.id} session={session} />)}
      {!isError && sessions.length === 0 ? (
        <p className="py-2 text-[13px] text-ink-3" role="status">
          {query ? 'No matching archived threads.' : 'Nothing archived.'}
        </p>
      ) : null}
      {isError ? (
        <div className="flex items-center justify-between gap-3 py-2">
          <p className="text-[13px] text-danger" role="alert">{archived.error.message}</p>
          <Button
            variant="ghost"
            className="min-h-10"
            disabled={isFetching}
            onClick={() => {
              if (archived.isFetchNextPageError) void fetchNextPage()
              else void archived.refetch()
            }}
          >
            Retry
          </Button>
        </div>
      ) : null}
      <div ref={more}>
        {hasNextPage && !isError ? (
          <Button
            variant="ghost"
            className="min-h-10 w-full"
            disabled={isFetching}
            onClick={() => void fetchNextPage()}
          >
            {isFetching ? 'Loading more…' : 'Load more'}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

function ArchivedRow({ session }: { session: Session }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const unarchive = useMutation({
    mutationFn: () => setSessionArchived(session.id, false),
    onSuccess: () => toast(`Restored ${sessionLabel(session)}`),
    onError: (error: Error) => toast(`Couldn't restore: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
    },
  })

  return (
    <div className="flex items-center gap-2 rounded-full px-3 py-2 text-[13px] text-ink-2">
      {session.parent_id ? (
        <span title="Child thread" className="shrink-0 text-ink-3"><CornerDownRight size={12} /></span>
      ) : null}
      {session.runtime === 'acp' ? <RuntimeBadge session={session} compact /> : null}
      <span className="min-w-0 flex-1 truncate" title={sessionLabel(session)}>
        {sessionLabel(session)}
      </span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">
        {relativeTime(session.updated_at)}
      </span>
      <IconButton
        variant="ghost"
        size="xs"
        aria-label="Unarchive thread"
        title="Unarchive thread"
        disabled={unarchive.isPending}
        onClick={() => unarchive.mutate()}
      >
        <ArchiveRestore size={14} />
      </IconButton>
    </div>
  )
}
