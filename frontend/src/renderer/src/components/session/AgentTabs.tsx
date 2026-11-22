import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { agentLabel } from '@/lib/agentLabel'

const TAB = 28
const STEP = TAB + 2
const NAMED = 96

export interface AgentChoice {
  value: string
  options: string[]
  onChange: (agent: string) => void
}

export function agentTabsWidth(count: number): number {
  return NAMED + (count - 1) * STEP
}

// One logo per agent; the selected one widens to show its name.
export function AgentTabs({ value, options, onChange, disabled }: AgentChoice & { disabled?: boolean }) {
  const index = options.indexOf(value)
  const named = `calc(100% - ${(options.length - 1) * STEP}px)`
  return (
    <div
      role="tablist"
      aria-label="Agent"
      className="relative mb-1.5 flex gap-0.5 border-b border-border pb-1.5"
      onKeyDown={(event) => {
        if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') {
          return
        }
        event.preventDefault()
        const next = (index + (event.key === 'ArrowRight' ? 1 : options.length - 1)) % options.length
        onChange(options[next])
        event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus()
      }}
    >
      <span
        aria-hidden
        className="absolute top-0 h-7 rounded-control bg-surface-2 transition-[left] duration-300 ease-[cubic-bezier(0.3,1.2,0.5,1)] motion-reduce:transition-none"
        style={{ left: Math.max(0, index) * STEP, width: named }}
      />
      {options.map((agent, i) => {
        const on = i === index
        const label = agentLabel(agent)
        return (
          <button
            key={agent}
            type="button"
            role="tab"
            aria-selected={on}
            aria-label={label}
            title={label}
            tabIndex={on ? 0 : -1}
            disabled={disabled}
            onClick={() => onChange(agent)}
            className={`group relative flex h-7 shrink-0 items-center overflow-hidden rounded-control px-[7px] text-[12px] font-medium transition-[width,color,scale] duration-300 ease-[cubic-bezier(0.2,0,0,1)] active:scale-[0.92] motion-reduce:transition-none ${on ? 'text-ink' : 'text-ink-3 hover:text-ink focus-visible:text-ink'}`}
            style={{ width: on ? named : TAB }}
          >
            <span className="grid size-3.5 shrink-0 place-items-center transition-transform duration-200 group-hover:scale-110">
              {hasAgentLogo(agent) ? <AgentLogo agent={agent} size={14} /> : label.slice(0, 1)}
            </span>
            <span className={`ml-1.5 truncate transition-[opacity,translate] duration-200 motion-reduce:transition-none ${on ? 'delay-75' : '-translate-x-1 opacity-0'}`}>
              {label}
            </span>
          </button>
        )
      })}
    </div>
  )
}
