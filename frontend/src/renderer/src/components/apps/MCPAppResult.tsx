import { ArrowUpRight } from 'lucide-react'
import zoom from '@/assets/integrations/zoom.png'
import { MCPAppFrame } from '@/components/apps/MCPAppFrame'
import type { MCPAppEvent } from '@/lib/api/types'
import { openExternal } from '@/lib/clientRuntime'
import { clockTime, hasTime, messageTime } from '@/lib/format/time'
import { isPlainClick } from '@/lib/markdownLinks'
import { zoomMeetingResults } from '@/lib/zoomMeetingResults'

export function MCPAppResult({ app }: { app: MCPAppEvent }) {
  const zoomResults = zoomMeetingResults(app)
  if (!zoomResults) return <MCPAppFrame app={app} call={app} inline active />

  return (
    <>
      <section aria-label="Upcoming Zoom meetings" className="rounded-card border border-border bg-panel">
        <h3 className="px-3 py-2 text-sm font-semibold text-ink">Upcoming meetings ({zoomResults.joinable.length})</h3>
        <ul className="divide-y divide-border border-t border-border">
          {zoomResults.joinable.map((meeting, index) => {
            const { join_url, topic, schedule_start_time, schedule_end_time, host_name } = meeting
            const start = hasTime(schedule_start_time) ? messageTime(schedule_start_time) : ''
            const end = start && hasTime(schedule_end_time) ? clockTime(schedule_end_time) : ''
            return (
              <li key={`${join_url}-${index}`} className="last:[&>a]:rounded-b-card">
                <a
                  href={join_url}
                  target="_blank"
                  rel="noreferrer"
                  onClick={(event) => {
                    if (!isPlainClick(event)) return
                    event.preventDefault()
                    openExternal(join_url)
                  }}
                  className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5 text-ink transition-colors duration-150 hover:bg-surface focus-visible:bg-surface focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary motion-reduce:transition-none"
                >
                  <img src={zoom} alt="" className="size-8 shrink-0 rounded-control" />
                  <span className="min-w-24 flex-1">
                    <span className="block break-words text-sm font-medium">{topic || 'Zoom meeting'}</span>
                    {start && <span className="block text-xs text-ink-2 tabular-nums">{start}{end && ` – ${end}`}</span>}
                    {host_name && <span className="block break-words text-xs text-ink-2">Host: {host_name}</span>}
                  </span>
                  <span className="ml-auto inline-flex shrink-0 items-center gap-1 text-xs font-medium text-primary">
                    Open in Zoom <ArrowUpRight size={14} aria-hidden />
                  </span>
                </a>
              </li>
            )
          })}
        </ul>
      </section>
      {zoomResults.providerResult && <MCPAppFrame app={app} call={{ ...app, result: zoomResults.providerResult }} inline active />}
    </>
  )
}
