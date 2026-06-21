import { MessageSquare } from 'lucide-react'
import { agentLabel } from '@/lib/agentLabel'
import { AgentLogo, hasAgentLogo } from './AgentLogo'

export function AgentAvatar({
  agent,
  size = 18,
  className = '',
}: {
  agent?: string
  size?: number
  className?: string
}) {
  const slug = agent?.trim()
  const label = slug ? agentLabel(slug) : 'Thread'
  const style = { width: size, height: size }

  if (slug && hasAgentLogo(slug)) {
    return (
      <span title={label} style={style} className={`inline-grid shrink-0 place-items-center text-ink ${className}`}>
        <AgentLogo agent={slug} size={size} />
      </span>
    )
  }

  if (slug) {
    return (
      <span
        title={label}
        style={style}
        className={`inline-grid shrink-0 place-items-center rounded-full bg-primary-soft text-[10px] leading-none font-semibold text-primary-strong ${className}`}
      >
        {label.slice(0, 1)}
      </span>
    )
  }

  return (
    <span title={label} style={style} className={`inline-grid shrink-0 place-items-center text-primary ${className}`}>
      <MessageSquare size={Math.max(11, size - 5)} aria-hidden />
    </span>
  )
}
