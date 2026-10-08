import { Zap } from 'lucide-react'
import { Select } from '@/components/ui/Select'
import { SwitchRow } from '@/components/ui/Switch'
import { fastModeOption, type FastModeControl } from '@/lib/agentConfig'
import type { AgentSessionConfigOption } from '@/lib/api/types'

export function FastModeOption({ checked, disabled, onChange }: FastModeControl) {
  return <SwitchRow icon={<Zap size={13} className="shrink-0" />} label="Fast Mode" checked={checked} disabled={disabled} onChange={onChange} />
}

export function NativeModelOptions({ options, running, pending, onChange }: {
  options?: AgentSessionConfigOption[] | null
  running: boolean
  pending: boolean
  onChange: (id: string, value: string) => void
}) {
  const fastMode = fastModeOption(options)
  return options?.filter((option) => option.category === 'model_config').map((option) => (
    option === fastMode ? <FastModeOption
      key={option.id}
      checked={option.current_value === 'on'}
      disabled={pending}
      onChange={(checked) => onChange(option.id, checked ? 'on' : 'off')}
    /> : <div key={option.id} className="px-2.5 py-1">
      <div className="flex min-h-10 items-center justify-between gap-3">
        <span className="text-[13px] text-ink-2">{option.name}</span>
        <Select
          aria-label={option.name}
          value={option.current_value}
          options={option.options.map((value) => ({ value: value.value, label: value.name }))}
          disabled={running || pending}
          onChange={(value) => onChange(option.id, value)}
        />
      </div>
      {option.description ? <p className="max-w-64 text-[11px] text-ink-3">{option.description}</p> : null}
    </div>
  ))
}
