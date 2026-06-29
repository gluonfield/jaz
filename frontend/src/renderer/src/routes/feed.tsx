import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { AnimatePresence } from 'motion/react'
import { FeedCard } from '@/components/feed/FeedCard'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { feedQuery } from '@/lib/api/feed'

export const Route = createFileRoute('/feed')({
  component: FeedPage,
})

function FeedPage() {
  const feed = useQuery(feedQuery)

  return (
    <div className="mx-auto max-w-[680px] px-10 pb-12">
      <header className="pb-6">
        <h1 className="text-lg font-semibold text-ink">Feed</h1>
        {feed.data ? (
          <p className="mt-0.5 text-[13px] text-ink-3">
            {feed.data.length === 0
              ? 'Nothing waiting on you'
              : `${feed.data.length} thread${feed.data.length === 1 ? '' : 's'} to respond`}
          </p>
        ) : null}
      </header>

      {feed.isPending ? (
        <SkeletonRows count={5} />
      ) : feed.isError ? (
        <EmptyState title="Couldn't load the feed">
          <p>{feed.error.message}</p>
        </EmptyState>
      ) : feed.data.length === 0 ? (
        <EmptyState title="You're all caught up">
          <p>New replies across your threads will collect here.</p>
        </EmptyState>
      ) : (
        <div className="flex flex-col gap-2.5">
          <AnimatePresence initial={false}>
            {feed.data.map((item) => (
              <FeedCard key={item.id} item={item} />
            ))}
          </AnimatePresence>
        </div>
      )}
    </div>
  )
}
