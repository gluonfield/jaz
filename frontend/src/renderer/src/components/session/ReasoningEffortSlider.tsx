import type { ReasoningEffortOption } from '@/lib/api/types'

// Width of the native range thumb; dots are laid out on the same rail the
// thumb's center travels, so the two stay aligned at every stop.
const THUMB = 24

function stopPosition(index: number, count: number): string {
  if (count <= 1) return '50%'
  return `calc(${THUMB / 2}px + ${index / (count - 1)} * (100% - ${THUMB}px))`
}

// Discrete effort slider: one stop per level the model and agent support,
// fastest on the left, smartest on the right.
export function ReasoningEffortSlider({
  options,
  value,
  defaultValue,
  showDefaultReset,
  onChange,
}: {
  // Concrete stops, ordered fastest → smartest ('' never appears here).
  options: ReasoningEffortOption[]
  // '' inherits the configured default; the thumb then parks on defaultValue.
  value: string
  defaultValue?: string
  showDefaultReset?: boolean
  onChange: (effort: string) => void
}) {
  const selected = value || defaultValue || ''
  let index = options.findIndex((option) => option.value === selected)
  if (index < 0) index = Math.floor((options.length - 1) / 2)

  return (
    <div className="px-2.5 pb-2">
      <div className="flex items-baseline justify-between pt-1">
        <p className="text-[12px] text-ink-3">
          Effort <span className="font-medium text-ink">{options[index]?.label ?? 'Default'}</span>
        </p>
        {showDefaultReset ? (
          <button
            type="button"
            onClick={() => onChange('')}
            className={`rounded-full px-1.5 text-[11px] transition-colors duration-150 ${
              value === '' ? 'text-primary' : 'text-ink-3 hover:text-ink'
            }`}
          >
            Default
          </button>
        ) : null}
      </div>
      <div className="flex items-baseline justify-between text-[11px] text-ink-3">
        <span>Faster</span>
        <span>Smarter</span>
      </div>
      <div className="relative mt-1.5 h-7">
        <div className="absolute inset-0 rounded-full bg-ink/10" />
        {options.map((option, i) => (
          <span
            key={option.value}
            className={`absolute top-1/2 size-[5px] -translate-x-1/2 -translate-y-1/2 rounded-full ${
              option.value === 'ultracode' ? 'bg-primary' : 'bg-ink/25'
            }`}
            style={{ left: stopPosition(i, options.length) }}
          />
        ))}
        <input
          type="range"
          min={0}
          max={options.length - 1}
          step={1}
          value={index}
          aria-label="Reasoning effort"
          aria-valuetext={options[index]?.label}
          onChange={(e) => onChange(options[Number(e.target.value)]?.value ?? '')}
          className="absolute inset-0 w-full cursor-pointer appearance-none bg-transparent outline-none
            [&::-webkit-slider-thumb]:h-5 [&::-webkit-slider-thumb]:w-6 [&::-webkit-slider-thumb]:appearance-none
            [&::-webkit-slider-thumb]:rounded-[10px] [&::-webkit-slider-thumb]:bg-ink/35
            [&::-webkit-slider-thumb]:shadow-[0_1px_2px_rgba(0,0,0,0.25)]
            [&::-webkit-slider-thumb]:transition-colors [&::-webkit-slider-thumb]:duration-150
            hover:[&::-webkit-slider-thumb]:bg-ink/45
            [&::-moz-range-thumb]:h-5 [&::-moz-range-thumb]:w-6 [&::-moz-range-thumb]:appearance-none
            [&::-moz-range-thumb]:rounded-[10px] [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-ink/35"
        />
      </div>
    </div>
  )
}
