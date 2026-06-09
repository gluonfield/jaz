import { Check, ChevronDown } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

export type SelectOption = { value: string; label: string; description?: string }

// Codex-style dropdown: a filled trigger chip and a portalled popover menu
// (rows with a check on the selected item and optional descriptions). Portalled
// to the body so the menu escapes the `overflow-hidden` settings cards.
export function Select({
  value,
  options,
  onChange,
  disabled,
  className = '',
  'aria-label': ariaLabel,
}: {
  value: string
  options: SelectOption[]
  onChange: (value: string) => void
  disabled?: boolean
  className?: string
  'aria-label'?: string
}) {
  const [open, setOpen] = useState(false)
  const [rect, setRect] = useState<DOMRect | null>(null)
  const listboxId = useId()
  const [activeIndex, setActiveIndex] = useState(0)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const reduce = useReducedMotion()
  const selectedIndex = Math.max(
    0,
    options.findIndex((o) => o.value === value),
  )
  const current = options[selectedIndex] ?? options[0]

  const closeMenu = (restoreFocus = true) => {
    setOpen(false)
    if (restoreFocus) requestAnimationFrame(() => triggerRef.current?.focus())
  }

  const openMenu = (index = selectedIndex) => {
    setActiveIndex(Math.min(Math.max(index, 0), Math.max(options.length - 1, 0)))
    setOpen(true)
  }

  const chooseOption = (index: number) => {
    const option = options[index]
    if (!option) return
    onChange(option.value)
    closeMenu()
  }

  useLayoutEffect(() => {
    if (open && triggerRef.current) setRect(triggerRef.current.getBoundingClientRect())
  }, [open])

  useEffect(() => {
    if (!open) return
    requestAnimationFrame(() => optionRefs.current[activeIndex]?.focus())
  }, [activeIndex, open])

  useEffect(() => {
    if (!open) return
    const close = () => closeMenu(false)
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (triggerRef.current?.contains(t) || menuRef.current?.contains(t)) return
      closeMenu(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeMenu()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    // The menu is fixed-positioned to the trigger; close rather than chase it on scroll/resize.
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
    }
  }, [open])

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listboxId : undefined}
        aria-label={ariaLabel}
        onClick={() => {
          if (open) {
            closeMenu(false)
          } else {
            openMenu()
          }
        }}
        onKeyDown={(e) => {
          switch (e.key) {
            case 'ArrowDown':
              e.preventDefault()
              openMenu(open ? activeIndex + 1 : selectedIndex)
              break
            case 'ArrowUp':
              e.preventDefault()
              openMenu(open ? activeIndex - 1 : selectedIndex)
              break
            case 'Enter':
            case ' ':
              e.preventDefault()
              openMenu()
              break
          }
        }}
        className={`flex h-7 items-center justify-between gap-2 rounded-control bg-ink/10 px-2.5 text-[12px] text-ink transition-colors duration-150 hover:bg-ink/15 disabled:cursor-default disabled:opacity-50 ${className}`}
      >
        <span className="truncate">{current?.label}</span>
        <ChevronDown size={13} className="shrink-0 text-ink-3" />
      </button>
      {createPortal(
        <AnimatePresence>
          {open && rect ? (
            <motion.div
              ref={menuRef}
              id={listboxId}
              role="listbox"
              aria-label={ariaLabel}
              onKeyDown={(e) => {
                switch (e.key) {
                  case 'ArrowDown':
                    e.preventDefault()
                    setActiveIndex((index) => Math.min(index + 1, options.length - 1))
                    break
                  case 'ArrowUp':
                    e.preventDefault()
                    setActiveIndex((index) => Math.max(index - 1, 0))
                    break
                  case 'Home':
                    e.preventDefault()
                    setActiveIndex(0)
                    break
                  case 'End':
                    e.preventDefault()
                    setActiveIndex(Math.max(options.length - 1, 0))
                    break
                  case 'Enter':
                  case ' ':
                    e.preventDefault()
                    chooseOption(activeIndex)
                    break
                }
              }}
              initial={{ opacity: 0, y: reduce ? 0 : -4 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: reduce ? 0 : -4 }}
              transition={{ duration: 0.12, ease: 'easeOut' }}
              style={{
                position: 'fixed',
                top: rect.bottom + 4,
                left: rect.left,
                minWidth: rect.width,
                zIndex: 60,
              }}
              className="max-h-[280px] overflow-y-auto rounded-[8px] bg-surface p-1 shadow-xl ring-1 ring-border"
            >
              {options.map((option, index) => {
                const selected = option.value === value
                const active = index === activeIndex
                return (
                  <button
                    key={option.value || 'default'}
                    ref={(node) => {
                      optionRefs.current[index] = node
                    }}
                    type="button"
                    role="option"
                    aria-selected={selected}
                    tabIndex={active ? 0 : -1}
                    onFocus={() => setActiveIndex(index)}
                    onClick={() => {
                      chooseOption(index)
                    }}
                    className={`flex w-full items-start gap-2 rounded-[6px] px-2 py-1 text-left transition-colors duration-150 hover:bg-surface-2 ${
                      active ? 'bg-surface-2' : ''
                    } ${selected ? 'text-ink' : 'text-ink-2'}`}
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[12px]">{option.label}</span>
                      {option.description ? (
                        <span className="mt-0.5 block text-[11px] text-ink-3">{option.description}</span>
                      ) : null}
                    </span>
                    {selected ? <Check size={13} className="mt-0.5 shrink-0 text-ink" /> : null}
                  </button>
                )
              })}
            </motion.div>
          ) : null}
        </AnimatePresence>,
        document.body,
      )}
    </>
  )
}
