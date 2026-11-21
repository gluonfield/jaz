import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CornerDownRight } from 'lucide-react'
import { useEffect, useState, type MouseEvent, type ReactNode } from 'react'
import { RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotFileName, BotOwed, BotOwedThread, BotStartedThread } from '@/lib/api/types'
import { taskName, taskStateLabel } from '@/lib/bots'
import { clockTime, formatDuration, messageTime, shortDate } from '@/lib/format/time'

// One line of work: what it is and how far it got, opening a thread, a
// routine or the bot's memory.
type Row = {
  key: string
  at: string
  title: string
  running?: boolean
  // How the work ended, said in place of when, and a failed run's error.
  ended?: string
  failed?: boolean
  error?: string
  open?: { thread: string } | { routine: string } | { file: BotFileName }
}

export function BotActivity({
  bot,
  onOpenRoutine,
  onOpenFile,
}: {
  bot: Bot
  onOpenRoutine: (id: string) => void
  onOpenFile: (name: BotFileName) => void
}) {
  const activity = useQuery(botActivityQuery(bot.id))
  const [shown, setShown] = useState<number | null>(null)
  const now = useNow()
  if (activity.isPending) {
    return <p className="text-[13px] text-ink-3">Loading activity…</p>
  }
  if (activity.isError && !activity.data) {
    return <p role="alert" className="text-[13px] text-danger">Couldn’t load activity.</p>
  }
  const { owed, started, items } = activity.data
  const taskGroups = [
    { title: '', tasks: owed.filter((task) => task.status !== 'waiting_for_user') },
    { title: 'Waiting for you', tasks: owed.filter((task) => task.status === 'waiting_for_user') },
  ]
  const days = groupByDay(items.map(row), now)
  const open = (target: Row['open']) => {
    if (!target) return
    if ('routine' in target) onOpenRoutine(target.routine)
    else if ('file' in target) onOpenFile(target.file)
  }
  if (!owed.length && !days.length) {
    return <p className="text-[13px] text-ink-3">No tasks yet.</p>
  }
  return (
    <div className="flex flex-col gap-5">
      {owed.length ? <section aria-label="Tasks" className="flex flex-col gap-0.5">
        <h3 className="mb-1 text-[13px] font-semibold text-ink">Tasks <span className="ml-1 text-ink-3 tabular-nums">{owed.length}</span></h3>
        {taskGroups.map(({ title, tasks }) => tasks.length ? (
          <div key={title} className="flex min-w-0 flex-col gap-0.5">
            {title ? <h4 className="mt-3 mb-1 text-[11px] font-medium text-ink-3">{title} <span className="ml-1 tabular-nums">{tasks.length}</span></h4> : null}
            {tasks.map((task) => (
              <div key={task.id} className="flex min-w-0 flex-col">
                <button type="button" aria-expanded={task.id === shown} onClick={() => setShown(task.id === shown ? null : task.id)} className="-mx-2 flex min-h-10 min-w-0 cursor-pointer flex-col justify-center rounded-lg px-2 py-1.5 text-left transition-colors duration-150 hover:bg-list-hover">
                  <span className="line-clamp-2 w-full text-[13px] leading-5 font-medium wrap-break-word text-ink">{taskName(task)}</span>
                  {!title ? <span className="text-[11px] text-ink-3">{taskStateLabel(task)}</span> : null}
                </button>
                {task.id === shown ? <TaskDetails botId={bot.id} task={task} started={started} now={now} /> : null}
              </div>
            ))}
          </div>
        ) : null)}
      </section> : null}
      {days.map(([day, list]) => (
        <section key={day} className="flex flex-col gap-0.5">
          <h3 className="mb-1 text-[13px] font-semibold text-ink">{day}</h3>
          {list.map((item) => (
            <ActivityRow key={item.key} item={item} started={started} now={now} onOpen={open} />
          ))}
        </section>
      ))}
    </div>
  )
}

const THREAD_STATES: Record<BotOwedThread['state'], string> = { working: 'Working', background: 'Working in the background', finished: 'Finished', failed: 'Failed', idle: 'Not started', gone: 'Archived' }

// What a task needs and has produced, opened in place under it: its work
// threads, the question it waits on, its last note and where it came from.
function TaskDetails({ botId, task, started, now }: { botId: string; task: BotOwed; started: Started; now: number }) {
  const toMessage = (event: MouseEvent<HTMLAnchorElement>) => document.getElementById(event.currentTarget.hash.slice(1))?.scrollIntoView({ block: 'center' })
  const chatLink = (seq: number, label: string) => (
    <Link to="/bots/$botId" params={{ botId }} search={{ message: seq }} hash={`message-${seq}`} onClick={toMessage} className="font-medium text-ink-2 hover:text-ink">{label}</Link>
  )
  return (
    <div className="mb-2 ml-0.5 flex min-w-0 flex-col gap-2 border-l border-border pl-3 text-[13px] leading-relaxed">
      {task.threads?.length ? (
        <ul aria-label="Work" className="flex flex-col">
          {task.threads.map((thread) => (
            <li key={thread.id}>
              <Link to="/sessions/$sessionId" params={{ sessionId: thread.id }} className={LINE}>
                <Line title={thread.title} state={THREAD_STATES[thread.state]} tone={thread.state === 'failed' ? 'danger' : thread.state === 'working' || thread.state === 'background' ? 'running' : undefined} />
              </Link>
              <StartedThreads started={started} by={thread.id} now={now} />
            </li>
          ))}
        </ul>
      ) : null}
      {task.question ? (
        <section aria-label="Question for you" className="flex flex-col gap-1">
          <RenderedMarkdown text={task.question} />
          {task.question_seq ? chatLink(task.question_seq, 'Answer in chat') : null}
        </section>
      ) : null}
      {task.next ? <p className="line-clamp-3 whitespace-pre-wrap wrap-break-word text-ink-2">{task.next}</p> : null}
      <p className="flex flex-wrap gap-x-3 text-[12px] text-ink-3">
        <span>{[`For ${task.for ?? 'you'}`, `since ${messageTime(task.opened_at)}`, task.blocked && task.depends_on?.length ? `after ${task.depends_on.map((id) => `#${id}`).join(', ')}` : ''].filter(Boolean).join(' · ')}</span>
        {task.opening_seq ? chatLink(task.opening_seq, 'Opening message') : null}
      </p>
      <details className="text-[12px] text-ink-3">
        <summary className="cursor-pointer font-medium hover:text-ink">Request</summary>
        <p className="mt-1 whitespace-pre-wrap wrap-break-word text-ink-2">{task.request}</p>
      </details>
    </div>
  )
}

const ROUTINE_ENDS: Partial<Record<string, string>> = { error: 'Failed', skipped: 'Skipped', cancelled: 'Stopped' }

function row(item: BotActivity, index: number): Row {
  const base = { key: `${item.kind}:${item.thread_id ?? item.loop_id ?? ''}:${item.at}:${index}`, at: item.at, running: item.running }
  switch (item.kind) {
    case 'side_chat':
      return { ...base, title: item.title || 'Side chat', ended: item.status === 'error' ? 'Failed' : undefined, failed: item.status === 'error', open: item.thread_id ? { thread: item.thread_id } : undefined }
    case 'routine':
      return {
        ...base,
        title: item.title,
        ended: ROUTINE_ENDS[item.status ?? ''],
        failed: item.status === 'error',
        error: item.status === 'error' ? item.text : undefined,
        open: item.loop_id ? { routine: item.loop_id } : undefined,
      }
    case 'file_changed':
    case 'file_edited':
      return { ...base, title: item.kind === 'file_edited' ? 'You edited memory' : 'Updated memory', open: { file: item.title as BotFileName } }
  }
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

const LINE = '-mx-2 flex min-h-8 min-w-0 flex-col justify-center rounded-lg px-2 py-1 text-left text-[13px] transition-colors duration-150 hover:bg-list-hover'

// A piece of work in one line, its state at the end; a thread another thread
// started is marked as such.
function Line({ title, state, tone, error, started }: { title: string; state: string; tone?: 'running' | 'danger'; error?: string; started?: boolean }) {
  return (
    <>
      <span className="flex min-w-0 items-center gap-1.5">
        {started ? <CornerDownRight size={12} className="shrink-0 text-ink-3" /> : null}
        <span className={`min-w-0 flex-1 truncate ${started ? 'text-ink-2' : 'font-medium text-ink'}`}>{title}</span>
        <span className={`shrink-0 text-[12px] tabular-nums ${tone === 'running' ? 'text-running' : tone === 'danger' ? 'text-danger' : 'text-ink-3'}`}>{state}</span>
      </span>
      {error ? <span className="truncate text-[12px] text-danger">{error}</span> : null}
    </>
  )
}

function rowLine(item: Row, now: number, started?: boolean): ReactNode {
  const state = item.running ? `Running for ${formatDuration(now - Date.parse(item.at))}` : item.ended ?? clockTime(item.at)
  return <Line title={item.title} state={state} tone={item.running ? 'running' : item.failed ? 'danger' : undefined} error={item.error} started={started} />
}

function ActivityRow({ item, started, now, onOpen }: { item: Row; started: Started; now: number; onOpen: (target: Row['open']) => void }) {
  if (item.open && 'thread' in item.open) {
    return (
      <>
        <Link to="/sessions/$sessionId" params={{ sessionId: item.open.thread }} className={LINE}>
          {rowLine(item, now)}
        </Link>
        <StartedThreads started={started} by={item.open.thread} now={now} />
      </>
    )
  }
  return item.open ? (
    <button type="button" onClick={() => onOpen(item.open)} className={`${LINE} cursor-pointer`}>
      {rowLine(item, now)}
    </button>
  ) : (
    <div className={LINE}>{rowLine(item, now)}</div>
  )
}

// Started holds the threads each of a bot's threads started, by its id.
type Started = Record<string, BotStartedThread[]>

// StartedThreads lists the threads a thread started under it, and theirs
// under them, each opening its thread.
function StartedThreads({ started, by, now, nested }: { started: Started; by: string; now: number; nested?: boolean }) {
  const threads = started[by]
  if (!threads?.length) return null
  return (
    <ul className={`flex flex-col ${nested ? 'ml-[18px]' : ''}`}>
      {threads.map((thread, index) => (
        <li key={thread.thread_id}>
          <Link to="/sessions/$sessionId" params={{ sessionId: thread.thread_id }} className={LINE}>
            {rowLine(row(thread, index), now, true)}
          </Link>
          <StartedThreads started={started} by={thread.thread_id} now={now} nested />
        </li>
      ))}
    </ul>
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
