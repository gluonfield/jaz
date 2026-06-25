import { useEffect, useState } from 'react'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from './useWindowEvent'

export function useMetaHeld(enabled = true): boolean {
  const [held, setHeld] = useState(false)
  useEffect(() => {
    if (!enabled) setHeld(false)
  }, [enabled])
  useWindowEvent('keydown', (e) => {
    if (modalDialogOpen()) setHeld(false)
    else if (e.key === 'Meta' || e.metaKey) setHeld(true)
  }, enabled)
  useWindowEvent('keyup', (e) => {
    if (e.key === 'Meta') setHeld(false)
  }, enabled)
  useWindowEvent('blur', () => setHeld(false), enabled)
  return enabled && held
}
