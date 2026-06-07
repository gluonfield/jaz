import { motion } from 'motion/react'
import type { ChatMessage, SessionEvent } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { MessageMarkdown } from './MessageMarkdown'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCallCard } from './ToolCallCard'

function messageText(message: ChatMessage): string {
  // Each text block is a separate utterance; join as paragraphs so block
  // boundaries don't fuse sentences together ("…intact.Updated…").
  const text = message.blocks
    ?.filter((block) => block.type === 'text')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.content
}

function messageReasoning(message: ChatMessage): string {
  const text = message.blocks
    ?.filter((block) => block.type === 'reasoning')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.reasoning || ''
}

function Bubble({ message }: { message: ChatMessage }) {
  switch (message.role) {
    case 'user':
      return (
        <div className="flex justify-end">
          <div className="max-w-[80%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap select-text">
            {messageText(message)}
          </div>
        </div>
      )
    case 'assistant': {
      const text = messageText(message)
      const reasoning = messageReasoning(message)
      return (
        <div className="flex max-w-[72ch] flex-col gap-2">
          <ThinkingBlock text={reasoning} />
          {text ? <MessageMarkdown text={text} /> : null}
          {message.blocks
            ?.filter((block) => block.type === 'tool')
            .map((block) => (
              <ToolCallCard
                key={block.id}
                name={block.name}
                args={block.input_json}
                result={block.result}
              />
          ))}
        </div>
      )
    }
    // system/developer prompts are plumbing, not conversation — never shown
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
      {event.content ? <MessageMarkdown text={event.content} /> : null}
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
  return (
    <div className="flex flex-col gap-5">
      {messages.map((message) => (
        <Bubble key={message.seq} message={message} />
      ))}
      {events.map((event, i) => (
        <LiveEvent key={i} event={event} />
      ))}
    </div>
  )
}
