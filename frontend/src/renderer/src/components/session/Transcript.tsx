import { motion } from 'motion/react'
import type { ChatMessage, SessionEvent } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { ToolCallCard } from './ToolCallCard'

function Bubble({ message, toolResults }: { message: ChatMessage; toolResults: Map<string, string> }) {
  switch (message.role) {
    case 'user':
      return (
        <div className="flex justify-end">
          <div className="max-w-[80%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap select-text">
            {message.content}
          </div>
        </div>
      )
    case 'assistant':
      return (
        <div className="flex max-w-[72ch] flex-col gap-2">
          {message.content ? (
            <p className="text-sm leading-relaxed whitespace-pre-wrap select-text">{message.content}</p>
          ) : null}
          {message.tool_calls?.map((call) => (
            <ToolCallCard
              key={call.id}
              name={call.function.name}
              args={call.function.arguments}
              result={toolResults.get(call.id)}
            />
          ))}
        </div>
      )
    case 'system':
    case 'developer':
      return (
        <details className="text-[12px] text-ink-3">
          <summary className="cursor-pointer select-none">{message.role} prompt</summary>
          <pre className="mt-1 max-h-56 overflow-auto rounded-card bg-surface px-3 py-2 font-mono whitespace-pre-wrap select-text">
            {message.content}
          </pre>
        </details>
      )
    default:
      return null
  }
}

function LiveEvent({ event }: { event: SessionEvent }) {
  return (
    <motion.div
      className="flex max-w-[72ch] flex-col gap-2"
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', stiffness: 380, damping: 30 }}
    >
      {event.acp ? (
        <p className="text-[12px] text-ink-3">
          <span className="font-mono">{event.acp.agent}</span>
          {event.acp.title ? ` · ${event.acp.title}` : ''} · {relativeTime(event.at)}
        </p>
      ) : null}
      {event.content ? (
        <p className="text-sm leading-relaxed whitespace-pre-wrap select-text">{event.content}</p>
      ) : null}
      {event.acp?.error ? (
        <p className="rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
          {event.acp.error}
        </p>
      ) : null}
      {event.acp?.tool_calls?.length ? (
        <div className="flex flex-wrap gap-1.5">
          {event.acp.tool_calls.map((call) => (
            <span
              key={call.id}
              className="rounded border border-border px-1.5 py-px font-mono text-[11px] text-ink-2"
            >
              {call.title || call.id}
            </span>
          ))}
        </div>
      ) : null}
    </motion.div>
  )
}

export function Transcript({
  messages,
  events,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
}) {
  // Pair tool results with the assistant tool_call that produced them.
  const toolResults = new Map<string, string>()
  for (const message of messages) {
    if (message.role === 'tool') toolResults.set(message.tool_call_id, message.content)
  }

  return (
    <div className="flex flex-col gap-5">
      {messages.map((message, i) =>
        message.role === 'tool' ? null : (
          <Bubble key={i} message={message} toolResults={toolResults} />
        ),
      )}
      {events.map((event, i) => (
        <LiveEvent key={i} event={event} />
      ))}
    </div>
  )
}
