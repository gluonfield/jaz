import { ArrowDownLeft, ArrowLeftRight, ArrowUpRight, BookOpen, CalendarPlus, CircleAlert, Clock3, ListChecks, type LucideIcon, PenLine, Users } from 'lucide-react'
import { agentLabel } from '@/lib/agentLabel'
import type { BotActivityEvent, SessionEvent } from '@/lib/api/types'

const ACTIVITY: Record<Exclude<BotActivityEvent['kind'], 'group_progress'>, [LucideIcon, (label: string) => string]> = {
  routine: [Clock3, (label) => label],
  message_sent: [ArrowUpRight, (label) => `Messaged ${label}`],
  message_received: [ArrowDownLeft, (label) => `Message from ${label}`],
  group: [Users, (label) => `In ${label}`],
  unreachable: [CircleAlert, (label) => `Couldn't reach ${label}`],
  file_changed: [BookOpen, (label) => `Updated ${label}`],
  file_edited: [PenLine, (label) => `You edited ${label}`],
}

// What woke a bot, or what it set up, as one quiet centered line; with `onOpen`
// the line opens what it names.
export function SystemEventRow({ event, onOpen }: { event: SessionEvent; onOpen?: () => void }) {
  if (event.bot_activity?.kind === 'group_progress') {
    return (
      <div className="flex items-start gap-2 rounded-lg border border-border px-3 py-2 text-sm text-ink-2">
        <ListChecks size={14} aria-hidden className="mt-0.5 shrink-0" />
        <div className="min-w-0 break-words">
          <p className="text-xs font-medium text-ink-3">{event.bot_activity.label} · Progress update</p>
          <p className="whitespace-pre-wrap">{event.bot_activity.text}</p>
        </div>
      </div>
    )
  }
  const [Icon, text] = event.bot_activity
    ? ACTIVITY[event.bot_activity.kind]
    : event.type === 'agent_switch'
      ? [ArrowLeftRight, (label: string) => `Switched to ${agentLabel(label)}`]
      : [CalendarPlus, (label: string) => `Created routine · ${label}`]
  const line = (
    <>
      <Icon size={12} aria-hidden className="shrink-0" />
      <span className="truncate">{text(event.bot_activity?.label ?? event.loop_created?.loop_name ?? event.content ?? '')}</span>
    </>
  )
  return onOpen ? (
    <button
      type="button"
      onClick={onOpen}
      className="mx-auto flex min-w-0 max-w-full cursor-pointer items-center justify-center gap-1.5 rounded-md px-2 py-1 text-[12px] text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink-2"
    >
      {line}
    </button>
  ) : (
    <p className="flex min-w-0 items-center justify-center gap-1.5 py-1 text-[12px] text-ink-3">{line}</p>
  )
}
