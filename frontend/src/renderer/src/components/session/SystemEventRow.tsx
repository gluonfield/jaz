import { ArrowDownLeft, ArrowLeftRight, ArrowUpRight, BookOpen, CalendarPlus, CircleAlert, Clock3, type LucideIcon, PenLine, Users } from 'lucide-react'
import { agentLabel } from '@/lib/agentLabel'
import type { BotActivityEvent, SessionEvent } from '@/lib/api/types'

const ACTIVITY: Record<BotActivityEvent['kind'], [LucideIcon, (label: string) => string]> = {
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
