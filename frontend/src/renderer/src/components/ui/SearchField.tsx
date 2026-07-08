import { Search, X } from 'lucide-react'
import { IconButton } from './IconButton'

// The pill search input used across settings surfaces. Height and
// responsive tweaks come from the caller via `className`.
export function SearchField({
  value,
  onChange,
  placeholder,
  className = '',
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
  className?: string
}) {
  return (
    <div className="relative">
      <Search
        size={14}
        className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-3"
      />
      <input
        type="text"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        aria-label={placeholder.replace(/…$/, '')}
        className={`w-full rounded-full bg-ink/10 pl-8 text-[13px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25 ${value ? 'pr-9' : 'pr-3'} ${className}`}
      />
      {value ? (
        <IconButton
          size="xs"
          aria-label="Clear search"
          onClick={() => onChange('')}
          className="absolute right-1.5 top-1/2 -translate-y-1/2"
        >
          <X size={12} />
        </IconButton>
      ) : null}
    </div>
  )
}
