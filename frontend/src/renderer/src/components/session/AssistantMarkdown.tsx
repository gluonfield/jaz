import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { hasTime, messageTime } from '@/lib/format/time'
import { useCopyAction } from '@/lib/useCopyAction'
import { MessageMarkdown } from './MessageMarkdown'
import { PreviewSuggestions } from './PreviewSuggestion'

export function AssistantMarkdown({
  text,
  createdAt,
  showCopy = true,
}: {
  text: string
  createdAt?: string
  showCopy?: boolean
}) {
  return (
    <div className="group/message flex min-w-0 flex-col items-start gap-1">
      <MessageMarkdown text={text} />
      <PreviewSuggestions text={text} />
      {showCopy ? <AssistantMessageActions text={text} createdAt={createdAt} /> : null}
    </div>
  )
}

function AssistantMessageActions({ text, createdAt }: { text: string; createdAt?: string }) {
  const { copied, copy } = useCopyAction(text)
  return (
    <div className="pointer-events-none mt-0.5 flex items-center gap-4 [font-size:var(--prose-font-size,0.875rem)] text-ink-3 opacity-0 group-hover/message:pointer-events-auto group-hover/message:opacity-100 has-[:focus-visible]:pointer-events-auto has-[:focus-visible]:opacity-100">
      <button
        type="button"
        aria-label={copied ? 'Copied message as Markdown' : 'Copy message as Markdown'}
        title={copied ? 'Copied' : 'Copy message as Markdown'}
        onClick={() => void copy()}
        className="group relative grid size-4 shrink-0 cursor-pointer place-items-center rounded transition-[color,transform] duration-150 before:absolute before:-inset-3 before:content-[''] hover:text-ink active:scale-[0.96]"
      >
        <CopyToggleIcon copied={copied} />
      </button>
      {hasTime(createdAt) ? (
        <time dateTime={createdAt} title={new Date(createdAt).toLocaleString()} className="tabular-nums">
          {messageTime(createdAt)}
        </time>
      ) : null}
    </div>
  )
}
