import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CornerDownRight } from 'lucide-react'
import { useEffect, useState, type MouseEvent } from 'react'
import { Modal } from '@/components/ui/Modal'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotOwed, BotOwedThread, BotThreadActivity } from '@/lib/api/types'
import { type ConnectedTaskThread, TASK_GROUPS, connectedTaskThreads, taskName, taskState } from '@/lib/bots'
import { clockTime, formatDuration, isToday, shortDate } from '@/lib/format/time'

const EMPTY_OWED: BotOwed[] = []
const EMPTY_ITEMS: BotActivity[] = []
const EMPTY_STARTED: Record<string, BotThreadActivity[]> = {}

// What a bot is working on and what it did: its tasks grouped by state, open
// and closed alike, then by day the threads it started. A task opens a modal
// with what it is, its next step, and all connected threads; a thread line
// opens its thread.
export function BotActivity({ bot }: { bot: Bot }) {
  const activity = useQuery(botActivityQuery(bot.id))
  const [shown, setShown] = useState<number>()
  const now = useNow()
  const owed = activity.data?.owed ?? EMPTY_OWED
  const items = activity.data?.items ?? EMPTY_ITEMS
  const started = activity.data?.started ?? EMPTY_STARTED
  if (activity.isPending) {
    return <p className="text-[13px] text-ink-3">Loading activity…</p>
  }
  if (activity.isError && !activity.data) {
    return <p role="alert" className="text-[13px] text-danger">Couldn’t load activity.</p>
  }
  const tasks: TaskRow[] = [...owed.map((task) => ({ task })), ...items.filter((item) => item.kind === 'task')]
  const days = groupByDay(items.filter((item) => item.kind !== 'task'), now)
  const task = tasks.find((row) => row.task.id === shown)?.task
  if (!tasks.length && !days.length) {
    return <p className="text-[13px] text-ink-3">No tasks yet.</p>
  }
  return (
    <div className="flex flex-col gap-5">
      {TASK_GROUPS.map((group) => {
        const list = tasks.filter((row) => taskState(row.task)[1] === group)
        return list.length ? (
          <section key={group} aria-label={group} className="flex flex-col">
            <h3 className={`${HEADING} ${group === 'Waiting for you' ? 'text-accent-strong' : 'text-ink-3'}`}>
              {group} <span className="ml-0.5 font-normal tabular-nums opacity-70">{list.length}</span>
            </h3>
            <ul className="flex flex-col">
              {list.map((row) => (
                <li key={row.task.id} className="flex flex-col">
                  <TaskButton task={row.task} at={row.at} now={now} onOpen={setShown} />
                </li>
              ))}
            </ul>
          </section>
        ) : null
      })}
      {days.map(([day, list]) => (
        <section key={day} aria-label={day} className="flex flex-col">
          <h3 className={`${HEADING} text-ink-3`}>{day}</h3>
          <ul className="flex flex-col">
            {list.map((item) => <li key={item.thread_id}><ThreadLink line={activityLine(item, now)} /></li>)}
          </ul>
        </section>
      ))}
      {task ? (
        <TaskModal
          botId={bot.id}
          task={task}
          threads={connectedTaskThreads(task, started)}
          now={now}
          onClose={() => setShown(undefined)}
        />
      ) : null}
    </div>
  )
}

const HEADING = 'mb-1 text-[12px] font-medium'
const ROW = '-mx-2 flex min-w-0 rounded-lg px-2 text-left text-[13px] transition-colors duration-150 hover:bg-list-hover'

// A task and, once it has closed, when.
type TaskRow = { task: BotOwed; at?: string }

// A task's whole name, wrapping as it needs; under it only a state that says
// more than its group does, and beside it when a closed task closed.
function TaskButton({ task, at, now, onOpen }: TaskRow & { now: number; onOpen: (id: number) => void }) {
  const [label, group, detail] = taskState(task)
  return (
    <button type="button" onClick={() => onOpen(task.id)} className={`${ROW} cursor-pointer items-baseline gap-3 py-[7px] leading-[18px] active:bg-list-active`}>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className={`text-pretty wrap-break-word ${group === 'Done' || group === 'Stopped' ? 'text-ink-2' : 'text-ink'}`}>{taskName(task)}</span>
        {detail ? <span className="text-[12px] text-ink-3">{label}</span> : null}
      </span>
      {at ? <span className="shrink-0 text-[12px] text-ink-3 tabular-nums">{isToday(at, now) ? clockTime(at) : shortDate(at)}</span> : null}
    </button>
  )
}

// A task's name, what was asked, its next step or reason, all connected
// threads and, when it waits on you, its question in the chat.
function TaskModal({
  botId,
  task,
  threads,
  now,
  onClose,
}: {
  botId: string
  task: BotOwed
  threads: ConnectedTaskThread[]
  now: number
  onClose: () => void
}) {
  const name = taskName(task)
  const request = task.request.trim()
  const next = task.next?.trim()
  const nextLabel = task.status === 'stopped' || task.status === 'cancelled' ? 'Reason' : 'Next step'
  // The chat jumps to a message once per link, so a repeated click scrolls itself.
  const toMessage = (event: MouseEvent<HTMLAnchorElement>) => {
    onClose()
    document.getElementById(event.currentTarget.hash.slice(1))?.scrollIntoView({ block: 'center' })
  }
  return (
    <Modal open onClose={onClose} title={name} description={taskState(task)[0]} size="sm">
      <div className="flex flex-col gap-4 text-[13px]">
        {request && request !== name ? <p className="whitespace-pre-wrap wrap-break-word text-ink-2">{task.request}</p> : null}
        {next ? (
          <section aria-label={nextLabel} className="flex flex-col gap-1">
            <h3 className="text-[12px] font-medium text-ink-3">{nextLabel}</h3>
            <p className="whitespace-pre-wrap wrap-break-word text-ink-2">{task.next}</p>
          </section>
        ) : null}
        {threads.length ? (
          <section aria-label="Threads" className="flex flex-col">
            <h3 className="mb-1 flex items-center justify-between text-[12px] font-medium text-ink-3">
              <span>Threads</span>
              <span className="tabular-nums">{threads.length}</span>
            </h3>
            <ul className="flex flex-col">
              {threads.map((entry) => (
                <li key={entry.id}>
                  <ThreadLink
                    line={connectedThreadLine(entry, now)}
                    depth={entry.depth}
                    onClick={onClose}
                  />
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        {task.status === 'waiting_for_user' && task.question_seq ? (
          <Link to="/bots/$botId" params={{ botId }} search={{ message: task.question_seq }} hash={`message-${task.question_seq}`} onClick={toMessage} className="self-start font-medium text-ink-2 hover:text-ink">Answer in chat</Link>
        ) : null}
      </div>
    </Modal>
  )
}

// A thread in one line: its name and how far it got, with the reason a
// failed run gave.
type ThreadLine = { id: string; title: string; state: string; tone?: Tone; hint?: string }

const ENDS: Partial<Record<string, string>> = { error: 'Failed', skipped: 'Skipped', cancelled: 'Stopped' }

function activityLine(item: BotThreadActivity, now: number): ThreadLine {
  const line = { id: item.thread_id, title: item.title || 'Side chat', hint: item.text }
  if (item.running) return { ...line, state: `Running for ${formatDuration(now - Date.parse(item.at))}`, tone: 'running' }
  return { ...line, state: ENDS[item.status ?? ''] ?? clockTime(item.at), tone: item.status === 'error' ? 'danger' : undefined }
}

const THREAD_STATES: Record<BotOwedThread['state'], [string, Tone?]> = {
  working: ['Working', 'running'],
  background: ['Working', 'running'],
  finished: ['Finished'],
  failed: ['Failed', 'danger'],
  idle: ['Not started'],
  gone: ['Archived'],
}

function taskThreadLine(thread: BotOwedThread): ThreadLine {
  const [state, tone] = THREAD_STATES[thread.state] ?? [thread.state]
  return { id: thread.id, title: thread.title || thread.id, state, tone }
}

function connectedThreadLine(entry: ConnectedTaskThread, now: number): ThreadLine {
  if (entry.thread) return taskThreadLine(entry.thread)
  if (entry.activity) return activityLine(entry.activity, now)
  return { id: entry.id, title: entry.id, state: '' }
}

function ThreadLink({ line, depth = 0, onClick }: { line: ThreadLine; depth?: number; onClick?: () => void }) {
  const indent = depth > 0 ? Math.min(depth, 4) * 14 : 0
  return (
    <Link
      to="/sessions/$sessionId"
      params={{ sessionId: line.id }}
      title={line.hint || line.title}
      onClick={onClick}
      style={indent ? { paddingLeft: `${8 + indent}px` } : undefined}
      className={`${ROW} h-8 items-center gap-1.5`}
    >
      {depth > 0 ? <CornerDownRight size={12} aria-hidden className="shrink-0 text-ink-3" /> : null}
      <span className="min-w-0 flex-1 truncate text-ink">{line.title}</span>
      {line.state ? <span className={`shrink-0 text-[12px] tabular-nums ${line.tone ? TONES[line.tone] : 'text-ink-3'}`}>{line.state}</span> : null}
    </Link>
  )
}

const TONES = { running: 'text-running', danger: 'text-danger' }
type Tone = keyof typeof TONES

function groupByDay(items: BotThreadActivity[], now: number): [string, BotThreadActivity[]][] {
  const days = new Map<string, BotThreadActivity[]>()
  for (const item of items) {
    const day = isToday(item.at, now) ? 'Today' : shortDate(item.at)
    days.set(day, [...(days.get(day) ?? []), item])
  }
  return [...days]
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
