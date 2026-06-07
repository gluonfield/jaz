import { ArrowUp, Square } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'

// A rainbow comet (~100° arc fading in and out of transparency) that orbits
// the card while focused; the rest of the perimeter stays a quiet track.
const RAINBOW_BEAM =
  'conic-gradient(from var(--ring-angle, 0deg), transparent 0deg 250deg, var(--color-rainbow-1) 278deg, var(--color-rainbow-2) 296deg, var(--color-rainbow-3) 312deg, var(--color-rainbow-4) 326deg, var(--color-rainbow-5) 340deg, transparent 352deg 360deg)'

// Composer in the agent-council style: borderless auto-growing textarea on a
// raised card, toolbar row beneath with the send/stop action. The card is the
// focus surface — while focused, a rainbow conic ring circles the card.
export function ComposerCard({
  streaming,
  autoFocus,
  placeholder = 'Message your assistant…',
  disabled = false,
  onSend,
  onStop,
}: {
  streaming: boolean
  autoFocus?: boolean
  placeholder?: string
  disabled?: boolean
  onSend: (text: string) => void
  onStop?: () => void
}) {
  const [text, setText] = useState('')
  const [focused, setFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const reducedMotion = useReducedMotion()

  // autoFocus lands before React's focus listeners attach; sync the initial state.
  useEffect(() => {
    if (document.activeElement === textareaRef.current) setFocused(true)
  }, [])

  const submit = () => {
    const trimmed = text.trim()
    if (!trimmed || streaming || disabled) return
    onSend(trimmed)
    setText('')
    const el = textareaRef.current
    if (el) {
      el.style.height = 'auto'
      el.focus()
    }
  }

  return (
    <div
      className="relative"
      onFocusCapture={() => setFocused(true)}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false)
      }}
    >
      <AnimatePresence>
        {focused ? (
          <motion.div
            key="ring"
            aria-hidden
            className="pointer-events-none absolute -inset-[2px]"
            initial={{ opacity: 0 }}
            animate={{
              opacity: 1,
              ...(reducedMotion ? {} : { '--ring-angle': ['0deg', '360deg'] }),
            }}
            exit={{ opacity: 0 }}
            transition={{
              opacity: { duration: 0.25, ease: 'easeOut' },
              '--ring-angle': { duration: 2.6, ease: 'linear', repeat: Infinity },
            }}
          >
            {/* glow trailing the comet, bleeding softly outside the card */}
            <div
              className="absolute -inset-[4px] rounded-[18px] opacity-50 blur-[10px]"
              style={{ background: RAINBOW_BEAM }}
            />
            {/* the comet itself; the card's opaque surface covers the center */}
            <div className="absolute inset-0 rounded-[14px]" style={{ background: RAINBOW_BEAM }} />
          </motion.div>
        ) : null}
      </AnimatePresence>

      {/* borderless card, agent-council style: the surface tone IS the card.
          The whole card is a click target for the textarea. */}
      <div
        className="relative flex cursor-text flex-col gap-1.5 rounded-[12px] bg-surface p-2.5"
        onClick={(e) => {
          if ((e.target as HTMLElement).closest('button, textarea')) return
          textareaRef.current?.focus()
        }}
      >
        <textarea
          ref={textareaRef}
          value={text}
          rows={1}
          autoFocus={autoFocus}
          disabled={disabled}
          placeholder={placeholder}
          className="max-h-[200px] min-h-[30px] w-full resize-none bg-transparent px-2 pt-1.5 pb-0.5 text-sm leading-relaxed text-ink select-text placeholder:text-ink-3 disabled:cursor-default"
          onChange={(e) => {
            setText(e.target.value)
            e.target.style.height = 'auto'
            e.target.style.height = `${Math.min(e.target.scrollHeight, 200)}px`
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              submit()
            }
          }}
        />
        <div className="flex items-center justify-end gap-2.5">
          {streaming && onStop ? (
            <motion.button
              type="button"
              aria-label="Stop response"
              title="Stop response"
              onClick={onStop}
              whileTap={{ scale: 0.92 }}
              className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-full bg-bg text-ink shadow-sm transition-colors duration-150 hover:bg-surface-2"
            >
              <Square size={13} fill="currentColor" strokeWidth={0} />
            </motion.button>
          ) : (
            <motion.button
              type="button"
              aria-label="Send message"
              title="Send message"
              disabled={!text.trim() || streaming || disabled}
              onClick={submit}
              whileTap={{ scale: 0.92 }}
              className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-full bg-primary text-white shadow-sm transition-colors duration-150 hover:bg-primary-strong disabled:cursor-default disabled:bg-bg disabled:text-ink-3 disabled:shadow-none"
            >
              <ArrowUp size={18} />
            </motion.button>
          )}
        </div>
      </div>
    </div>
  )
}

// Bottom dock for the session view: content scrolls beneath a fade into the
// page background; only the card itself receives pointer events.
export function Composer({
  streaming,
  disabled,
  placeholder,
  onSend,
  onStop,
}: {
  streaming: boolean
  disabled?: boolean
  placeholder?: string
  onSend: (text: string) => void
  onStop: () => void
}) {
  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-b from-transparent to-bg to-45% px-10 pt-6 pb-5">
      <motion.div
        className="pointer-events-auto mx-auto max-w-[640px]"
        initial={{ y: 12, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ type: 'spring', stiffness: 380, damping: 32 }}
      >
        <ComposerCard
          streaming={streaming}
          disabled={disabled}
          placeholder={placeholder}
          onSend={onSend}
          onStop={onStop}
        />
      </motion.div>
    </div>
  )
}
