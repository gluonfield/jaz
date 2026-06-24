import { useReducedMotion } from 'motion/react'
import { useEffectsEnabled } from './appearance'

export function useReducedEffectsMotion(): boolean {
  const reducedMotion = useReducedMotion()
  const effectsEnabled = useEffectsEnabled()
  return Boolean(reducedMotion) || !effectsEnabled
}
