import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { relativeTime } from '@/lib/format/time'
import type { ActivityHeader } from './timeline'

export function ACPEventHeader({ agent, title, at }: ActivityHeader) {
  return (
    <p className="text-[12px] text-ink-3">
      {hasAgentLogo(agent) ? (
        <AgentLogo
          agent={agent}
          size={12}
          className="inline-block translate-y-[2px] text-ink-2"
        />
      ) : (
        <span className="font-mono">{agent}</span>
      )}
      {title ? ` · ${title}` : ''} · {relativeTime(at)}
    </p>
  )
}
