import { Check, ChevronDown, ChevronRight, LoaderCircle, Zap } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useRef, useState } from 'react'
import { ReasoningEffortSlider } from '@/components/acp/ReasoningEffortSlider'
import { Button } from '@/components/ui/Button'
import { Popover } from '@/components/ui/Popover'
import type { ReasoningEffortOption } from '@/lib/api/types'
import type { FastModeControl } from '@/lib/agentConfig'
import type { ModelPickerOption, ModelSelection } from '@/lib/modelPicker'
import { modelSuggestionFor, modelSuggestionLabel } from '@/lib/modelSuggestion'
import { reasoningEffortLabel } from '@/lib/reasoningEfforts'

export function ModelSelect({
  value,
  effort,
  suggestions,
  effortOptions,
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
  loading?: boolean
  disabled?: boolean
  fastMode?: FastModeControl
  placement?: 'above' | 'below'
  align?: 'start' | 'end'
  onChange: (selection: ModelSelection) => void
}) {
  const [open, setOpen] = useState(false)
  const [view, setView] = useState<'slider' | 'models'>('slider')
  const triggerRef = useRef<HTMLButtonElement>(null)
  const reduceMotion = useReducedMotion()
  const selected = modelSuggestionFor(suggestions, value)
  const effortValue = effort || selected?.reasoning?.default_effort || ''
  const label = value ? modelSuggestionLabel(suggestions, value) : 'Model'
  const options = effortOptions
  const effortLabel = selected?.reasoning?.automatic && effortOptions.length === 0
    ? 'Thinking'
    : reasoningEffortLabel(effortValue, options)
  const ultra = effortValue === 'ultra' || effortValue === 'ultracode'
  const fast = fastMode?.checked === true
  const height = view === 'models'
    ? Math.min(224, Math.max(1, suggestions.length) * 28)
    : options.length > 1 ? 64 : 28

  const selectModel = (model: ModelPickerOption) => {
    onChange({
      model: model.value,
      effort: model.reasoning && !model.reasoning.efforts?.includes(effortValue) ? model.reasoning.default_effort ?? '' : effortValue,
    })
    setView('slider')
  }

  return (
    <Popover
      open={open}
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
          className="model-picker max-w-[13rem] focus-visible:bg-surface-2"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={`Model: ${label}, reasoning effort: ${effortLabel}${fast ? ', Fast Mode' : ''}`}
          title={`${label} ${effortLabel}${fast ? ' · Fast Mode' : ''}`}
          disabled={disabled}
          onClick={() => {
            setView('slider')
            setOpen(!open)
          }}
        >
          <span className="truncate">{label}</span>
          <span className={`shrink-0 ${ultra ? 'jaz-gradient' : 'text-ink-3'}`}>{effortLabel}</span>
          {fast ? <Zap size={12} fill="currentColor" className="shrink-0 text-primary" /> : null}
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      <motion.div
        role="dialog"
        aria-label="Model and effort"
        className="model-picker w-[256px] max-w-[calc(100vw-32px)] overflow-hidden"
        initial={false}
        animate={{ height }}
        transition={{ duration: reduceMotion ? 0 : 0.2, ease: [0.2, 0, 0, 1] }}
      >
        <motion.div
          key={view}
          initial={{ opacity: 0, x: reduceMotion ? 0 : view === 'models' ? 6 : -6 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ duration: reduceMotion ? 0 : 0.16, ease: 'easeOut' }}
        >
          {view === 'slider' ? (
            <div className="px-2 pb-1">
              <div className="flex items-center gap-1">
                <button
                  autoFocus
                  type="button"
                  onClick={() => setView('models')}
                  aria-label={`Select model, ${label}`}
                  className="inline-flex h-6 min-w-0 items-center gap-1.5 rounded-control px-2 text-[12px] font-medium transition-colors hover:bg-surface-2 focus-visible:bg-surface-2"
                >
                  <span className="min-w-0 truncate text-ink">
                    {label} <span className={ultra ? 'jaz-gradient' : 'text-ink-2'}>{effortLabel}</span>
                  </span>
                  {loading ? <LoaderCircle size={13} className="shrink-0 animate-spin text-ink-3" /> : <ChevronRight size={13} className="shrink-0 text-ink-3" />}
                </button>
                {fastMode ? (
                  <button
                    type="button"
                    role="switch"
                    aria-label="Fast Mode"
                    aria-checked={fast}
                    disabled={fastMode.disabled}
                    onClick={() => fastMode.onChange(!fast)}
                    className={`ml-auto inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2 text-[12px] font-medium transition-colors disabled:opacity-50 ${fast
                      ? 'bg-primary text-on-primary'
                      : 'text-ink-3 enabled:hover:bg-surface-2 enabled:hover:text-ink'}`}
                  >
                    <Zap size={12} fill={fast ? 'currentColor' : 'none'} />
                    Fast
                  </button>
                ) : null}
              </div>
              {options.length > 1 ? (
                <ReasoningEffortSlider
                  compact
                  options={options}
                  value={effortValue}
                  disabled={disabled || loading}
                  onChange={(effort) => onChange({ model: value, effort })}
                />
              ) : null}
            </div>
          ) : (
            <div
              role="menu"
              aria-label="Models"
              className="max-h-56 overflow-y-auto"
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
              {suggestions.map((model, index) => (
                <button
                  key={model.value}
                  type="button"
                  role="menuitemradio"
                  aria-checked={model.value === selected?.value}
                  autoFocus={model.value === selected?.value || (!selected && index === 0)}
                  onClick={() => selectModel(model)}
                  disabled={disabled}
                  className={`flex h-7 w-full items-center gap-2 rounded-control px-2.5 text-left text-[12px] transition-colors hover:bg-surface-2 focus-visible:bg-surface-2 ${model.value === selected?.value ? 'text-ink' : 'text-ink-2'}`}
                >
                  <span className="min-w-0 flex-1 truncate">{model.label}</span>
                  {model.value === selected?.value ? <Check size={13} className="shrink-0 text-ink-3" /> : null}
                </button>
              ))}
              {!suggestions.length ? <div className="h-7 px-2.5 text-[12px] leading-7 text-ink-3">{loading ? 'Loading models…' : 'No models available'}</div> : null}
            </div>
          )}
        </motion.div>
      </motion.div>
    </Popover>
  )
}
