import { Cloud, Laptop } from 'lucide-react'
import { useState, type WheelEvent } from 'react'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { agentLabel } from '@/lib/agentLabel'
import { groupPickerAgents, type PickerAgentOption } from '@/lib/modelPicker'
import { createSwipe } from '@/lib/swipe'

const TAB = 28
const STEP = TAB + 2
const NAMED = 96

export interface AgentChoice {
  value: string
  options: string[]
  items?: PickerAgentOption[]
  onChange: (agent: string) => void
}

export function agentTabsWidth(count: number): number {
  return NAMED + (count - 1) * STEP
}

function orderedAgentOptions(agent: AgentChoice): string[] {
  if (!agent.items?.length) {
    return agent.options
  }
  return groupPickerAgents(agent.items).flatMap((group) => group.items.map((item) => item.id))
}

function selectTab(agent: AgentChoice, index: number, within: HTMLElement) {
  const ordered = orderedAgentOptions(agent)
  if (!ordered[index]) {
    return
  }
  agent.onChange(ordered[index])
  within.querySelectorAll<HTMLButtonElement>('[role="tab"]')[index]?.focus()
}

// A horizontal touchpad swipe anywhere in the picker moves one agent along.
export function useAgentSwipe(agent?: AgentChoice) {
  const [swipe] = useState(createSwipe)
  return (event: WheelEvent<HTMLElement>) => {
    if (!agent || Math.abs(event.deltaX) <= Math.abs(event.deltaY)) {
      return
    }
    const ordered = orderedAgentOptions(agent)
    const step = swipe(event.deltaX, event.timeStamp)
    const next = ordered.indexOf(agent.value) + step
    if (step && ordered[next]) {
      selectTab(agent, next, event.currentTarget)
    }
  }
}

function itemLabel(item: PickerAgentOption): string {
  return item.label || agentLabel(item.id)
}

function itemLogoAgent(item: PickerAgentOption): string {
  return item.logoAgent || item.id
}

// One logo per agent; the selected one widens to show its name.
// When `items` includes Cloud agents (or mixed Cloud + Local agents), renders
// explicit Cloud / Local group headers so Desktop can unify both seamlessly.
export function AgentTabs({ value, options, items, onChange, disabled }: AgentChoice & { disabled?: boolean }) {
  const resolved: PickerAgentOption[] = items?.length
    ? items
    : options.map((id) => ({ id, location: 'local' }))
  const hasCloud = resolved.some((item) => item.location === 'cloud')
  const groups = groupPickerAgents(resolved)

  if (hasCloud) {
    return (
      <div
        role="tablist"
        aria-label="Agent"
        className="mb-1.5 flex flex-col gap-1.5 border-b border-border pb-1.5"
        onKeyDown={(event) => {
          if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return
          event.preventDefault()
          const ordered = groups.flatMap((group) => group.items.map((item) => item.id))
          if (!ordered.length) return
          const index = ordered.indexOf(value)
          const next = (index + (event.key === 'ArrowRight' ? 1 : ordered.length - 1)) % ordered.length
          selectTab({ value, options, items, onChange }, next, event.currentTarget)
        }}
      >
        {groups.map((group) => (
          <div key={group.location} className="flex flex-col gap-1">
            <div className="flex items-center gap-1 px-1 text-[10px] font-medium uppercase tracking-wide text-ink-3">
              {group.location === 'cloud' ? <Cloud size={10} className="text-primary" /> : <Laptop size={10} />}
              <span>{group.label}</span>
            </div>
            <div className="flex flex-wrap gap-1">
              {group.items.map((item) => {
                const on = item.id === value
                const label = itemLabel(item)
                const logoId = itemLogoAgent(item)
                return (
                  <button
                    key={item.id}
                    type="button"
                    role="tab"
                    aria-selected={on}
                    aria-label={`${group.label}: ${label}`}
                    title={`${group.label} · ${label}`}
                    tabIndex={on ? 0 : -1}
                    disabled={disabled}
                    onClick={() => onChange(item.id)}
                    className={`group flex h-7 items-center gap-1.5 rounded-control px-2 text-[12px] font-medium transition-[background-color,color,scale] duration-150 active:scale-[0.95] ${
                      on ? 'bg-surface-2 text-ink' : 'text-ink-3 hover:bg-surface-2/60 hover:text-ink'
                    }`}
                  >
                    <span className="grid size-3.5 shrink-0 place-items-center">
                      {hasAgentLogo(logoId) ? <AgentLogo agent={logoId} size={14} /> : label.slice(0, 1)}
                    </span>
                    <span className="truncate">{label}</span>
                  </button>
                )
              })}
            </div>
          </div>
        ))}
      </div>
    )
  }

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
        selectTab({ value, options, onChange }, next, event.currentTarget)
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
