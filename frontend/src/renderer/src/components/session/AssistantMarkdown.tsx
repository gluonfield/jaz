import { MessageActions } from '@/components/session/MessageActions'
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
      {showCopy ? <MessageActions text={text} createdAt={createdAt} /> : null}
    </div>
  )
}
