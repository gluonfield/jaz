import { ChevronDown, Zap } from 'lucide-react'
import { AnimatePresence, MotionConfig, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, type RefObject, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { ReasoningEffortSlider } from '@/components/acp/ReasoningEffortSlider'
import { AgentTabs, agentTabsWidth, type AgentChoice } from '@/components/session/AgentTabs'
import { Button } from '@/components/ui/Button'
import { Popover } from '@/components/ui/Popover'
import { agentLabel } from '@/lib/agentLabel'
import type { ReasoningEffortOption } from '@/lib/api/types'
import type { FastModeControl } from '@/lib/agentConfig'
import type { ModelPickerOption, ModelSelection } from '@/lib/modelPicker'
import { modelSuggestionFor, modelSuggestionLabel } from '@/lib/modelSuggestion'
import { reasoningEffortLabel } from '@/lib/reasoningEfforts'

const EASE = [0.2, 0, 0, 1] as const
const SWAP = { duration: 0.22, ease: EASE }
const ROW = 28

const roll = {
  enter: (direction: number) => ({ y: `${direction * 100}%`, opacity: 0 }),
  rest: { y: 0, opacity: 1 },
  exit: (direction: number) => ({ y: `${direction * -100}%`, opacity: 0 }),
}
const pop = {
  enter: { scale: 0.4, opacity: 0, filter: 'blur(3px)' },
  rest: { scale: 1, opacity: 1, filter: 'blur(0px)' },
}
const list = {
  rest: { transition: { staggerChildren: 0.025, delayChildren: 0.03 } },
  exit: (direction: number) => ({ x: direction * -16, opacity: 0, transition: { duration: 0.14 } }),
}
const row = {
  enter: (direction: number) => ({ x: direction * 16, opacity: 0 }),
  rest: { x: 0, opacity: 1, transition: SWAP },
}

// Which way the value last moved, so entering content follows it.
function useDirection(index: number): number {
  const [last, setLast] = useState({ index, direction: 1 })
  if (last.index === index) {
    return last.direction
  }
  const next = { index, direction: index > last.index ? 1 : -1 }
  setLast(next)
  return next.direction
}

function useWidthTransition(ref: RefObject<HTMLElement | null>, content: string) {
  const reduceMotion = useReducedMotion()
  const settled = useRef<number>(undefined)
  const running = useRef<Animation>(undefined)
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) {
      return
    }
    const from = running.current?.playState === 'running' ? element.offsetWidth : settled.current
    running.current?.cancel()
    const to = element.offsetWidth
    settled.current = to
    if (!reduceMotion && from !== undefined && from !== to) {
      running.current = element.animate({ width: [`${from}px`, `${to}px`] }, { duration: 240, easing: `cubic-bezier(${EASE.join()})` })
    }
  }, [ref, content, reduceMotion])
}

function Roll({ value, direction, className = '' }: { value: string; direction: number; className?: string }) {
  return (
    <span className="relative inline-flex h-4 shrink-0 overflow-hidden leading-4">
      <AnimatePresence mode="popLayout" initial={false} custom={direction}>
        <motion.span
          key={value}
          custom={direction}
          variants={roll}
          initial="enter"
          animate="rest"
          exit="exit"
          transition={SWAP}
          className={`truncate ${className}`}
        >
          {value}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}

function Pop({ id, children }: { id: string; children: ReactNode }) {
  return (
    <AnimatePresence mode="popLayout" initial={false}>
      <motion.span key={id} variants={pop} initial="enter" animate="rest" exit="enter" transition={SWAP} className="grid shrink-0 place-items-center">
        {children}
      </motion.span>
    </AnimatePresence>
  )
}

function Tick({ on }: { on: boolean }) {
  return (
    <svg viewBox="0 0 24 24" className="size-[13px] shrink-0 text-ink-3" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <motion.path
        d="M4 12l5 5L20 6"
        initial={false}
        animate={{ pathLength: on ? 1 : 0, opacity: on ? 1 : 0 }}
        transition={{ duration: on ? 0.26 : 0.1, ease: EASE }}
      />
    </svg>
  )
}

export function ModelSelect({
  value,
  effort,
  suggestions,
  effortOptions,
  agent,
  loading,
  disabled,
  fastMode,
  placement,
  align,
  onChange,
}: {
  value: string
  effort: string
  suggestions: ModelPickerOption[]
  effortOptions: ReasoningEffortOption[]
  // Puts the agent's logo on the button and, with several options, agent tabs in the picker.
  agent?: AgentChoice
  loading?: boolean
  disabled?: boolean
  fastMode?: FastModeControl
  placement?: 'above' | 'below'
  align?: 'start' | 'end'
  onChange: (selection: ModelSelection) => void
}) {
  const tabs = agent && agent.options.length > 1 ? agent : undefined
  disabled ||= (!tabs && suggestions.length === 0 && effortOptions.length === 0 && !fastMode)
  const [open, setOpen] = useState(false)
  useEffect(() => {
    if (disabled) setOpen(false)
  }, [disabled])
  const triggerRef = useRef<HTMLButtonElement>(null)
  const selected = modelSuggestionFor(suggestions, value)
  const effortValue = effort || selected?.reasoning?.default_effort || ''
  const label = value ? modelSuggestionLabel(suggestions, value) : agent ? agentLabel(agent.value) : 'Model'
  const automatic = selected?.reasoning?.automatic && effortOptions.length === 0
  const effortLabel = automatic
    ? 'Thinking'
    : effortOptions.length ? reasoningEffortLabel(effortValue, effortOptions) : ''
  const effortClass = effortValue === 'ultra' || effortValue === 'ultracode' ? 'jaz-gradient' : ''
  const fast = fastMode?.checked === true
  const agentDirection = useDirection(agent ? agent.options.indexOf(agent.value) : 0)
  const effortDirection = useDirection(effortOptions.findIndex((option) => option.value === effortValue))
  useWidthTransition(triggerRef, `${agent?.value}/${label}/${effortLabel}/${fast}`)
  const focusSelection = useCallback((dialog: HTMLElement | null) => {
    const model = dialog?.querySelector<HTMLElement>('[role="menuitemradio"][tabindex="0"]')
    const target = model ?? dialog?.querySelector<HTMLElement>('[role="tab"][tabindex="0"]')
    target?.focus()
  }, [])
  const listHeight = suggestions.length ? Math.min(224, suggestions.length * ROW) : loading ? ROW : 0

  return (
    <MotionConfig reducedMotion="user">
      <Popover
        open={open && !disabled}
        onClose={() => {
          setOpen(false)
          triggerRef.current?.focus()
        }}
        placement={placement}
        align={align}
        trigger={
          <Button
            ref={triggerRef}
            variant="secondary"
            size="sm"
            className="model-picker relative justify-start overflow-hidden focus-visible:bg-surface-2"
            aria-haspopup="dialog"
            aria-expanded={open && !disabled}
            aria-label={`Model: ${label}${effortLabel ? `, reasoning effort: ${effortLabel}` : ''}${agent ? `, agent: ${agentLabel(agent.value)}` : ''}${fast ? ', Fast Mode' : ''}`}
            title={`${label} ${effortLabel}${fast ? ' · Fast Mode' : ''}`}
            disabled={disabled}
            onClick={() => setOpen(!open)}
          >
            {agent && hasAgentLogo(agent.value) ? <Pop id={agent.value}><AgentLogo agent={agent.value} size={14} /></Pop> : null}
            <Roll value={label} direction={agentDirection} className="max-w-28" />
            {effortLabel ? <Roll value={effortLabel} direction={effortDirection} className={effortClass || 'text-ink-3'} /> : null}
            {fast ? <Zap size={12} fill="currentColor" className="shrink-0 text-primary" /> : null}
            <ChevronDown size={13} className={`shrink-0 transition-transform duration-200 ${open && !disabled ? 'rotate-180' : ''}`} />
          </Button>
        }
      >
        <div
          ref={focusSelection}
          role="dialog"
          aria-label="Model and effort"
          className="model-picker max-w-[calc(100vw-32px)]"
          style={{ width: Math.max(256, tabs ? agentTabsWidth(tabs.options.length) : 0) }}
        >
          {tabs ? <AgentTabs {...tabs} disabled={disabled} /> : null}
          <motion.div
            className="relative overflow-hidden"
            initial={false}
            animate={{ height: listHeight }}
            transition={{ duration: 0.26, ease: EASE }}
          >
            <AnimatePresence mode="popLayout" custom={agentDirection}>
              <motion.div
                key={agent?.value ?? ''}
                role="menu"
                aria-label="Models"
                custom={agentDirection}
                variants={list}
                initial="enter"
                animate="rest"
                exit="exit"
                className="max-h-56 w-full overflow-y-auto"
                onKeyDown={(event) => {
                  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') {
                    return
                  }
                  event.preventDefault()
                  const rows = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button')]
                  const current = rows.indexOf(document.activeElement as HTMLButtonElement)
                  rows[(current + (event.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length]?.focus()
                }}
              >
                {suggestions.map((model, index) => {
                  const on = model.value === selected?.value
                  return (
                    <motion.button
                      key={model.value}
                      type="button"
                      role="menuitemradio"
                      aria-checked={on}
                      tabIndex={on || (!selected && index === 0) ? 0 : -1}
                      custom={agentDirection}
                      variants={row}
                      disabled={disabled}
                      onClick={() => onChange({
                        model: model.value,
                        effort: model.reasoning && !model.reasoning.efforts?.includes(effortValue) ? model.reasoning.default_effort ?? '' : effortValue,
                      })}
                      className={`flex h-7 w-full items-center gap-2 rounded-control px-2.5 text-left text-[12px] transition-colors hover:bg-surface-2 focus-visible:bg-surface-2 ${on ? 'text-ink' : 'text-ink-2'}`}
                    >
                      <span className="min-w-0 flex-1 truncate">{model.label}</span>
                      <Tick on={on} />
                    </motion.button>
                  )
                })}
                {!suggestions.length && loading ? <div className="h-7 px-2.5 text-[12px] leading-7 text-ink-3">Loading models…</div> : null}
              </motion.div>
            </AnimatePresence>
          </motion.div>
          <AnimatePresence initial={false}>
            {effortOptions.length > 1 || fastMode ? (
              <motion.div
                key="effort"
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: 'auto', opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.26, ease: EASE }}
                className="overflow-hidden"
              >
                <div className={`px-1 pt-1 ${tabs || listHeight ? 'mt-1.5 border-t border-border' : ''}`}>
                  <div className="flex h-6 items-center px-1.5 text-[12px] font-medium">
                    <Roll value={effortLabel} direction={effortDirection} className={effortClass || 'text-ink'} />
                    {fastMode ? (
                      <button
                        type="button"
                        role="switch"
                        aria-label="Fast Mode"
                        aria-checked={fast}
                        disabled={fastMode.disabled}
                        onClick={() => fastMode.onChange(!fast)}
                        className={`relative ml-auto inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2 transition-[background-color,color,scale] active:scale-[0.94] disabled:opacity-50 ${fast
                          ? 'bg-primary text-on-primary'
                          : 'text-ink-3 enabled:hover:bg-surface-2 enabled:hover:text-ink'}`}
                      >
                        <Pop id={String(fast)}><Zap size={12} fill={fast ? 'currentColor' : 'none'} /></Pop>
                        Fast
                      </button>
                    ) : null}
                  </div>
                  {effortOptions.length > 1 ? (
                    <ReasoningEffortSlider
                      compact
                      options={effortOptions}
                      value={effortValue}
                      disabled={disabled}
                      onChange={(effort) => onChange({ model: value, effort })}
                    />
                  ) : null}
                </div>
              </motion.div>
            ) : null}
          </AnimatePresence>
        </div>
      </Popover>
    </MotionConfig>
  )
}
