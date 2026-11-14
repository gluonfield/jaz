import { Check, ChevronDown, LoaderCircle, Zap } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Popover } from '@/components/ui/Popover'
import type { FastModeControl } from '@/lib/agentConfig'
import type { ModelPickerOption, ModelSelection } from '@/lib/modelPicker'
import { modelSuggestionFor, modelSuggestionLabel } from '@/lib/modelSuggestion'

export function ModelSelect({
  value,
  effort,
  suggestions,
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
  loading?: boolean
  disabled?: boolean
  fastMode?: FastModeControl
  placement?: 'above' | 'below'
  align?: 'start' | 'end'
  onChange: (selection: ModelSelection) => void
}) {
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const reduceMotion = useReducedMotion()
  const selected = modelSuggestionFor(suggestions, value)
  const label = value ? modelSuggestionLabel(suggestions, value) : 'Model'
  const fast = fastMode?.checked === true
  const height = Math.min(224, Math.max(1, suggestions.length) * 28 + (fastMode ? 36 : 0))

  const selectModel = (model: ModelPickerOption) => {
    onChange({
      model: model.value,
      effort: model.reasoning && !model.reasoning.efforts?.includes(effort) ? model.reasoning.default_effort ?? '' : effort,
    })
    setOpen(false)
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
          aria-label={`Model: ${label}${fast ? ', Fast Mode' : ''}`}
          title={`${label}${fast ? ' · Fast Mode' : ''}`}
          disabled={disabled}
          onClick={() => setOpen(!open)}
        >
          <span className="truncate">{label}</span>
          {fast ? <Zap size={12} fill="currentColor" className="shrink-0 text-primary" /> : null}
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      <motion.div
        role="dialog"
        aria-label="Model"
        className="model-picker w-[256px] max-w-[calc(100vw-32px)] overflow-hidden"
        initial={false}
        animate={{ height }}
        transition={{ duration: reduceMotion ? 0 : 0.2, ease: [0.2, 0, 0, 1] }}
      >
        {fastMode ? (
          <div className="flex h-9 items-center justify-between px-2.5">
            <span className="text-[12px] text-ink-2">Fast Mode</span>
            <button
              type="button"
              role="switch"
              aria-label="Fast Mode"
              aria-checked={fast}
              disabled={fastMode.disabled}
              onClick={() => fastMode.onChange(!fast)}
              className={`inline-flex h-6 items-center gap-1 rounded-full px-2 text-[12px] font-medium transition-colors disabled:opacity-50 ${fast
                ? 'bg-primary text-on-primary'
                : 'text-ink-3 hover:bg-surface-2 hover:text-ink'}`}
            >
              <Zap size={12} fill={fast ? 'currentColor' : 'none'} />
              {fast ? 'On' : 'Off'}
            </button>
          </div>
        ) : null}
        <div
          role="menu"
          aria-label="Models"
          className="max-h-56 overflow-y-auto"
          onKeyDown={(event) => {
            if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
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
              {loading ? <LoaderCircle size={13} className="shrink-0 animate-spin text-ink-3" /> : null}
              {model.value === selected?.value ? <Check size={13} className="shrink-0 text-ink-3" /> : null}
            </button>
          ))}
          {!suggestions.length ? <div className="h-7 px-2.5 text-[12px] leading-7 text-ink-3">{loading ? 'Loading models…' : 'No models available'}</div> : null}
        </div>
      </motion.div>
    </Popover>
  )
}
