import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { AnimatePresence } from 'motion/react'
import { useState } from 'react'
import { FeedCard } from '@/components/feed/FeedCard'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { feedQuery } from '@/lib/api/feed'

export const Route = createFileRoute('/feed')({
  component: FeedPage,
})

function FeedPage() {
  const feed = useQuery(feedQuery)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  return (
    <div className="mx-auto flex min-h-full max-w-[var(--thread-max)] flex-col px-10 pb-12">
      <header className="shrink-0 pb-6">
        <h1 className="text-lg font-semibold text-ink">Feed</h1>
      </header>

      {feed.isPending ? (
        <SkeletonRows count={5} />
      ) : feed.isError ? (
        <div className="flex flex-1 items-center justify-center">
          <EmptyState title="Couldn't load the feed">
            <p>{feed.error.message}</p>
          </EmptyState>
        </div>
      ) : feed.data.length === 0 ? (
        <div className="flex flex-1 items-center justify-center">
          <EmptyState title="You're all caught up">
            <p>New replies across your threads will collect here.</p>
          </EmptyState>
        </div>
      ) : (
        <div className="flex flex-col">
          <AnimatePresence initial={false}>
            {feed.data.map((item) => (
              <FeedCard
                key={item.id}
                item={item}
                expanded={expandedId === item.id}
                onToggle={() => setExpandedId((prev) => (prev === item.id ? null : item.id))}
              />
            ))}
          </AnimatePresence>
        </div>
      )}
    </div>
  )
}
