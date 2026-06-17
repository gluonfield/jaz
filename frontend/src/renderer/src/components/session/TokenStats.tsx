import { ChartNoAxesColumn } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { Popover } from '@/components/ui/Popover'
import type { Session } from '@/lib/api/types'
import { formatTokens } from '@/lib/format/tokens'
import { useContextWindow } from '@/lib/models'

// Titlebar token meter: an icon that unfolds into the session's cumulative
// token fields plus the live context-window fill.
export function TokenStats({ session }: { session: Session }) {
  const [open, setOpen] = useState(false)
  const contextWindow = useContextWindow(session)
  const usage = session.usage
  const input = usage?.input_tokens ?? 0
  const output = usage?.output_tokens ?? 0
  const cached = usage?.cached_input_tokens ?? 0
  const cacheWrite = usage?.cached_write_tokens ?? 0
  const reasoning = usage?.reasoning_output_tokens ?? 0
  const total = usage?.total_tokens ?? 0
  const context = usage?.context_tokens ?? 0
  if (input + output + cached + cacheWrite + reasoning + total + context === 0) return null

  const pct = contextWindow && context > 0 ? Math.min(100, Math.round((context / contextWindow) * 100)) : null

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="below"
      trigger={
        <IconButton
          size="sm"
          title="Token usage"
          aria-label="Token usage"
          onClick={() => setOpen((value) => !value)}
        >
          <ChartNoAxesColumn size={15} />
        </IconButton>
      }
    >
      <div className="flex min-w-[200px] flex-col gap-1 px-2 py-1.5">
        <StatRow label="Input" value={formatTokens(input)} />
        <StatRow label="Output" value={formatTokens(output)} />
        {total > 0 ? <StatRow label="Total" value={formatTokens(total)} /> : null}
        <StatRow label="Cache read" value={formatTokens(cached)} />
        {cacheWrite > 0 ? <StatRow label="Cache write" value={formatTokens(cacheWrite)} /> : null}
        {reasoning > 0 ? <StatRow label="Reasoning" value={formatTokens(reasoning)} /> : null}
        {context > 0 ? (
          <>
            <StatRow
              label="Latest context"
              value={
                contextWindow
                  ? `${formatTokens(context)} / ${formatTokens(contextWindow)}`
                  : formatTokens(context)
              }
            />
            {pct !== null ? (
              <div className="mt-0.5 flex items-center gap-2">
                <div className="h-1 flex-1 overflow-hidden rounded-full bg-surface-2">
                  <div className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
                </div>
                <span className="font-mono text-[10px] leading-none text-ink-3">{pct}%</span>
              </div>
            ) : null}
          </>
        ) : null}
      </div>
    </Popover>
  )
}

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 text-[11px] leading-tight">
      <span className="text-ink-2">{label}</span>
      <span className="font-mono text-ink">{value}</span>
    </div>
  )
}
