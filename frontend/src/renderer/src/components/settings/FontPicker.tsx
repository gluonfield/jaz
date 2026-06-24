import { Check, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { Popover } from '@/components/ui/Popover'
import { useSystemFonts } from '@/lib/systemFonts'

// Shown when the Local Font Access API is unavailable or denied, so the picker
// still offers sensible choices.
const BUILTIN_UI = ['system-ui', 'SF Pro Text', 'Helvetica Neue', 'Arial', 'Georgia', 'IBM Plex Sans']
const BUILTIN_MONO = ['SF Mono', 'Menlo', 'Monaco', 'Consolas', 'Fira Code', 'JetBrains Mono', 'IBM Plex Mono']

const MAX_VISIBLE = 400

// A font field that lists the fonts actually installed on the machine, each row
// previewed in its own face. The text input stays the source of truth, so any
// family name can still be typed even if it isn't enumerated.
export function FontPicker({
  value,
  placeholder,
  ariaLabel,
  monospaceOnly = false,
  onChange,
}: {
  value: string
  placeholder: string
  ariaLabel: string
  monospaceOnly?: boolean
  onChange: (value: string) => void
}) {
  const [open, setOpen] = useState(false)
  const { fonts, load } = useSystemFonts()

  const pool = monospaceOnly
    ? fonts.mono.length
      ? fonts.mono
      : BUILTIN_MONO
    : fonts.all.length
      ? fonts.all
      : BUILTIN_UI

  const query = value.trim().toLowerCase()
  const matches = query ? pool.filter((font) => font.toLowerCase().includes(query)) : pool
  const visible = matches.slice(0, MAX_VISIBLE)
  const previewFallback = monospaceOnly ? 'ui-monospace, monospace' : 'ui-sans-serif, sans-serif'

  return (
    <div className="flex items-center gap-1.5">
      <Popover
        open={open && visible.length > 0}
        onClose={() => setOpen(false)}
        placement="below"
        align="start"
        trigger={
          <input
            type="text"
            value={value}
            placeholder={placeholder}
            aria-label={ariaLabel}
            spellCheck={false}
            autoComplete="off"
            onFocus={() => {
              load()
              setOpen(true)
            }}
            onChange={(event) => {
              onChange(event.target.value)
              setOpen(true)
            }}
            onKeyDown={(event) => {
              if (event.key === 'Escape') setOpen(false)
            }}
            className="h-8 w-[210px] max-w-full rounded-[10px] bg-bg px-2.5 text-[13px] text-ink ring-1 ring-border outline-none transition duration-150 placeholder:text-ink-3 focus:ring-2 focus:ring-primary"
          />
        }
      >
        <div className="w-[230px]">
          <p className="px-2 pt-0.5 pb-1 text-[11px] text-ink-3">
            {monospaceOnly ? 'Monospace fonts' : 'Installed fonts'}
            {matches.length > MAX_VISIBLE ? ` · ${MAX_VISIBLE}+` : ` · ${matches.length}`}
          </p>
          <div className="max-h-[252px] overflow-y-auto">
            {visible.map((font) => (
              <button
                key={font}
                type="button"
                // Keep input focus so selecting doesn't blur-close before onClick.
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  onChange(font)
                  setOpen(false)
                }}
                className="flex w-full items-center gap-2 rounded-[8px] px-2 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
                style={{ fontFamily: `"${font.replace(/"/g, '')}", ${previewFallback}` }}
                title={font}
              >
                <span className="min-w-0 flex-1 truncate">{font}</span>
                {font === value ? <Check size={13} className="shrink-0 text-primary" /> : null}
              </button>
            ))}
          </div>
        </div>
      </Popover>
      {value ? (
        <button
          type="button"
          aria-label={`Reset ${ariaLabel.toLowerCase()}`}
          title="Reset to default"
          onClick={() => onChange('')}
          className="grid size-8 shrink-0 place-items-center rounded-[10px] text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <RotateCcw size={14} />
        </button>
      ) : null}
    </div>
  )
}
