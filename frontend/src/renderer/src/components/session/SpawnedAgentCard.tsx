import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Ban, CheckCircle2, ChevronRight, CircleAlert, LoaderCircle, type LucideIcon } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import type { SessionEvent } from '@/lib/api/types'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { sessionMessagesQuery } from '@/lib/api/sessions'
import { agentLabel } from '@/lib/agentLabel'
import { relativeTime } from '@/lib/format/time'
import { normalized } from './TranscriptUtils'

type RunStatus = 'working' | 'completed' | 'failed' | 'cancelled'

const STATUS: Record<RunStatus, { label: string; className: string; Icon: LucideIcon; spin?: boolean }> = {
  working: { label: 'working', className: 'text-running', Icon: LoaderCircle, spin: true },
  completed: { label: 'completed', className: 'text-primary', Icon: CheckCircle2 },
  failed: { label: 'failed', className: 'text-danger', Icon: CircleAlert },
  cancelled: { label: 'cancelled', className: 'text-ink-3', Icon: Ban },
}

function runStatus(state: string | undefined): RunStatus {
  switch (normalized(state)) {
    case 'idle':
      return 'completed'
    case 'failed':
      return 'failed'
    case 'cancelled':
      return 'cancelled'
    default:
      return 'working'
  }
}

function childTitle(event: SessionEvent): string {
  const acp = event.acp
  return acp?.title || acp?.slug || 'child task'
}

// A spawned child agent's run, shown inside the parent transcript. The same card
// carries the run through its whole lifecycle (working → completed/failed/
// cancelled); expanding reveals the task prompt and a link into the child thread.
export function SpawnedAgentCard({ event }: { event: SessionEvent }) {
  const [open, setOpen] = useState(false)
  const childId = event.acp?.id ?? event.session_id
  const agent = event.acp?.agent
  const time = relativeTime(event.at)
  const status = STATUS[runStatus(event.acp?.state)]
  const messages = useQuery({ ...sessionMessagesQuery(childId), enabled: open })
  const prompt = messages.data?.messages.find((message) => message.role === 'user')?.content?.trim()

  return (
    <div className="flex w-fit min-w-[280px] max-w-[52ch] flex-col rounded-card bg-surface">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="group flex w-full items-center gap-3 px-4 py-3 text-left"
      >
        <AgentAvatar agent={agent} size={18} />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex items-center gap-1.5 text-[12px] text-ink-3">
            {agentLabel(agent)}
            <span className="text-ink-3/50" aria-hidden>·</span>
            <span className={`inline-flex items-center gap-1 ${status.className}`}>
              <status.Icon className={`size-3 ${status.spin ? 'animate-spin' : ''}`} aria-hidden />
              {status.label}
            </span>
            {time ? (
              <>
                <span className="text-ink-3/50" aria-hidden>·</span>
                <span className="tabular-nums">{time}</span>
              </>
            ) : null}
          </span>
          <span className="truncate text-[14px] font-medium text-ink">{childTitle(event)}</span>
        </span>
        <motion.span
          animate={{ rotate: open ? 90 : 0 }}
          transition={{ duration: 0.15, ease: 'easeOut' }}
          className="shrink-0 text-ink-3 transition-colors group-hover:text-ink-2"
        >
          <ChevronRight size={15} />
        </motion.span>
      </button>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="overflow-hidden"
          >
            <div className="mx-4 border-t border-border/70 py-3">
              <p className="text-[10.5px] font-medium uppercase tracking-wide text-ink-3">Prompt</p>
              {prompt ? (
                <p className="mt-1.5 max-h-48 overflow-auto whitespace-pre-wrap text-[13px] leading-relaxed text-ink-2 select-text">
                  {prompt}
                </p>
              ) : (
                <p className="mt-1.5 text-[13px] text-ink-3">
                  {messages.isFetching ? 'Loading prompt…' : 'No prompt recorded.'}
                </p>
              )}
              <Link
                to="/sessions/$sessionId"
                params={{ sessionId: childId }}
                className="mt-3 inline-flex text-[12.5px] text-ink-2 transition-colors hover:text-primary"
              >
                View thread →
              </Link>
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}
