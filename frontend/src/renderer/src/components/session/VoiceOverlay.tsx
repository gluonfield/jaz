import { useEffect, useState } from 'react'
import type { VoiceOverlayState } from '@shared/voice'
import { FloatingVoice } from '@/components/session/FloatingVoice'
import { clientRuntime } from '@/lib/clientRuntime'

export function VoiceOverlay() {
  const [state, setState] = useState<VoiceOverlayState | null>(null)
  useEffect(() => clientRuntime.voiceOverlay?.subscribe(setState), [])
  useEffect(() => {
    document.documentElement.classList.toggle('dark', state?.dark ?? false)
  }, [state?.dark])
  if (!state) {
    return null
  }
  const command = clientRuntime.voiceOverlay!.command
  const voice = { ...state, analyser: null, outputAnalyser: null,
    start: () => command('reconnect'), end: () => command('exit'), dismiss: () => command('exit'),
    mute: () => command('mute'), muteSpeaker: () => command('muteSpeaker') }
  return <div className="grid h-full place-items-center p-3"><FloatingVoice voice={voice} level={state.level} outputLevel={state.outputLevel} reducedMotion={state.reducedMotion} onReturn={() => command('return')} /></div>
}
