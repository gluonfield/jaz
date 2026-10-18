import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, useState, type ReactNode } from 'react'
import { Modal } from '@/components/ui/Modal'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotFileName, BotOwed, BotOwedThread } from '@/lib/api/types'
import { BOT_FILE_LABELS } from '@/lib/bots'
import { clockTime, formatDuration, messageTime, shortDate } from '@/lib/format/time'
import { type ProviderSubagentView, subagentRunning } from '@/lib/providerSubagents'

// A row says what happened and how to open it: a thread, a routine, a file, or
// a message or commitment shown in full. Since marks what is still owed.
type Row = {
  key: string
  title: string
  detail?: string
  at: string
  running?: boolean
  since?: boolean
  danger?: boolean
  open?: { thread: string } | { routine: string } | { file: BotFileName } | { message: BotActivity } | { owed: BotOwed }
}

export function BotActivity({
  bot,
  subagents,
  onOpenRoutine,
  onOpenFile,
}: {
  bot: Bot
  subagents: ProviderSubagentView[]
  onOpenRoutine: (id: string) => void
  onOpenFile: (name: BotFileName) => void
}) {
  const activity = useQuery(botActivityQuery(bot.id))
  const [message, setMessage] = useState<BotActivity | null>(null)
  const [owedShown, setOwedShown] = useState<BotOwed | null>(null)
  const now = useNow()
  if (activity.isPending) return null
  const rows = (activity.data?.items ?? []).map(row)
  const owed = (activity.data?.owed ?? []).map(owedRow)
  const helpers: Row[] = subagents.filter(subagentRunning).map((subagent) => ({
    key: subagent.key,
    title: subagent.name || subagent.task || 'Helper',
    detail: 'Helper',
    at: subagent.started_at_ms ? new Date(subagent.started_at_ms).toISOString() : '',
    running: true,
  }))
  const running = [...rows.filter((item) => item.running), ...helpers]
  const days = groupByDay(rows.filter((item) => !item.running), now)
  if (!owed.length && !running.length && !days.length) {
    return <p className="text-[13px] text-ink-3">What {bot.name} owes, its side chats, routine runs and messages show up here.</p>
  }
  const open = (target: Row['open']) => {
    if (!target) return
    if ('routine' in target) onOpenRoutine(target.routine)
    else if ('file' in target) onOpenFile(target.file)
    else if ('message' in target) setMessage(target.message)
    else if ('owed' in target) setOwedShown(target.owed)
  }
  return (
    <div className="flex flex-col gap-5">
      {owed.length ? (
        <Group title="Owed">
          {owed.map((item) => (
            <ActivityRow key={item.key} item={item} now={now} onOpen={open} />
          ))}
        </Group>
      ) : null}
      {running.length ? (
        <Group title="Now">
          {running.map((item) => (
            <ActivityRow key={item.key} item={item} now={now} onOpen={open} />
          ))}
        </Group>
      ) : null}
      {days.map(([day, list]) => (
        <Group key={day} title={day}>
          {list.map((item) => (
            <ActivityRow key={item.key} item={item} now={now} onOpen={open} />
          ))}
        </Group>
      ))}
      {owedShown ? <OwedModal owed={owedShown} onClose={() => setOwedShown(null)} /> : null}
      {message ? (
        <Modal open onClose={() => setMessage(null)} title={messageTitle(message)} description={messageTime(message.at)} size="sm">
          <p className="text-[13px] leading-relaxed whitespace-pre-wrap text-ink">{message.text}</p>
        </Modal>
      ) : null}
    </div>
  )
}

// owedRow is a commitment still open: who it is for, how its threads stand
// and the next step. It opens its one live thread, or else its details.
function owedRow(owed: BotOwed): Row {
  const live = owed.threads?.filter((thread) => thread.state !== 'gone') ?? []
  return {
    key: `owed:${owed.id}`,
    title: owed.request,
    detail: [`For ${owed.for ?? 'you'}`, threadsState(live), owed.next && `Next: ${owed.next}`].filter(Boolean).join(' · '),
    at: owed.opened_at,
    since: true,
    danger: live.some((thread) => thread.state === 'failed'),
    open: live.length === 1 ? { thread: live[0].id } : { owed },
  }
}

// threadsState says how far the threads serving a commitment have got.
function threadsState(threads: BotOwedThread[]): string {
  if (threads.some((thread) => thread.state === 'working')) return 'Working'
  if (threads.some((thread) => thread.state === 'failed')) return 'A thread failed'
  if (threads.some((thread) => thread.state === 'finished')) return 'Result in'
  return ''
}

const THREAD_STATES: Record<BotOwedThread['state'], string> = { working: 'Working', finished: 'Finished', failed: 'Failed', idle: 'Not started', gone: 'Archived' }

// OwedModal shows a commitment with no single thread to open: its next step
// and the threads serving it.
function OwedModal({ owed, onClose }: { owed: BotOwed; onClose: () => void }) {
  return (
    <Modal open onClose={onClose} title={owed.request} description={`For ${owed.for ?? 'you'} · since ${messageTime(owed.opened_at)}`} size="sm">
      <div className="flex flex-col gap-3 text-[13px] leading-relaxed text-ink">
        {owed.next ? <p className="whitespace-pre-wrap">Next: {owed.next}</p> : null}
        {owed.threads?.length ? (
          <ul className="flex flex-col">
            {owed.threads.map((thread) => (
              <li key={thread.id}>
                <Link to="/sessions/$sessionId" params={{ sessionId: thread.id }} onClick={onClose} className="-mx-2 flex items-baseline justify-between gap-3 rounded-lg px-2 py-1.5 transition-colors duration-150 hover:bg-list-hover">
                  <span className="truncate font-medium">{thread.title}</span>
                  <span className={`shrink-0 text-[12px] ${thread.state === 'failed' ? 'text-danger' : thread.state === 'working' ? 'text-running' : 'text-ink-3'}`}>{THREAD_STATES[thread.state]}</span>
                </Link>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </Modal>
  )
}

function row(item: BotActivity, index: number): Row {
  const key = `${item.kind}:${item.thread_id ?? item.loop_id ?? ''}:${item.at}:${index}`
  const base = { key, at: item.at, running: item.running }
  switch (item.kind) {
    case 'side_chat':
      return {
        ...base,
        title: item.title || 'Side chat',
        detail: item.status === 'error' ? 'Side chat · failed' : 'Side chat',
        danger: item.status === 'error',
        open: item.thread_id ? { thread: item.thread_id } : undefined,
      }
    case 'routine':
      return {
        ...base,
        title: item.title,
        detail: item.status === 'error' ? item.text || 'Failed' : item.status === 'skipped' ? 'Skipped' : item.status === 'cancelled' ? 'Stopped' : 'Routine',
        danger: item.status === 'error',
        open: item.loop_id ? { routine: item.loop_id } : undefined,
      }
    case 'commitment':
      return {
        ...base,
        title: item.title,
        detail: item.status === 'done' ? `Delivered to ${item.for ?? 'you'}` : `Dropped · ${item.text}`,
        open: item.thread_id ? { thread: item.thread_id } : undefined,
      }
    case 'message_sent':
    case 'message_received':
      return {
        ...base,
        title: messageTitle(item),
        detail: item.text,
        open: item.text ? { message: item } : undefined,
      }
    case 'file_changed':
    case 'file_edited': {
      const label = BOT_FILE_LABELS[item.title as BotFileName]
      return {
        ...base,
        title: item.kind === 'file_edited' ? `You edited ${label}` : item.title === 'MEMORY.md' ? 'Learned something new' : `Updated ${label}`,
        detail: item.kind === 'file_changed' && item.title === 'MEMORY.md' ? label : undefined,
        open: { file: item.title as BotFileName },
      }
    }
  }
}

function messageTitle(item: BotActivity): string {
  return item.kind === 'message_sent' ? `Messaged ${item.title}` : `Message from ${item.title}`
}

function groupByDay(rows: Row[], now: number): [string, Row[]][] {
  const today = new Date(now).toDateString()
  const days = new Map<string, Row[]>()
  for (const item of rows) {
    const day = new Date(item.at).toDateString() === today ? 'Today' : shortDate(item.at)
    days.set(day, [...(days.get(day) ?? []), item])
  }
  return [...days]
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-0.5">
      <h3 className="mb-1 text-[13px] font-semibold text-ink">{title}</h3>
      {children}
    </section>
  )
}

function ActivityRow({ item, now, onOpen }: { item: Row; now: number; onOpen: (target: Row['open']) => void }) {
  const since = item.running && item.at ? now - Date.parse(item.at) : undefined
  const content = (
    <>
      <span className="block truncate text-[13px] font-medium text-ink">{item.title}</span>
      {item.detail ? <span className={`line-clamp-2 text-[12px] ${item.danger ? 'text-danger' : 'text-ink-2'}`}>{item.detail}</span> : null}
      <span className={`block text-[12px] tabular-nums ${item.running ? 'text-running' : 'text-ink-3'}`}>
        {item.running ? (since !== undefined ? `Running for ${formatDuration(since)}` : 'Running') : item.since ? `Since ${messageTime(item.at, now)}` : clockTime(item.at)}
      </span>
    </>
  )
  const className = '-mx-2 flex min-w-0 flex-col rounded-lg px-2 py-1.5 text-left transition-colors duration-150'
  if (item.open && 'thread' in item.open) {
    return (
      <Link to="/sessions/$sessionId" params={{ sessionId: item.open.thread }} className={`${className} hover:bg-list-hover`}>
        {content}
      </Link>
    )
  }
  return item.open ? (
    <button type="button" onClick={() => onOpen(item.open)} className={`${className} cursor-pointer hover:bg-list-hover`}>
      {content}
    </button>
  ) : (
    <div className={className}>{content}</div>
  )
}

// A clock that ticks every half minute, for running times.
function useNow(): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  return now
}
