import { useQueryClient } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import type { AssistantAction } from '@shared/assistant'
import { botsQuery } from '@/lib/api/bots'
import { isCloudApiSession, post } from '@/lib/api/client'
import { createCloudTurn } from '@/lib/api/cloudThreads'
import { voiceSettingsQuery } from '@/lib/api/liveVoice'
import { clientRuntime } from '@/lib/clientRuntime'
import { useBackendChange, useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'
import { useToast } from '@/components/ui/toast'
import { createAssistantController, findPrimaryBot } from './controller'
import { getAssistantWakeMode } from './settings'

export function AssistantBridge() {
  const assistant = clientRuntime.assistant
  const queryClient = useQueryClient()
  const router = useRouter()
  const toast = useToast()
  const voice = useGlobalVoice()
  const { status: connectionStatus } = useConnection()

  const voiceRef = useRef(voice)
  voiceRef.current = voice
  const connectionRef = useRef(connectionStatus)
  connectionRef.current = connectionStatus
  const pendingAction = useRef<AssistantAction | null>(null)

  const controllerRef = useRef<ReturnType<typeof createAssistantController> | null>(null)
  if (!controllerRef.current && assistant) {
    controllerRef.current = createAssistantController({
      getWakeMode: getAssistantWakeMode,
      isCloud: isCloudApiSession,
      getVoice: () => voiceRef.current,
      getDictation: () => window.jaz?.dictation,
      resolvePrimaryBot: async () => {
        try {
          return findPrimaryBot(await queryClient.ensureQueryData(botsQuery)) ?? null
        } catch {
          return null
        }
      },
      isVoiceAvailable: async () => {
        const settings = await queryClient.ensureQueryData(voiceSettingsQuery)
        return settings.providers.some((provider) => provider.id === settings.provider && provider.available)
      },
      sendBotMessage: async (botId, text) => {
        if (isCloudApiSession()) {
          await createCloudTurn(botId, { text })
        } else {
          await post(`/v1/sessions/${botId}/agent/input`, { message: text })
        }
        void queryClient.invalidateQueries({ queryKey: keys.sessionMessages(botId) })
        void queryClient.invalidateQueries({ queryKey: keys.bots })
        void queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      },
      focusMain: () => assistant.focusMain(),
      openRoute: (path) => router.history.push(path),
      toast: (message, tone) => toast(message, tone),
      publish: (state) => assistant.publishQuickDictate(state),
    })
  }

  useBackendChange(() => {
    pendingAction.current = null
    controllerRef.current?.cancel(true)
  })

  useEffect(() => {
    const controller = controllerRef.current
    if (!assistant || !controller) return
    const offTrigger = assistant.onTrigger((action) => {
      if (connectionRef.current !== 'connected') {
        pendingAction.current = action
        if (action.mode === 'voice' || (action.mode === 'default' && getAssistantWakeMode() === 'voice')) {
          assistant.focusMain()
        }
        return
      }
      void controller.handleAction(action)
    })
    const offCommand = assistant.onQuickDictateCommand((command) => {
      if (command.type !== 'hover') controller.handlePillCommand(command.type)
    })
    const offDictation = window.jaz?.dictation?.onEvent((id, event) => controller.handleDictationEvent(id, event))
    return () => {
      offTrigger()
      offCommand()
      offDictation?.()
      controller.cancel(true)
    }
  }, [assistant])

  useEffect(() => {
    if (connectionStatus !== 'connected' || !pendingAction.current) return
    const action = pendingAction.current
    pendingAction.current = null
    void controllerRef.current?.handleAction(action)
  }, [connectionStatus])

  return null
}
