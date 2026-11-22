import { ChevronDown, Zap } from 'lucide-react'
import { AnimatePresence, MotionConfig, motion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { ReasoningEffortSlider } from '@/components/acp/ReasoningEffortSlider'
import { AgentTabs, agentTabsWidth, useAgentSwipe, type AgentChoice } from '@/components/session/AgentTabs'
import { Pop, Roll, SWAP, Tick, useDirection, useWidthTransition } from '@/components/session/pickerMotion'
import { Button } from '@/components/ui/Button'
import { Popover } from '@/components/ui/Popover'
import { agentLabel } from '@/lib/agentLabel'
import type { ReasoningEffortOption } from '@/lib/api/types'
import type { FastModeControl } from '@/lib/agentConfig'
import type { ModelPickerOption, ModelSelection } from '@/lib/modelPicker'
import { modelSuggestionFor, modelSuggestionLabel } from '@/lib/modelSuggestion'
import { reasoningEffortLabel } from '@/lib/reasoningEfforts'

const list = {
  rest: { transition: { staggerChildren: 0.025, delayChildren: 0.03 } },
  exit: (direction: number) => ({ x: direction * -16, opacity: 0, transition: { duration: 0.14 } }),
}
const row = {
  enter: (direction: number) => ({ x: direction * 16, opacity: 0 }),
  rest: { x: 0, opacity: 1, transition: SWAP },
}

// One agent's models in a fixed-height, scrolling area, so the picker keeps its size.
// A new `id` slides the old list out and the new one in along `direction`.
function ModelList({
  id,
  direction,
  models,
  selected,
  loading,
  disabled,
  onPick,
}: {
  id: string
  direction: number
  models: ModelPickerOption[]
  selected?: ModelPickerOption
  loading?: boolean
  disabled?: boolean
  onPick: (model: ModelPickerOption) => void
}) {
  return (
    <div className="relative h-42 overflow-hidden">
      <AnimatePresence mode="popLayout" custom={direction}>
        <motion.div
          key={id}
          role="menu"
          aria-label="Models"
          custom={direction}
          variants={list}
          initial="enter"
          animate="rest"
          exit="exit"
          className="h-full w-full overflow-y-auto"
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
          {models.map((model, index) => {
            const on = model === selected
            return (
              <motion.button
                key={model.value}
                type="button"
                role="menuitemradio"
                aria-checked={on}
                tabIndex={on || (!selected && index === 0) ? 0 : -1}
                custom={direction}
                variants={row}
                disabled={disabled}
                onClick={() => onPick(model)}
                className={`flex h-7 w-full items-center gap-2 rounded-control px-2.5 text-left text-[12px] transition-colors hover:bg-surface-2 focus-visible:bg-surface-2 ${on ? 'text-ink' : 'text-ink-2'}`}
              >
                <span className="min-w-0 flex-1 truncate">{model.label}</span>
                <Tick on={on} />
              </motion.button>
            )
          })}
          {models.length ? null : <div className="h-7 px-2.5 text-[12px] leading-7 text-ink-3">{loading ? 'Loading models…' : 'No models'}</div>}
        </motion.div>
      </AnimatePresence>
    </div>
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
  const swipeAgent = useAgentSwipe(disabled ? undefined : tabs)
  const adjustable = effortOptions.length > 1
  const listed = Boolean(tabs || suggestions.length || loading)
  const focusSelection = useCallback((dialog: HTMLElement | null) => {
    const model = dialog?.querySelector<HTMLElement>('[role="menuitemradio"][tabindex="0"]')
    const target = model ?? dialog?.querySelector<HTMLElement>('[role="tab"][tabindex="0"]')
    target?.focus()
  }, [])

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
            className="model-picker relative overflow-hidden focus-visible:bg-surface-2"
            style={{ justifyContent: 'flex-start' }}
            aria-haspopup="dialog"
            aria-expanded={open && !disabled}
            aria-label={`Model: ${label}${effortLabel ? `, reasoning effort: ${effortLabel}` : ''}${agent ? `, agent: ${agentLabel(agent.value)}` : ''}${fast ? ', Fast Mode' : ''}`}
            title={[label, effortLabel].filter(Boolean).join(' ') + (fast ? ' · Fast Mode' : '')}
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
          onWheel={swipeAgent}
          className="model-picker max-w-[calc(100vw-32px)]"
          style={{ width: Math.max(256, tabs ? agentTabsWidth(tabs.options.length) : 0) }}
        >
          {tabs ? <AgentTabs {...tabs} disabled={disabled} /> : null}
          {listed ? (
            <ModelList
              id={agent?.value ?? ''}
              direction={agentDirection}
              models={suggestions}
              selected={selected}
              loading={loading}
              disabled={disabled}
              onPick={(model) => onChange({
                model: model.value,
                effort: model.reasoning && !model.reasoning.efforts?.includes(effortValue) ? model.reasoning.default_effort ?? '' : effortValue,
              })}
            />
          ) : null}
          {listed || adjustable || fastMode ? (
            <div className={`px-1 pt-1 ${listed ? 'mt-1.5 border-t border-border' : ''}`}>
              <div className="flex h-6 items-center px-1.5 text-[12px] font-medium">
                <Roll value={effortLabel || 'No effort levels'} direction={effortDirection} className={effortClass || (effortLabel ? 'text-ink' : 'font-normal text-ink-3')} />
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
              {adjustable ? (
                <ReasoningEffortSlider
                  compact
                  options={effortOptions}
                  value={effortValue}
                  disabled={disabled}
                  onChange={(effort) => onChange({ model: value, effort })}
                />
              ) : (
                <div className="flex h-9 items-center">
                  <div className="h-6 w-full rounded-[10px] bg-ink/5" />
                </div>
              )}
            </div>
          ) : null}
        </div>
      </Popover>
    </MotionConfig>
  )
}
