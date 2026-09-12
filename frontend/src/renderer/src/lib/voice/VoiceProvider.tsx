import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useBackendChange } from '@/lib/connection'
import { keys } from '@/lib/query/keys'
import { initialVoiceState, VoiceSession, type VoiceHandle } from '@/lib/voice/session'

type VoiceContextValue = VoiceHandle & { sessionId: string | null; connect: (sessionId: string) => void }
const VoiceContext = createContext<VoiceContextValue | null>(null)

export function VoiceProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient()
  const current = useRef<VoiceSession | null>(null)
  const [state, setState] = useState(initialVoiceState)
  const [sessionId, setSessionId] = useState<string | null>(null)

  const connect = useCallback((id: string) => {
    if (current.current && current.current.phase !== 'off') {
      return
    }
    current.current?.dispose()
    const voice = new VoiceSession(id, (next) => {
      if (current.current === voice) {
        setState(next)
      }
    }, () => {
      void client.invalidateQueries({ queryKey: keys.sessionMessages(id) })
      void client.invalidateQueries({ queryKey: keys.sidebarSessions })
    })
    current.current = voice
    setSessionId(id)
    voice.start()
  }, [client])

  const actions = useMemo(() => ({
    start: () => current.current?.start(),
    end: () => current.current?.end(),
    dismiss: () => current.current?.dismiss(),
    mute: () => current.current?.mute(),
    muteSpeaker: () => current.current?.muteSpeaker(),
  }), [])
  useBackendChange(actions.dismiss)
  useEffect(() => () => {
    current.current?.dispose()
    current.current = null
  }, [])

  return <VoiceContext.Provider value={{ ...state, ...actions, sessionId, connect }}>{children}</VoiceContext.Provider>
}

export function useGlobalVoice() {
  const voice = useContext(VoiceContext)
  if (!voice) {
    throw new Error('VoiceProvider is required')
  }
  return voice
}
