import { useCallback } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { initialVoiceState } from '@/lib/voice/session'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'

export function useVoiceMode(sessionId: string) {
  const voice = useGlobalVoice()
  const navigate = useNavigate()
  const { sessionId: owner, connect, start: reconnect, phase } = voice
  const start = useCallback(() => {
    if (owner && phase !== 'off' && owner !== sessionId) {
      void navigate({ to: '/sessions/$sessionId', params: { sessionId: owner } })
    } else if (owner && phase !== 'off') {
      if (phase === 'error') {
        reconnect()
      }
    } else {
      connect(sessionId)
    }
  }, [connect, navigate, owner, phase, reconnect, sessionId])
  return { ...voice, ...(owner !== sessionId ? initialVoiceState : {}), start }
}
