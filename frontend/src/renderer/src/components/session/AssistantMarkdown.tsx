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
  return (
    <button
      type="button"
      aria-label={copied ? 'Copied message as Markdown' : 'Copy message as Markdown'}
      title={copied ? 'Copied' : 'Copy message as Markdown'}
      onClick={() => void copy()}
      className="group relative mt-0.5 grid size-4 cursor-pointer place-items-center rounded text-ink-3 transition-[color,transform] duration-150 before:absolute before:-inset-3 before:content-[''] hover:text-ink active:scale-[0.96]"
    >
      <CopyToggleIcon copied={copied} />
    </button>
  )
}
