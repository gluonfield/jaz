import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useBackendChange, useConnection } from '@/lib/connection'
import { clientRuntime } from '@/lib/clientRuntime'
import { connectClaudeVoiceHost } from '@/lib/voice/desktopHost'
import { createDesktopVoiceHost } from '@/lib/voice/desktop'
import type { Bot } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { useVoiceBot, voiceHome } from '@/lib/voice/bot'
import { initialVoiceState, VoiceSession, type VoiceHandle } from '@/lib/voice/session'

// The conversation, the bot it is with if any, and the route it lives on.
type VoiceContextValue = VoiceHandle & { sessionId: string | null; bot?: Bot; home: string; connect: (sessionId: string) => void }
const VoiceContext = createContext<VoiceContextValue | null>(null)

export function VoiceProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient()
  const { status, url } = useConnection()
  const connected = status === 'connected' || status === 'reconnecting'
  useEffect(() => {
    const api = window.jaz?.claudeVoice
    if (!api || clientRuntime.windowKind !== 'main' || !connected) {
      return
    }
    return connectClaudeVoiceHost(api)
  }, [connected, url])
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
    }, createDesktopVoiceHost())
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
  const bot = useVoiceBot(sessionId)
  const home = sessionId ? voiceHome(sessionId, bot) : ''
  useBackendChange(actions.dismiss)
  useEffect(() => () => {
    current.current?.dispose()
    current.current = null
  }, [])

  return <VoiceContext.Provider value={{ ...state, ...actions, sessionId, bot, home, connect }}>{children}</VoiceContext.Provider>
}

export function useGlobalVoice() {
  const voice = useContext(VoiceContext)
  if (!voice) {
    throw new Error('VoiceProvider is required')
  }
  return voice
}
