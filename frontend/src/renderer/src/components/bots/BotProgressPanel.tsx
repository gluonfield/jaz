import type { BotProgress } from '@/lib/botProgress'

export function BotProgressPanel({ name, progress, activities }: { name: string; progress?: BotProgress; activities?: string[] }) {
  return (
    <div className="space-y-1 break-words rounded-xl border border-border px-3 py-2.5">
      <p className="text-xs text-ink-3">{name}</p>
      <p className="font-medium text-ink">{progress?.stale && progress.kind !== 'error' ? 'Waiting for a new agent event' : progress?.label ?? 'Tasks and threads'}</p>
      {progress?.stale && progress.kind !== 'error' ? <p>Last observed phase: {progress.label}</p> : null}
      {progress?.toolDetail ? <p className="whitespace-pre-wrap">Tool: {progress.toolDetail}</p> : null}
      {progress?.planStep ? <p className="whitespace-pre-wrap">Plan: {progress.planStep}</p> : null}
      {activities?.map((activity, index) => <p key={`${index}:${activity}`}>{activity}</p>)}
      {progress?.waitReason ? <p>{progress.waitReason}</p> : progress?.stale && progress.kind !== 'error' ? <p>The agent has not reported a reason for the pause</p> : null}
      {progress?.ageMs != null ? <p className="text-xs text-ink-3" aria-live="off">Last event: {Math.round(progress.ageMs / 1000)}s ago</p> : null}
    </div>
  )
}
