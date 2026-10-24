import { AnimatePresence, motion, useAnimate } from 'motion/react'
import { type ReactNode, useEffect, useRef } from 'react'
import { DitherArt, type Silhouette } from '@/components/launch/DitherArt'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { type OnboardingStep, onboardingEase, slideExit, slideRise, slideStagger } from './OnboardingParts'

const NO_MOTIF: Silhouette = () => {}

// A step change plays in three acts while the footer stays put: the slide's
// copy and controls leave bottom-up, the pixel mark rises to centre stage and
// swirls into the next step's mark, then it settles back and the new slide
// rises in under it. A slide without a mark lets the swirl collapse to
// nothing before its own content arrives. The mark floats over the stage and
// each slide reserves its room, so outgoing and incoming slides never stack
// the mark's height onto the wrong content.
const LIFT = { duration: 1.4, times: [0, 0.2, 0.75, 1], ease: onboardingEase }

const content = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.85 } },
  out: { transition: { staggerChildren: 0.035, staggerDirection: -1 } },
}

export function OnboardingStage({
  step,
  motif,
  children,
  footer,
}: {
  step: OnboardingStep
  motif?: Silhouette
  children: ReactNode
  footer: ReactNode
}) {
  const reduced = useReducedEffectsMotion()
  const stage = useRef<HTMLDivElement>(null)
  const [hero, animateHero] = useAnimate<HTMLDivElement>()
  const shown = useRef(step)
  useEffect(() => {
    if (shown.current === step) return
    shown.current = step
    if (reduced) return
    const lift = stage.current!.clientHeight / 2 - hero.current.offsetTop - hero.current.offsetHeight / 2
    animateHero(hero.current, { y: [0, lift, lift, 0], scale: [1, 1.4, 1.4, 1] }, LIFT)
  }, [step, reduced, hero, animateHero])

  return (
    <motion.div
      ref={stage}
      variants={slideStagger}
      initial="hidden"
      animate="show"
      exit={slideExit}
      className="relative flex min-h-[min(640px,calc(100vh-190px))] w-full max-w-[460px] flex-col items-center"
    >
      <motion.div
        initial={false}
        animate={{ opacity: motif ? 1 : 0 }}
        transition={{ duration: 0.4, ease: onboardingEase, delay: motif ? 0 : 0.95 }}
        className="pointer-events-none absolute inset-x-0 top-0 z-10"
      >
        <div ref={hero}>
          <DitherArt draw={motif ?? NO_MOTIF} cols={104} rows={34} delay={0.25} buildKey="onboarding-hero" />
        </div>
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
          {motif && <div className="aspect-[415/135] w-full max-w-[415px]" />}
          {children}
        </motion.div>
      </AnimatePresence>
      <motion.div variants={slideRise} className="mt-auto w-full">
        {footer}
      </motion.div>
    </motion.div>
  )
}
