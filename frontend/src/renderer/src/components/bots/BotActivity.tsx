import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CornerDownRight } from 'lucide-react'
import { useEffect, useState, type MouseEvent } from 'react'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotOwed, BotOwedThread } from '@/lib/api/types'
import { taskName, taskStateLabel } from '@/lib/bots'
import { clockTime, formatDuration, shortDate } from '@/lib/format/time'

// What a bot is working on and what it did: its tasks, then the threads it
// started by day, one line each. A task opens in place to say what it is and
// list its threads; every thread line opens its thread.
export function BotActivity({ bot }: { bot: Bot }) {
  const activity = useQuery(botActivityQuery(bot.id))
  const [shown, setShown] = useState<number>()
  const now = useNow()
  if (activity.isPending) {
    return <p className="text-[13px] text-ink-3">Loading activity…</p>
  }
  if (activity.isError && !activity.data) {
    return <p role="alert" className="text-[13px] text-danger">Couldn’t load activity.</p>
  }
  const { owed, started, items } = activity.data
  const groups = [
    { title: '', tasks: owed.filter((task) => task.status !== 'waiting_for_user') },
    { title: 'Waiting for you', tasks: owed.filter((task) => task.status === 'waiting_for_user') },
  ]
  const days = groupByDay(items, now)
  if (!owed.length && !days.length) {
    return <p className="text-[13px] text-ink-3">No tasks yet.</p>
  }
  return (
    <div className="flex flex-col gap-5">
      {owed.length ? (
        <section aria-label="Tasks" className="flex flex-col">
          <h3 className="mb-1 text-[13px] font-semibold text-ink">Tasks <span className="ml-1 text-ink-3 tabular-nums">{owed.length}</span></h3>
          {groups.map(({ title, tasks }) => tasks.length ? (
            <div key={title} className="flex flex-col">
              {title ? <h4 className="mt-3 mb-1 text-[11px] font-medium text-ink-3">{title} <span className="ml-1 tabular-nums">{tasks.length}</span></h4> : null}
              <ul className="flex flex-col">
                {tasks.map((task) => (
                  <TaskRow key={task.id} botId={bot.id} task={task} open={task.id === shown} onToggle={() => setShown(task.id === shown ? undefined : task.id)} started={started} now={now} />
                ))}
              </ul>
            </div>
          ) : null)}
        </section>
      ) : null}
      {days.map(([day, list]) => (
        <section key={day} aria-label={day} className="flex flex-col">
          <h3 className="mb-1 text-[13px] font-semibold text-ink">{day}</h3>
          <ul className="flex flex-col">
            {list.map((item) => <ThreadRow key={item.thread_id} line={activityLine(item, now)} started={started} now={now} />)}
          </ul>
        </section>
      ))}
    </div>
  )
}

const LINE = '-mx-2 flex min-h-8 min-w-0 items-center gap-1.5 rounded-lg px-2 py-1 text-left text-[13px] transition-colors duration-150 hover:bg-list-hover'

// Started holds the threads each of a bot's threads started, by its id.
type Started = Record<string, BotActivity[]>

// A task in one line, opening in place to its full name, what was asked, its
// threads and, when it waits on you, its question in the chat. Tasks waiting
// on you sit under their own heading, so they carry no state.
function TaskRow({ botId, task, open, onToggle, started, now }: { botId: string; task: BotOwed; open: boolean; onToggle: () => void; started: Started; now: number }) {
  const name = taskName(task)
  const waiting = task.status === 'waiting_for_user'
  // The chat jumps to a message once per link, so a repeated click scrolls itself.
  const toMessage = (event: MouseEvent<HTMLAnchorElement>) => document.getElementById(event.currentTarget.hash.slice(1))?.scrollIntoView({ block: 'center' })
  return (
    <li className="flex flex-col">
      <button type="button" aria-expanded={open} onClick={onToggle} className={`${LINE} cursor-pointer`}>
        <Line title={name} state={waiting ? undefined : taskStateLabel(task)} tone={task.moving ? 'running' : undefined} wrap={open} />
      </button>
      {open ? (
        <div className="mb-2 flex min-w-0 flex-col gap-1 border-l border-border pl-3">
          {task.request !== name ? <p className="line-clamp-2 text-[12px] leading-5 wrap-break-word text-ink-2">{task.request}</p> : null}
          {task.threads?.length ? (
            <ul aria-label="Threads" className="flex flex-col">
              {task.threads.map((thread) => <ThreadRow key={thread.id} line={taskThreadLine(thread)} started={started} now={now} />)}
            </ul>
          ) : null}
          {waiting && task.question_seq ? (
            <Link to="/bots/$botId" params={{ botId }} search={{ message: task.question_seq }} hash={`message-${task.question_seq}`} onClick={toMessage} className="self-start text-[12px] font-medium text-ink-2 hover:text-ink">Answer in chat</Link>
          ) : null}
        </div>
      ) : null}
    </li>
  )
}

// A thread in one line: its name and how far it got, with the reason a
// failed run gave.
type ThreadLine = { id: string; title: string; state: string; tone?: Tone; hint?: string }

const ENDS: Partial<Record<string, string>> = { error: 'Failed', skipped: 'Skipped', cancelled: 'Stopped' }

function activityLine(item: BotActivity, now: number): ThreadLine {
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
  const [state, tone] = THREAD_STATES[thread.state]
  return { id: thread.id, title: thread.title, state, tone }
}

// A thread opening itself, with the threads it started beneath it.
function ThreadRow({ line, started, now, depth = 0 }: { line: ThreadLine; started: Started; now: number; depth?: number }) {
  const children = started[line.id]
  return (
    <li>
      <Link to="/sessions/$sessionId" params={{ sessionId: line.id }} title={line.hint} className={LINE}>
        {depth ? <CornerDownRight size={12} className="shrink-0 text-ink-3" /> : null}
        <Line title={line.title} state={line.state} tone={line.tone} />
      </Link>
      {children?.length ? (
        <ul className={`flex flex-col ${depth ? 'ml-[18px]' : ''}`}>
          {children.map((child) => <ThreadRow key={child.thread_id} line={activityLine(child, now)} started={started} now={now} depth={depth + 1} />)}
        </ul>
      ) : null}
    </li>
  )
}

const TONES = { running: 'text-running', danger: 'text-danger' }
type Tone = keyof typeof TONES

// A title and its state at the end; a wrapped title shows in full.
function Line({ title, state, tone, wrap }: { title: string; state?: string; tone?: Tone; wrap?: boolean }) {
  return (
    <>
      <span className={`min-w-0 flex-1 text-ink ${wrap ? 'wrap-break-word' : 'truncate'}`}>{title}</span>
      {state ? <span className={`shrink-0 text-[12px] tabular-nums ${tone ? TONES[tone] : 'text-ink-3'}`}>{state}</span> : null}
    </>
  )
}

function groupByDay(items: BotActivity[], now: number): [string, BotActivity[]][] {
  const today = new Date(now).toDateString()
  const days = new Map<string, BotActivity[]>()
  for (const item of items) {
    const day = new Date(item.at).toDateString() === today ? 'Today' : shortDate(item.at)
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
