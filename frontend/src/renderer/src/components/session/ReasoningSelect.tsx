import { Select } from '@/components/ui/Select'
import type { ReasoningEffortOption } from '@/lib/api/types'
import type { ModelReasoningSelection } from '@/lib/reasoningEfforts'

export function ReasoningSelect({
  value,
  options,
  disabled,
  labeled = true,
  status,
  onChange,
}: {
  value: string
  options: ReasoningEffortOption[]
  disabled?: boolean
  labeled?: boolean
  status?: ModelReasoningSelection['status']
  onChange: (value: string) => void
}) {
  if (options.length < 2) {
    if (!status) return null
    const label = status === 'pending' ? 'Loading controls…'
      : status === 'error' ? 'Controls unavailable'
      : status === 'unknown' ? 'Controls not reported'
      : 'Model default'
    return <span className="text-[12px] text-ink-3" role="status">{labeled ? 'Reasoning: ' : ''}{label}</span>
  }
  return (
    <div className="inline-flex min-w-0 items-center gap-1.5">
      {labeled ? <span className="text-[12px] text-ink-3">Reasoning</span> : null}
      <Select
        aria-label="Reasoning effort"
        value={value}
        options={options}
        disabled={disabled}
        onChange={onChange}
      />
    </div>
  )
}
