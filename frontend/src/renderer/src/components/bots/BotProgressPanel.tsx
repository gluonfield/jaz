import { Link } from '@tanstack/react-router'
import type { BotProgress } from '@/lib/botProgress'
import type { BotWork } from '@/lib/bots'

export function BotProgressPanel({ name, progress, activities, workers }: Pick<BotWork, 'progress' | 'activities' | 'workers'> & { name: string }) {
  return (
    <div className="space-y-1 break-words rounded-xl border border-border px-3 py-2.5">
      <p className="text-xs text-ink-3">{name}</p>
      {progress ? <ProgressDetails progress={progress} /> : <p className="font-medium text-ink">Tasks and threads</p>}
      {activities?.map((activity, index) => <p key={`${index}:${activity}`}>{activity}</p>)}
      {workers?.map(({ id, title, progress, message }) => (
        <div key={id} className="space-y-1 border-t border-border pt-2">
          <Link to="/sessions/$sessionId" params={{ sessionId: id }} className="font-medium text-ink underline underline-offset-2">
            {title}
          </Link>
          <ProgressDetails progress={progress} />
          {message ? (
            <div className="mt-2 border-l border-border pl-2">
              <p className="text-xs text-ink-3">Latest agent message</p>
              <p className="line-clamp-5 whitespace-pre-wrap text-ink-2">{message}</p>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  )
}

function ProgressDetails({ progress }: { progress: BotProgress }) {
  const stale = progress.stale && progress.kind !== 'error'
  return (
    <>
      <p className="font-medium text-ink">{stale ? 'Waiting for a new agent event' : progress.label}</p>
      {stale ? <p>Last observed phase: {progress.label}</p> : null}
      {progress.toolDetail ? <p className="whitespace-pre-wrap">Tool: {progress.toolDetail}</p> : null}
      {progress.planStep ? <p className="whitespace-pre-wrap">Plan: {progress.planStep}</p> : null}
      {progress.waitReason ? <p>{progress.waitReason}</p> : stale ? <p>The agent has not reported a reason for the pause</p> : null}
      {progress.ageMs != null ? <p className="text-xs text-ink-3" aria-live="off">Last event: {Math.round(progress.ageMs / 1000)}s ago</p> : null}
    </>
  )
}
