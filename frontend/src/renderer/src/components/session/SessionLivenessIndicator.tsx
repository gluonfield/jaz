import { CircleAlert } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useEffect, useState } from 'react'
import { LivePulse } from '@/components/ui/LivePulse'
import { agentLabel } from '@/lib/agentLabel'
import {
  deriveSessionRunSignal,
  type RunSignal,
} from '@/lib/sessionLiveness'

function formatDuration(ms: number | undefined): string {
  if (ms === undefined) return ''
  const seconds = Math.max(1, Math.floor(ms / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  return `${Math.floor(minutes / 60)}h`
}

function detailFor(signal: RunSignal, ageMs: number | undefined): string {
  const age = formatDuration(ageMs)
  if (signal === 'live') return age ? `- live ${age} ago` : '- live'
  if (signal === 'quiet') return age ? `- quiet for ${age}` : '- quiet'
  if (signal === 'stale') return age ? `no updates for ${age}` : 'no recent updates'
  return ''
}

export function SessionLivenessIndicator({
  agent,
  running,
  activeOperation,
  updatedAt,
  lastActivityAt,
}: {
  agent?: string
  running: boolean
  activeOperation?: string
  updatedAt: string
  lastActivityAt?: string
}) {
  const [, setTick] = useState(0)
  const { signal, ageMs } = deriveSessionRunSignal({
    running,
    updatedAt,
    lastActivityAt,
    now: Date.now(),
  })

  useEffect(() => {
    if (signal === 'idle') return
    const timer = window.setInterval(() => setTick((tick) => tick + 1), 1000)
    return () => window.clearInterval(timer)
  }, [signal])

  const stale = signal === 'stale'
  const detail = detailFor(signal, ageMs)
  const label = livenessLabel(agent, activeOperation, stale)

  return (
    <AnimatePresence initial={false}>
      {signal !== 'idle' ? (
        <motion.div
          role="status"
          initial={{ opacity: 0, y: 4, scale: 0.98 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: 4, scale: 0.98 }}
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          className={`flex w-fit max-w-full items-center gap-1.5 text-[12px] leading-5 ${
            stale ? 'text-danger' : 'text-ink-3 live-shimmer'
          }`}
        >
          {stale ? (
            <CircleAlert className="size-3.5 shrink-0" aria-hidden />
          ) : (
            <LivePulse className="text-running" />
          )}
          <span className="min-w-0 truncate">{label}</span>
          {detail ? (
            <span className="shrink-0 tabular-nums">{detail}</span>
          ) : null}
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}

function livenessLabel(agent: string | undefined, activeOperation: string | undefined, stale: boolean): string {
  if (activeOperation === 'compact') {
    return stale ? 'Compaction is still marked running' : 'Compacting'
  }
  return stale ? `${agentLabel(agent)} is still marked running` : 'Working'
}
