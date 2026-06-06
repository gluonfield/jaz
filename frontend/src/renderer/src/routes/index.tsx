import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { EmptyState } from '@/components/ui/EmptyState'
import { healthQuery } from '@/lib/api/sessions'

export const Route = createFileRoute('/')({
  component: HomePage,
})

function HomePage() {
  const health = useQuery(healthQuery)

  if (health.isError) {
    return (
      <EmptyState title="The jaz backend isn't running">
        <p>
          Start it with <code className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[12px]">jaz serve</code>{' '}
          and this window will reconnect on its own.
        </p>
      </EmptyState>
    )
  }

  return (
    <div className="mx-auto max-w-[640px] px-10 py-12">
      <h1 className="text-[1.375rem] font-semibold text-ink">{greeting()}</h1>
      <p className="mt-2 max-w-[52ch] text-sm text-ink-2">
        Your assistant is connected. Pick a recent session from the sidebar to watch it work, or
        open <Link to="/agent" className="font-medium text-primary hover:underline">Agent</Link> to
        shape how it thinks.
      </p>
    </div>
  )
}

function greeting(): string {
  const hour = new Date().getHours()
  if (hour < 5) return 'Up late?'
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}
