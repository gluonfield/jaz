import { AnimatePresence, motion } from 'motion/react'
import type { ReactNode } from 'react'
import { DitherArt, MORPH_SECONDS, ditherSize } from '@/components/launch/DitherArt'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { type Mark, type OnboardingStep, onboardingEase, slideExit, slideRise, slideStagger } from './OnboardingParts'

const HERO = { cols: 104, rows: 34, delay: 0.2 }
const HERO_SIZE = ditherSize(HERO.cols, HERO.rows)

// A slide without a mark draws nothing, so the old mark's dots all burst away.
const NO_MARK: Mark = { draw: () => {}, enter: (x, y) => ({ x, y, at: 0 }) }

// The new slide starts rising in as the mark lands.
const ARRIVAL = HERO.delay + MORPH_SECONDS - 0.2

// A step change plays in three acts while the mark and footer stay put: the
// slide's copy and controls leave bottom-up, the old mark's dots rebuild the
// next mark through its own entrance, then the new slide rises in under it.
// The mark floats over the stage and each slide reserves its room, so
// outgoing and incoming slides never stack the mark's height onto the wrong
// content.
const content = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: ARRIVAL } },
  out: { transition: { staggerChildren: 0.035, staggerDirection: -1 } },
}

export function OnboardingStage({
  step,
  mark,
  children,
  footer,
}: {
  step: OnboardingStep
  mark?: Mark
  children: ReactNode
  footer: ReactNode
}) {
  const reduced = useReducedEffectsMotion()
  const { draw, enter } = mark ?? NO_MARK

  return (
    <motion.div
      variants={slideStagger}
      initial="hidden"
      animate="show"
      exit={slideExit}
      className="relative flex min-h-[min(640px,calc(100vh-190px))] w-full max-w-[460px] flex-col items-center"
    >
      <motion.div
        initial={false}
        animate={{ opacity: mark ? 1 : 0 }}
        transition={{ duration: 0.4, ease: onboardingEase, delay: mark ? 0 : ARRIVAL }}
        className="pointer-events-none absolute inset-x-0 top-0 z-10"
      >
        <DitherArt draw={draw} enter={enter} cols={HERO.cols} rows={HERO.rows} delay={HERO.delay} buildKey="onboarding-hero" />
      </motion.div>
      <AnimatePresence mode="wait">
        <motion.div
          key={step}
          variants={reduced ? slideStagger : content}
          initial="hidden"
          animate="show"
          exit="out"
          className="flex w-full flex-col items-center"
        >
          {mark && (
            <div className="w-full" style={{ maxWidth: HERO_SIZE.width, aspectRatio: `${HERO_SIZE.width} / ${HERO_SIZE.height}` }} />
          )}
          {children}
        </motion.div>
      </AnimatePresence>
      <motion.div variants={slideRise} className="mt-auto w-full">
        {footer}
      </motion.div>
    </motion.div>
  )
}
