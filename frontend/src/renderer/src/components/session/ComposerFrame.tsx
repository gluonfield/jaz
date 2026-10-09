import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ComponentProps, useEffect, useRef, useState } from 'react'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { useEffectsEnabled } from '@/lib/appearance'

// Borderless card, agent-council style: the surface tone IS the card. While
// focused a rainbow conic ring circles it; with effects off the ring is a calm
// static border that never animates. A chat card is a messenger pill whose end
// cap is concentric with its 40px row of controls.
export function ComposerFrame({ chat = false, className = '', ...card }: { chat?: boolean } & ComponentProps<'div'>) {
  const ref = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState(false)
  const reducedMotion = useReducedMotion()
  const effectsEnabled = useEffectsEnabled()
  const radius = chat ? 26 : 12
  // autoFocus lands before React's focus listeners attach; sync the ring state.
  useEffect(() => {
    if (ref.current?.contains(document.activeElement)) setFocused(true)
  }, [])
  return (
    <div
      ref={ref}
      className="relative"
      onFocusCapture={() => setFocused(true)}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false)
      }}
    >
      <AnimatePresence>
        {focused && effectsEnabled ? (
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
              className="absolute -inset-[4px] opacity-50 blur-[10px]"
              style={{ background: RAINBOW_BEAM, borderRadius: radius + 6 }}
            />
            {/* the comet itself; the card's opaque surface covers the center */}
            <div className="absolute inset-0" style={{ background: RAINBOW_BEAM, borderRadius: radius + 2 }} />
          </motion.div>
        ) : null}
      </AnimatePresence>
      <div
        {...card}
        style={{ borderRadius: radius }}
        className={`relative bg-surface transition-shadow ${chat ? 'px-2.5 py-1.5' : 'p-2.5'} ${
          effectsEnabled ? '' : focused ? 'ring-2 ring-primary' : 'ring-1 ring-border'
        } ${className}`}
      />
    </div>
  )
}
