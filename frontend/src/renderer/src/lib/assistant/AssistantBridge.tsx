import { useQueryClient } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { useCallback, useEffect, useRef } from 'react'
import type { AssistantAction, QuickDictatePublishState } from '@shared/assistant'
import { botsQuery } from '@/lib/api/bots'
import { isCloudApiSession, post } from '@/lib/api/client'
import { createCloudTurn } from '@/lib/api/cloudThreads'
import { voiceSettingsQuery } from '@/lib/api/liveVoice'
import type { Bot } from '@/lib/api/types'
import { clientRuntime } from '@/lib/clientRuntime'
import { useBackendChange, useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'
import { useToast } from '@/components/ui/toast'
import { DictationEndpoint } from './endpointing'
import { getAssistantWakeMode } from './settings'

const SENT_HOLD_MS = 2500
const ERROR_HOLD_MS = 4000
const ENDPOINT_TICK_MS = 200

type ActiveDictation = {
  id: string
  bot: Bot
  phase: 'starting' | 'downloading' | 'listening' | 'transcribing'
  text: string
  level: number
  endpoint: DictationEndpoint | null
  tickTimer: ReturnType<typeof setInterval> | null
}

function findPrimaryBot(bots: Bot[]): Bot | undefined {
  return bots.find((bot) => bot.kind === 'bot' && bot.main) ?? bots.find((bot) => bot.kind === 'bot')
}

export function AssistantBridge() {
  const assistant = clientRuntime.assistant
  const queryClient = useQueryClient()
  const router = useRouter()
  const toast = useToast()
  const voice = useGlobalVoice()
  const { status: connectionStatus } = useConnection()

  const voiceRef = useRef(voice)
  voiceRef.current = voice

  const pendingAction = useRef<AssistantAction | null>(null)
  const active = useRef<ActiveDictation | null>(null)
  const lastBotId = useRef<string | undefined>(undefined)
  const dismissTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearDismissTimer = useCallback(() => {
    if (dismissTimer.current) {
      clearTimeout(dismissTimer.current)
      dismissTimer.current = null
    }
  }, [])

  const publish = useCallback((state: QuickDictatePublishState | null) => {
    assistant?.publishQuickDictate(state)
  }, [assistant])

  const scheduleDismiss = useCallback((ms: number) => {
    clearDismissTimer()
    dismissTimer.current = setTimeout(() => {
      if (!active.current) {
        publish(null)
      }
    }, ms)
  }, [clearDismissTimer, publish])

  const stopActiveTimers = useCallback((run: ActiveDictation) => {
    if (run.tickTimer) {
      clearInterval(run.tickTimer)
      run.tickTimer = null
    }
  }, [])

  const cancelActive = useCallback((hidePill = true) => {
    clearDismissTimer()
    const current = active.current
    active.current = null
    if (current) {
      stopActiveTimers(current)
      void window.jaz?.dictation?.cancel(current.id).catch(() => {})
    }
    if (hidePill) publish(null)
  }, [clearDismissTimer, publish, stopActiveTimers])

  useBackendChange(() => {
    pendingAction.current = null
    cancelActive(true)
  })

  const resolvePrimaryBot = useCallback(async (): Promise<Bot | null> => {
    try {
      const bots = await queryClient.ensureQueryData(botsQuery)
      return findPrimaryBot(bots) ?? null
    } catch {
      return null
    }
  }, [queryClient])

  const sendToPrimaryBot = useCallback(async (bot: Bot, rawText: string) => {
    const text = rawText.trim()
    if (!text) {
      publish({
        phase: 'error',
        botName: bot.name,
        botId: bot.id,
        avatar: bot.avatar,
        text: '',
        level: 0,
        error: 'No speech was detected.',
      })
      scheduleDismiss(ERROR_HOLD_MS)
      return
    }
    lastBotId.current = bot.id
    publish({
      phase: 'sending',
      botName: bot.name,
      botId: bot.id,
      avatar: bot.avatar,
      text,
      level: 0,
    })
    try {
      if (isCloudApiSession()) {
        await createCloudTurn(bot.id, { text })
      } else {
        await post(`/v1/sessions/${bot.id}/agent/input`, { message: text })
      }
      void queryClient.invalidateQueries({ queryKey: keys.sessionMessages(bot.id) })
      void queryClient.invalidateQueries({ queryKey: keys.bots })
      void queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      publish({
        phase: 'sent',
        botName: bot.name,
        botId: bot.id,
        avatar: bot.avatar,
        text,
        level: 0,
      })
      scheduleDismiss(SENT_HOLD_MS)
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Could not send message.'
      publish({
        phase: 'error',
        botName: bot.name,
        botId: bot.id,
        avatar: bot.avatar,
        text,
        level: 0,
        error: message,
      })
      scheduleDismiss(ERROR_HOLD_MS)
    }
  }, [publish, queryClient, scheduleDismiss])

  const stopAndSendActive = useCallback(() => {
    const current = active.current
    if (!current || current.phase !== 'listening') return
    stopActiveTimers(current)
    current.phase = 'transcribing'
    publish({
      phase: 'transcribing',
      botName: current.bot.name,
      botId: current.bot.id,
      avatar: current.bot.avatar,
      text: current.text,
      level: 0,
    })
    void window.jaz?.dictation?.stop(current.id).catch((error) => {
      if (active.current !== current) return
      active.current = null
      publish({
        phase: 'error',
        botName: current.bot.name,
        botId: current.bot.id,
        avatar: current.bot.avatar,
        text: current.text,
        level: 0,
        error: error instanceof Error ? error.message : 'Dictation could not finish.',
      })
      scheduleDismiss(ERROR_HOLD_MS)
    })
  }, [publish, scheduleDismiss, stopActiveTimers])

  const openBotInMain = useCallback((botId: string) => {
    assistant?.focusMain()
    router.history.push(`/bots/${botId}`)
  }, [assistant, router])

  const runVoiceMode = useCallback(async () => {
    assistant?.focusMain()
    const bot = await resolvePrimaryBot()
    if (!bot) {
      router.history.push('/bots')
      toast('No primary bot found yet.', 'danger')
      return
    }
    router.history.push(`/bots/${bot.id}`)
    const currentVoice = voiceRef.current
    if (currentVoice.phase !== 'off') {
      if (currentVoice.sessionId === bot.id) {
        if (currentVoice.phase === 'error') currentVoice.start()
        return
      }
      toast('End your active voice session before starting another.', 'danger')
      return
    }
    if (isCloudApiSession()) {
      toast('Live voice is unavailable in Cloud workspaces. Switch Hey Jaz to Quick Dictate in Settings → Voice.', 'danger')
      return
    }
    try {
      const settings = await queryClient.ensureQueryData(voiceSettingsQuery)
      const available = settings.providers.some((provider) => provider.id === settings.provider && provider.available)
      if (!available) {
        toast('Configure a voice provider in Settings → Voice to start live voice.', 'danger')
        return
      }
    } catch {
      toast('Voice settings are unavailable on this server.', 'danger')
      return
    }
    currentVoice.connect(bot.id)
  }, [assistant, queryClient, resolvePrimaryBot, router, toast])

  const runQuickDictate = useCallback(async () => {
    if (active.current) {
      if (active.current.phase === 'listening') {
        stopAndSendActive()
      }
      return
    }
    clearDismissTimer()
    if (voiceRef.current.phase !== 'off') {
      publish({
        phase: 'error',
        botName: 'Jaz',
        text: '',
        level: 0,
        error: 'End live voice before starting quick dictation.',
      })
      scheduleDismiss(ERROR_HOLD_MS)
      return
    }
    const bot = await resolvePrimaryBot()
    if (!bot) {
      publish({
        phase: 'error',
        botName: 'Jaz',
        text: '',
        level: 0,
        error: 'No primary bot found.',
      })
      scheduleDismiss(ERROR_HOLD_MS)
      return
    }
    lastBotId.current = bot.id
    const dictation = window.jaz?.dictation
    if (!dictation) {
      publish({
        phase: 'error',
        botName: bot.name,
        botId: bot.id,
        avatar: bot.avatar,
        text: '',
        level: 0,
        error: 'Native dictation is unavailable here. Use Shortcuts Dictate Text with jaz://assistant?text=...',
      })
      scheduleDismiss(ERROR_HOLD_MS)
      return
    }
    const availability = await dictation.availability().catch(() => ({
      available: false as const,
      reason: 'Native dictation is unavailable.',
    }))
    if (!availability.available) {
      publish({
        phase: 'error',
        botName: bot.name,
        botId: bot.id,
        avatar: bot.avatar,
        text: '',
        level: 0,
        error: availability.reason,
      })
      scheduleDismiss(ERROR_HOLD_MS)
      return
    }

    const run: ActiveDictation = {
      id: crypto.randomUUID(),
      bot,
      phase: 'starting',
      text: '',
      level: 0,
      endpoint: null,
      tickTimer: null,
    }
    active.current = run
    publish({
      phase: 'starting',
      botName: bot.name,
      botId: bot.id,
      avatar: bot.avatar,
      text: '',
      level: 0,
    })
    try {
      await dictation.start(run.id)
    } catch (error) {
      if (active.current !== run) return
      active.current = null
      publish({
        phase: 'error',
        botName: bot.name,
        botId: bot.id,
        avatar: bot.avatar,
        text: '',
        level: 0,
        error: error instanceof Error ? error.message : 'Dictation could not start.',
      })
      scheduleDismiss(ERROR_HOLD_MS)
    }
  }, [clearDismissTimer, publish, resolvePrimaryBot, scheduleDismiss, stopAndSendActive])

  const runDirectSend = useCallback(async (text: string) => {
    cancelActive(false)
    const bot = await resolvePrimaryBot()
    if (!bot) {
      publish({
        phase: 'error',
        botName: 'Jaz',
        text,
        level: 0,
        error: 'No primary bot found.',
      })
      scheduleDismiss(ERROR_HOLD_MS)
      return
    }
    await sendToPrimaryBot(bot, text)
  }, [cancelActive, publish, resolvePrimaryBot, scheduleDismiss, sendToPrimaryBot])

  const executeAction = useCallback(async (action: AssistantAction) => {
    if (action.mode === 'send') {
      await runDirectSend(action.text)
      return
    }
    const resolved = action.mode === 'default' ? getAssistantWakeMode() : action.mode
    if (resolved === 'voice') {
      await runVoiceMode()
    } else {
      await runQuickDictate()
    }
  }, [runDirectSend, runQuickDictate, runVoiceMode])

  useEffect(() => {
    if (!assistant) return
    return assistant.onTrigger((action) => {
      if (connectionStatus !== 'connected') {
        pendingAction.current = action
        return
      }
      void executeAction(action)
    })
  }, [assistant, connectionStatus, executeAction])

  useEffect(() => {
    if (connectionStatus !== 'connected' || !pendingAction.current) return
    const action = pendingAction.current
    pendingAction.current = null
    void executeAction(action)
  }, [connectionStatus, executeAction])

  useEffect(() => {
    if (!assistant) return
    return assistant.onQuickDictateCommand((command) => {
      switch (command.type) {
        case 'send':
          stopAndSendActive()
          return
        case 'cancel':
          cancelActive(true)
          return
        case 'open': {
          const targetId = active.current?.bot.id ?? lastBotId.current
          cancelActive(true)
          if (targetId) openBotInMain(targetId)
          return
        }
      }
    })
  }, [assistant, cancelActive, openBotInMain, stopAndSendActive])

  useEffect(() => {
    const dictation = window.jaz?.dictation
    if (!dictation) return
    return dictation.onEvent((id, event) => {
      const current = active.current
      if (!current || current.id !== id) return

      const applyEndpoint = (decision: 'continue' | 'stop' | 'no_speech') => {
        if (decision === 'stop') {
          stopAndSendActive()
        } else if (decision === 'no_speech') {
          cancelActive(false)
          publish({
            phase: 'error',
            botName: current.bot.name,
            botId: current.bot.id,
            avatar: current.bot.avatar,
            text: '',
            level: 0,
            error: "Didn't catch anything.",
          })
          scheduleDismiss(SENT_HOLD_MS)
        }
      }

      switch (event.type) {
        case 'status':
          if (event.phase === 'downloading') {
            current.phase = 'downloading'
            publish({
              phase: 'downloading',
              botName: current.bot.name,
              botId: current.bot.id,
              avatar: current.bot.avatar,
              text: current.text,
              level: 0,
            })
          } else if (event.phase === 'recording') {
            current.phase = 'listening'
            current.endpoint = new DictationEndpoint(Date.now())
            stopActiveTimers(current)
            current.tickTimer = setInterval(() => {
              if (active.current !== current || current.phase !== 'listening' || !current.endpoint) return
              applyEndpoint(current.endpoint.tick(Date.now()))
            }, ENDPOINT_TICK_MS)
            publish({
              phase: 'listening',
              botName: current.bot.name,
              botId: current.bot.id,
              avatar: current.bot.avatar,
              text: current.text,
              level: 0,
            })
          } else if (event.phase === 'transcribing') {
            stopActiveTimers(current)
            current.phase = 'transcribing'
            publish({
              phase: 'transcribing',
              botName: current.bot.name,
              botId: current.bot.id,
              avatar: current.bot.avatar,
              text: current.text,
              level: 0,
            })
          }
          return
        case 'level':
          if (current.phase !== 'listening') return
          current.level = event.level
          publish({
            phase: 'listening',
            botName: current.bot.name,
            botId: current.bot.id,
            avatar: current.bot.avatar,
            text: current.text,
            level: current.level,
          })
          if (current.endpoint) {
            applyEndpoint(current.endpoint.onLevel(event.level, Date.now()))
          }
          return
        case 'result':
          if (current.phase !== 'listening') return
          current.text = event.text
          publish({
            phase: 'listening',
            botName: current.bot.name,
            botId: current.bot.id,
            avatar: current.bot.avatar,
            text: event.text,
            level: current.level,
          })
          if (current.endpoint) {
            applyEndpoint(current.endpoint.onTranscript(event.text, Date.now()))
          }
          return
        case 'complete': {
          stopActiveTimers(current)
          active.current = null
          const finalText = event.text.trim() || current.text.trim()
          void sendToPrimaryBot(current.bot, finalText)
          return
        }
        case 'error':
          stopActiveTimers(current)
          active.current = null
          publish({
            phase: 'error',
            botName: current.bot.name,
            botId: current.bot.id,
            avatar: current.bot.avatar,
            text: current.text,
            level: 0,
            error: event.message,
          })
          scheduleDismiss(ERROR_HOLD_MS)
          return
      }
    })
  }, [cancelActive, publish, scheduleDismiss, sendToPrimaryBot, stopActiveTimers, stopAndSendActive])

  useEffect(() => () => cancelActive(true), [cancelActive])

  return null
}
