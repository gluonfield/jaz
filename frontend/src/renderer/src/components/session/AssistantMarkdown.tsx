import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { useCopyAction } from '@/lib/useCopyAction'
import { MessageMarkdown } from './MessageMarkdown'
import { PreviewSuggestions } from './PreviewSuggestion'

export function AssistantMarkdown({ text }: { text: string }) {
  return (
    <div className="flex min-w-0 flex-col items-start gap-1">
      <MessageMarkdown text={text} />
      <PreviewSuggestions text={text} />
      <AssistantCopyButton text={text} />
    </div>
  )
}

function AssistantCopyButton({ text }: { text: string }) {
  const { copied, copy } = useCopyAction(text)
  return (
    <button
      type="button"
      aria-label={copied ? 'Copied message as Markdown' : 'Copy message as Markdown'}
      title={copied ? 'Copied' : 'Copy message as Markdown'}
      onClick={() => void copy()}
      className="group mt-0.5 inline-flex h-7 w-fit cursor-pointer items-center gap-1.5 rounded-full px-2 text-[12px] font-medium text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
    >
      <CopyToggleIcon copied={copied} />
      <span>{copied ? 'Copied' : 'Copy'}</span>
    </button>
  )
}
