import { ChevronDown, Gauge } from 'lucide-react'
import { useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { Button } from '@/components/ui/Button'

// '' is the agent/server default; the rest map to provider reasoning efforts.
export const REASONING_EFFORT_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'Default' },
  { value: 'minimal', label: 'Minimal' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra high' },
]

export function reasoningEffortLabel(value: string | undefined): string {
  return (
    REASONING_EFFORT_OPTIONS.find((option) => option.value === (value ?? ''))?.label ?? 'Default'
  )
}

// Picks the per-loop reasoning effort. An empty value inherits the agent default.
export function ReasoningEffortSelect({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  const [open, setOpen] = useState(false)
  const label = reasoningEffortLabel(value)
  const select = (next: string) => {
    onChange(next)
    setOpen(false)
  }
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      trigger={
        <Button
          variant="secondary"
          size="md"
          className="max-w-[12rem]"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={`Reasoning effort: ${label}`}
          title={`Reasoning effort: ${label}`}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <Gauge size={13} className="shrink-0" />
          <span className="truncate">{label}</span>
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      {REASONING_EFFORT_OPTIONS.map((option) => (
        <MenuRow
          key={option.value || 'default'}
          selected={(value ?? '') === option.value}
          onClick={() => select(option.value)}
        >
          {option.label}
        </MenuRow>
      ))}
    </Popover>
  )
}
