import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, useState, type MouseEvent } from 'react'
import { Modal } from '@/components/ui/Modal'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotOwed, BotOwedThread, BotThreadActivity } from '@/lib/api/types'
import { taskName, taskStateLabel } from '@/lib/bots'
import { clockTime, formatDuration, shortDate } from '@/lib/format/time'

// What a bot is working on and what it did: its open tasks, then by day the
// tasks it closed and the threads it started, one line each. A task opens a
// modal with what it is and its threads; a thread line opens its thread.
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
  const { owed, items } = activity.data
  const groups = [
    { title: '', tasks: owed.filter((task) => task.status !== 'waiting_for_user') },
    { title: 'Waiting for you', tasks: owed.filter((task) => task.status === 'waiting_for_user') },
  ]
  const days = groupByDay(items, now)
  const task = [...owed, ...items.flatMap((item) => item.kind === 'task' ? [item.task] : [])].find((item) => item.id === shown)
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
                {tasks.map((item) => (
                  <li key={item.id} className="flex flex-col"><TaskButton task={item} stated={!title} onOpen={setShown} /></li>
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
            {list.map((item) => item.kind === 'task'
              ? <li key={`task-${item.task.id}`} className="flex flex-col"><TaskButton task={item.task} stated onOpen={setShown} /></li>
              : <li key={item.thread_id}><ThreadLink line={activityLine(item, now)} /></li>)}
          </ul>
        </section>
      ))}
      {task ? <TaskModal botId={bot.id} task={task} onClose={() => setShown(undefined)} /> : null}
    </div>
  )
}

const LINE = '-mx-2 flex h-8 min-w-0 items-center gap-1.5 rounded-lg px-2 text-left text-[13px] transition-colors duration-150 hover:bg-list-hover'

function TaskButton({ task, stated, onOpen }: { task: BotOwed; stated: boolean; onOpen: (id: number) => void }) {
  return (
    <button type="button" onClick={() => onOpen(task.id)} className={`${LINE} cursor-pointer`}>
      <Line title={taskName(task)} state={stated ? taskStateLabel(task) : undefined} tone={task.moving ? 'running' : undefined} />
    </button>
  )
}

// A task's name, what was asked, its threads and, when it waits on you, its
// question in the chat.
function TaskModal({ botId, task, onClose }: { botId: string; task: BotOwed; onClose: () => void }) {
  const name = taskName(task)
  // The chat jumps to a message once per link, so a repeated click scrolls itself.
  const toMessage = (event: MouseEvent<HTMLAnchorElement>) => {
    onClose()
    document.getElementById(event.currentTarget.hash.slice(1))?.scrollIntoView({ block: 'center' })
  }
  return (
    <Modal open onClose={onClose} title={name} description={taskStateLabel(task)} size="sm">
      <div className="flex flex-col gap-4 text-[13px]">
        {task.request !== name ? <p className="whitespace-pre-wrap wrap-break-word text-ink-2">{task.request}</p> : null}
        {task.threads?.length ? (
          <section aria-label="Threads" className="flex flex-col">
            <h3 className="mb-1 text-[12px] font-medium text-ink-3">Threads</h3>
            <ul className="flex flex-col">
              {task.threads.map((thread) => <li key={thread.id}><ThreadLink line={taskThreadLine(thread)} /></li>)}
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
  const [state, tone] = THREAD_STATES[thread.state]
  return { id: thread.id, title: thread.title, state, tone }
}

function ThreadLink({ line }: { line: ThreadLine }) {
  return (
    <Link to="/sessions/$sessionId" params={{ sessionId: line.id }} title={line.hint} className={LINE}>
      <Line title={line.title} state={line.state} tone={line.tone} />
    </Link>
  )
}

const TONES = { running: 'text-running', danger: 'text-danger' }
type Tone = keyof typeof TONES

function Line({ title, state, tone }: { title: string; state?: string; tone?: Tone }) {
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
