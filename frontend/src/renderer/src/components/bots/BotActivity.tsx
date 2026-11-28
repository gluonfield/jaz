import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronRight, CornerDownRight } from 'lucide-react'
import { useEffect, useState, type MouseEvent, type ReactNode } from 'react'
import { Modal } from '@/components/ui/Modal'
import { botActivityQuery } from '@/lib/api/bots'
import type { Bot, BotActivity, BotOwed, BotOwedThread, BotThreadActivity } from '@/lib/api/types'
import { type ConnectedTaskThread, TASK_GROUPS, connectedTaskThreads, taskBrief, taskName, taskState } from '@/lib/bots'
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
            <Heading count={list.length} accent={group === 'Waiting for you'}>{group}</Heading>
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
          <Heading>{day}</Heading>
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

function Heading({ count, accent, children }: { count?: number; accent?: boolean; children: string }) {
  return (
    <h3 className={`mb-1 text-[12px] font-medium ${accent ? 'text-accent-strong' : 'text-ink-3'}`}>
      {children}
      {count ? <span className="ml-0.5 font-normal tabular-nums opacity-70"> {count}</span> : null}
    </h3>
  )
}

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

// A task at a glance: its name and state, its latest note or what was asked
// in a couple of lines, its question in the chat when it waits on you, and
// its threads to open. Text that says more stays behind Full details.
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
  const [full, setFull] = useState(false)
  const name = taskName(task)
  const [label, group] = taskState(task)
  const request = task.request.trim()
  const note = task.next?.trim() ?? ''
  const brief = taskBrief(note || request)
  const noteLabel = task.status === 'stopped' || task.status === 'cancelled' ? 'Reason' : 'Next step'
  const details = [['Request', request], [noteLabel, note]].filter(([, text]) => text && text !== brief && text !== name)
  // The chat jumps to a message once per link, so a repeated click scrolls itself.
  const toMessage = (event: MouseEvent<HTMLAnchorElement>) => {
    onClose()
    document.getElementById(event.currentTarget.hash.slice(1))?.scrollIntoView({ block: 'center' })
  }
  return (
    <Modal open onClose={onClose} title={name} size="sm" chromeless>
      <div className="flex flex-col gap-4 p-5 text-[13px] leading-[19px]">
        <header className="flex flex-col gap-1 pr-7">
          <h2 className="text-[16px] font-semibold leading-[22px] text-balance wrap-break-word text-ink">{name}</h2>
          <p className={`text-[12px] ${group === 'Waiting for you' ? 'font-medium text-accent-strong' : 'text-ink-3'}`}>{label}</p>
        </header>
        {brief && brief !== name ? <p className="text-pretty wrap-break-word text-ink-2">{brief}</p> : null}
        {task.status === 'waiting_for_user' && task.question_seq ? (
          <Link to="/bots/$botId" params={{ botId }} search={{ message: task.question_seq }} hash={`message-${task.question_seq}`} onClick={toMessage} className="inline-flex h-8 items-center self-start rounded-full bg-primary px-3 font-medium text-on-primary transition-colors duration-150 hover:bg-primary-strong">Answer in chat</Link>
        ) : null}
        {threads.length ? (
          <section aria-label="Threads" className="flex flex-col">
            <Heading count={threads.length}>Threads</Heading>
            <ul className="flex flex-col">
              {threads.map((entry) => (
                <li key={entry.id}>
                  <ThreadLink line={connectedThreadLine(entry, now)} depth={entry.depth} onClick={onClose}>
                    <ChevronRight size={14} aria-hidden className="-mr-1 shrink-0 text-ink-3" />
                  </ThreadLink>
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        {details.length ? (
          <section className="flex flex-col gap-3">
            <button type="button" aria-expanded={full} onClick={() => setFull(!full)} className="flex cursor-pointer items-center gap-1 self-start text-[12px] text-ink-3 transition-colors duration-150 hover:text-ink-2">
              <ChevronRight size={12} aria-hidden className={`transition-transform duration-150 ${full ? 'rotate-90' : ''}`} />
              Full details
            </button>
            {full ? details.map(([heading, text]) => (
              <section key={heading} aria-label={heading} className="flex flex-col">
                <Heading>{heading}</Heading>
                <p className="whitespace-pre-wrap wrap-break-word text-ink-2">{text}</p>
              </section>
            )) : null}
          </section>
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

function ThreadLink({ line, depth = 0, onClick, children }: { line: ThreadLine; depth?: number; onClick?: () => void; children?: ReactNode }) {
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
      {children}
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
