import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { useCopyAction } from '@/lib/useCopyAction'
import { MessageMarkdown } from './MessageMarkdown'
import { PreviewSuggestions } from './PreviewSuggestion'

export function AssistantMarkdown({
  text,
  showCopy = true,
}: {
  text: string
  showCopy?: boolean
}) {
  return (
    <div className="flex min-w-0 flex-col items-start gap-1">
      <MessageMarkdown text={text} />
      <PreviewSuggestions text={text} />
      {showCopy ? <AssistantCopyButton text={text} /> : null}
    </div>
  )
}

function AssistantCopyButton({ text }: { text: string }) {
  const { copied, copy } = useCopyAction(text)
  const label = copied ? 'Copied' : 'Copy'
  return (
    <button
      type="button"
      aria-label={copied ? 'Copied message as Markdown' : 'Copy message as Markdown'}
      title={copied ? 'Copied' : 'Copy message as Markdown'}
      onClick={() => void copy()}
      className="group relative mt-0.5 grid size-7 cursor-pointer place-items-center rounded-full text-ink-3 transition-[background-color,color,transform] duration-150 before:absolute before:-inset-1.5 before:content-[''] hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
    >
      <CopyToggleIcon copied={copied} />
      <span
        aria-hidden
        className="pointer-events-none absolute left-full top-1/2 z-10 ml-1 -translate-x-1 -translate-y-1/2 rounded-md bg-surface-2 px-2 py-1 text-[11px] font-medium text-ink opacity-0 shadow-sm transition-[opacity,transform] duration-150 group-hover:translate-x-0 group-hover:opacity-100 group-focus-visible:translate-x-0 group-focus-visible:opacity-100"
      >
        {label}
      </span>
    </button>
  )
}
