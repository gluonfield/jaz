import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CornerDownRight } from 'lucide-react'
import { useEffect, useState, type MouseEvent } from 'react'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotOwed } from '@/lib/api/types'
import { taskName, taskStateLabel } from '@/lib/bots'
import { clockTime, formatDuration, shortDate } from '@/lib/format/time'

// What a bot is working on and what it did: its tasks, then the threads it
// started by day. Every row is one line that opens where the work is.
export function BotActivity({ bot }: { bot: Bot }) {
  const activity = useQuery(botActivityQuery(bot.id))
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
                {tasks.map((task) => <TaskRow key={task.id} botId={bot.id} task={task} />)}
              </ul>
            </div>
          ) : null)}
        </section>
      ) : null}
      {days.map(([day, list]) => (
        <section key={day} aria-label={day} className="flex flex-col">
          <h3 className="mb-1 text-[13px] font-semibold text-ink">{day}</h3>
          <ul className="flex flex-col">
            {list.map((item) => <ThreadRow key={item.thread_id} item={item} started={started} now={now} />)}
          </ul>
        </section>
      ))}
    </div>
  )
}

const LINE = '-mx-2 flex h-8 min-w-0 items-center gap-1.5 rounded-lg px-2 text-[13px] transition-colors duration-150 hover:bg-list-hover'

// A task opens its question when it waits on you, else the thread working
// on it, else the message that asked for it. Tasks waiting on you sit under
// their own heading, so they carry no state.
function TaskRow({ botId, task }: { botId: string; task: BotOwed }) {
  const thread = task.threads?.find((item) => item.state === 'working' || item.state === 'background') ?? task.threads?.at(-1)
  const message = task.status === 'waiting_for_user' && task.question_seq ? task.question_seq : thread ? undefined : task.opening_seq
  const line = <Line title={taskName(task)} state={task.status === 'waiting_for_user' ? undefined : taskStateLabel(task)} tone={task.moving ? 'running' : undefined} />
  // The chat jumps to a message once per link, so a repeated click scrolls itself.
  const toMessage = (event: MouseEvent<HTMLAnchorElement>) => document.getElementById(event.currentTarget.hash.slice(1))?.scrollIntoView({ block: 'center' })
  return (
    <li>
      {message ? (
        <Link to="/bots/$botId" params={{ botId }} search={{ message }} hash={`message-${message}`} onClick={toMessage} className={LINE}>{line}</Link>
      ) : thread ? (
        <Link to="/sessions/$sessionId" params={{ sessionId: thread.id }} className={LINE}>{line}</Link>
      ) : (
        <div className={LINE}>{line}</div>
      )}
    </li>
  )
}

// Started holds the threads each of a bot's threads started, by its id.
type Started = Record<string, BotActivity[]>

const ENDS: Partial<Record<string, string>> = { error: 'Failed', skipped: 'Skipped', cancelled: 'Stopped' }

// A thread the bot started, with the threads it started beneath it.
function ThreadRow({ item, started, now, depth = 0 }: { item: BotActivity; started: Started; now: number; depth?: number }) {
  const state = item.running ? `Running for ${formatDuration(now - Date.parse(item.at))}` : ENDS[item.status ?? ''] ?? clockTime(item.at)
  return (
    <li>
      <Link to="/sessions/$sessionId" params={{ sessionId: item.thread_id }} title={item.text} className={LINE}>
        {depth ? <CornerDownRight size={12} className="shrink-0 text-ink-3" /> : null}
        <Line title={item.title || 'Side chat'} state={state} tone={item.running ? 'running' : item.status === 'error' ? 'danger' : undefined} />
      </Link>
      {started[item.thread_id]?.length ? (
        <ul className={`flex flex-col ${depth ? 'ml-[18px]' : ''}`}>
          {started[item.thread_id].map((child) => <ThreadRow key={child.thread_id} item={child} started={started} now={now} depth={depth + 1} />)}
        </ul>
      ) : null}
    </li>
  )
}

const TONES = { running: 'text-running', danger: 'text-danger' }

function Line({ title, state, tone }: { title: string; state?: string; tone?: keyof typeof TONES }) {
  return (
    <>
      <span className="min-w-0 flex-1 truncate text-ink">{title}</span>
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
