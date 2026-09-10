import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { hasTime, messageTime } from '@/lib/format/time'
import { useCopyAction } from '@/lib/useCopyAction'

export function MessageActions({ text, createdAt }: { text: string; createdAt?: string }) {
  const { copied, copy } = useCopyAction(text)
  return (
    <div className="pointer-events-none mt-0.5 flex items-center gap-4 text-[12px] text-ink-3 opacity-0 group-hover/message:pointer-events-auto group-hover/message:opacity-100 has-[:focus-visible]:pointer-events-auto has-[:focus-visible]:opacity-100">
      {text ? (
        <button
          type="button"
          aria-label={copied ? 'Copied message as Markdown' : 'Copy message as Markdown'}
          title={copied ? 'Copied' : 'Copy message as Markdown'}
          onClick={() => void copy()}
          className="group relative grid size-4 shrink-0 cursor-pointer place-items-center rounded transition-[color,transform] duration-150 before:absolute before:-inset-3 before:content-[''] hover:text-ink active:scale-[0.96]"
        >
          <CopyToggleIcon copied={copied} />
        </button>
      ) : null}
      {hasTime(createdAt) ? (
        <time dateTime={createdAt} title={new Date(createdAt).toLocaleString()}>
          {messageTime(createdAt)}
        </time>
      ) : null}
    </div>
  )
}
