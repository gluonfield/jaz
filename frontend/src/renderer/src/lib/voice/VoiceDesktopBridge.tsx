import { useEffect } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { clientRuntime } from '@/lib/clientRuntime'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { useTheme } from '@/lib/theme'
import { audioLevel } from '@/lib/voice/audioLevel'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'

export function VoiceDesktopBridge() {
  const voice = useGlobalVoice()
  const { resolved } = useTheme()
  const reducedMotion = useReducedEffectsMotion()
  const { sessionId, phase, muted, speakerMuted, working, error, analyser, start, end, mute, muteSpeaker } = voice
  const docked = useRouterState({ select: (state) => state.location.pathname === `/sessions/${sessionId}` && !state.location.search.settings })
  useEffect(() => clientRuntime.voiceOverlay?.onCommand((command) => {
    const actions = { mute, muteSpeaker, reconnect: start, exit: end }
    if (command !== 'return') {
      actions[command]()
    }
  }), [end, mute, muteSpeaker, start])

  useEffect(() => {
    const bridge = clientRuntime.voiceOverlay
    if (!bridge) {
      return
    }
    if (!sessionId || phase === 'off') {
      bridge.publish(null)
      return
    }
    const samples = new Uint8Array(analyser?.fftSize ?? 0)
    const publish = () => bridge.publish({ sessionId, phase, muted, speakerMuted, working, error, docked,
      dark: resolved === 'dark', reducedMotion, level: analyser && !reducedMotion ? audioLevel(analyser, samples) : 0 })
    publish()
    if (!analyser || reducedMotion) {
      return
    }
    const timer = setInterval(publish, 80)
    return () => clearInterval(timer)
  }, [sessionId, phase, muted, speakerMuted, working, error, analyser, resolved, reducedMotion, docked])
  useEffect(() => () => clientRuntime.voiceOverlay?.publish(null), [])
  return null
}
