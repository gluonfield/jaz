import type { ComposerContext } from '@/lib/sendMessage'
import { ContextChip } from './ContextChip'

export function MessageContexts({ contexts }: { contexts: ComposerContext[] }) {
  if (contexts.length === 0) return null
  return (
    <div className="mb-1.5 flex flex-wrap justify-end gap-1">
      {contexts.map((context, index) => (
        <ContextChip key={context.id} index={index} context={context} align="right" />
      ))}
    </div>
  )
}
