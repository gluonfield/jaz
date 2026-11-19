import { Check } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { layoutRect } from '@/lib/dom/zoom'
import { filterModelSuggestions, modelSuggestionLabel, type ModelSuggestion } from '@/lib/models'

export function ModelCombobox({
  value,
  suggestions,
  placeholder = 'Default',
  allowDefault = true,
  disabled,
  onChange,
  className = '',
  'aria-label': ariaLabel,
}: {
  value: string
  suggestions: ModelSuggestion[]
  placeholder?: string
  allowDefault?: boolean
  disabled?: boolean
  onChange: (value: string) => void
  className?: string
  'aria-label'?: string
}) {
  const [open, setOpen] = useState(false)
  const [rect, setRect] = useState<DOMRect | null>(null)
  // Show the full list when the menu opens on focus; filter once the user types.
  const [filter, setFilter] = useState<string | null>(null)
  const [activeIndex, setActiveIndex] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const reduce = useReducedMotion()

  const choices = allowDefault ? [{ value: '', label: placeholder, reasoning: { status: 'unavailable' as const } }, ...suggestions] : suggestions
  const filtered = filter === null ? choices : filterModelSuggestions(choices, filter)
  const hasMenu = suggestions.length > 0

  const openMenu = () => {
    if (!hasMenu) return
    setFilter(null)
    setActiveIndex(-1)
    setOpen(true)
  }
  const closeMenu = () => {
    setOpen(false)
    setActiveIndex(-1)
  }

  useEffect(() => {
    if (disabled) {
      setOpen(false)
      setActiveIndex(-1)
    }
  }, [disabled])

  useLayoutEffect(() => {
    if (open && inputRef.current) setRect(layoutRect(inputRef.current))
  }, [open])

  useEffect(() => {
    if (!open) return
    const close = () => closeMenu()
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (inputRef.current?.contains(t) || menuRef.current?.contains(t)) return
      closeMenu()
    }
    // Scrolling inside the menu is navigation; only outside scrolls (which
    // would misposition the fixed-anchored menu) close it.
    const onScroll = (e: Event) => {
      if (menuRef.current?.contains(e.target as Node)) return
      closeMenu()
    }
    document.addEventListener('mousedown', onDown)
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('mousedown', onDown)
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', close)
    }
  }, [open])

  return (
    <>
      <input
        ref={inputRef}
        value={open && filter !== null ? filter : modelSuggestionLabel(suggestions, value)}
        disabled={disabled}
        placeholder={placeholder}
        aria-label={ariaLabel}
        aria-expanded={open && !disabled}
        role="combobox"
        onFocus={openMenu}
        onClick={() => {
          if (!open) openMenu()
        }}
        onChange={(e) => {
          setFilter(e.target.value)
          setActiveIndex(-1)
          if (!open && hasMenu) setOpen(true)
        }}
        onKeyDown={(e) => {
          switch (e.key) {
            case 'ArrowDown':
              e.preventDefault()
              if (!open) openMenu()
              else setActiveIndex((i) => Math.min(i + 1, filtered.length - 1))
              break
            case 'ArrowUp':
              e.preventDefault()
              setActiveIndex((i) => Math.max(i - 1, -1))
              break
            case 'Enter':
              if (open && activeIndex >= 0 && filtered[activeIndex]) {
                e.preventDefault()
                onChange(filtered[activeIndex].value)
                closeMenu()
              }
              break
            case 'Escape':
              if (open) {
                e.stopPropagation()
                closeMenu()
              }
              break
          }
        }}
        className={`h-7 rounded-full bg-ink/10 px-3 text-[12px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25 disabled:opacity-50 ${className}`}
      />
      {open && !disabled && rect && hasMenu ? createPortal(
        <motion.div
          ref={menuRef}
          role="listbox"
          aria-label={ariaLabel}
          initial={{ opacity: 0, y: reduce ? 0 : -4 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.12, ease: 'easeOut' }}
          style={{
            position: 'fixed',
            top: rect.bottom + 4,
            left: rect.left,
            minWidth: rect.width,
            maxWidth: Math.max(rect.width, 320),
            zIndex: 'var(--z-modal)',
          }}
          className="max-h-[280px] overflow-y-auto rounded-[8px] bg-surface p-1 shadow-xl ring-1 ring-border"
        >
          {filtered.length > 0 ? (
            filtered.map((s, index) => {
              const selected = s.value === value
              return (
                <button
                  key={s.value}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  tabIndex={-1}
                  // mousedown beats the input blur/outside-click close
                  onMouseDown={(e) => {
                    e.preventDefault()
                    onChange(s.value)
                    closeMenu()
                  }}
                  onMouseEnter={() => setActiveIndex(index)}
                  className={`flex w-full items-start gap-2 rounded-[6px] px-2 py-1 text-left transition-colors duration-150 hover:bg-surface-2 ${
                    index === activeIndex ? 'bg-surface-2' : ''
                  } ${selected ? 'text-ink' : 'text-ink-2'}`}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[12px]">{s.label}</span>
                    {s.description ? (
                      <span className="mt-0.5 block truncate text-[11px] text-ink-3">
                        {s.description}
                      </span>
                    ) : null}
                  </span>
                  {selected ? <Check size={13} className="mt-0.5 shrink-0 text-ink" /> : null}
                </button>
              )
            })
          ) : (
            <div className="px-2 py-1 text-[12px] text-ink-3">No matching models.</div>
          )}
        </motion.div>,
        document.body,
      ) : null}
    </>
  )
}
