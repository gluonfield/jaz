import type { AssistantAction, AssistantWakeMode, QuickDictatePhase, QuickDictatePublishState } from '@shared/assistant'
import type { DictationAvailability, DictationEvent } from '@shared/dictation'
import type { Bot } from '@/lib/api/types'
import { DictationEndpoint } from './endpointing'

const SENT_HOLD_MS = 2500
const ERROR_HOLD_MS = 4000
const ENDPOINT_TICK_MS = 200

export interface AssistantVoiceHost {
  phase: 'off' | 'connecting' | 'listening' | 'ending' | 'error'
  sessionId: string | null
  connect: (sessionId: string) => void
  start: () => void
}

export interface AssistantDictationHost {
  availability: () => Promise<DictationAvailability>
  start: (id: string) => Promise<void>
  stop: (id: string) => Promise<void>
  cancel: (id: string) => Promise<void>
}

export interface AssistantControllerDeps {
  getWakeMode: () => AssistantWakeMode
  isCloud: () => boolean
  getVoice: () => AssistantVoiceHost
  getDictation: () => AssistantDictationHost | undefined
  resolvePrimaryBot: () => Promise<Bot | null>
  isVoiceAvailable: () => Promise<boolean>
  sendBotMessage: (botId: string, text: string) => Promise<void>
  focusMain: () => void
  openRoute: (path: string) => void
  toast: (message: string, tone: 'danger') => void
  publish: (state: QuickDictatePublishState | null) => void
  now?: () => number
  newId?: () => string
  tickMs?: number
  sentHoldMs?: number
  errorHoldMs?: number
}

type ActiveDictation = {
  id: string
  generation: number
  bot: Bot
  phase: 'starting' | 'downloading' | 'listening' | 'transcribing'
  text: string
  level: number
  endpoint: DictationEndpoint | null
  tickTimer: ReturnType<typeof setInterval> | null
}

export function findPrimaryBot(bots: readonly Bot[]): Bot | undefined {
  return bots.find((bot) => bot.kind === 'bot' && bot.main) ?? bots.find((bot) => bot.kind === 'bot')
}

function pillState(
  bot: Pick<Bot, 'id' | 'name' | 'avatar'> | null,
  phase: QuickDictatePhase,
  extras: { text?: string; level?: number; error?: string } = {},
): QuickDictatePublishState {
  return {
    phase,
    botName: bot?.name ?? 'Jaz',
    botId: bot?.id,
    avatar: bot?.avatar,
    text: extras.text ?? '',
    level: extras.level ?? 0,
    ...(extras.error ? { error: extras.error } : {}),
  }
}

export function createAssistantController(deps: AssistantControllerDeps) {
  const now = deps.now ?? (() => Date.now())
  const newId = deps.newId ?? (() => crypto.randomUUID())
  const tickMs = deps.tickMs ?? ENDPOINT_TICK_MS
  const sentHoldMs = deps.sentHoldMs ?? SENT_HOLD_MS
  const errorHoldMs = deps.errorHoldMs ?? ERROR_HOLD_MS

  let generation = 0
  let startingDictation = false
  let active: ActiveDictation | null = null
  let lastBotId: string | undefined
  let dismissTimer: ReturnType<typeof setTimeout> | null = null

  const clearDismiss = () => {
    if (dismissTimer) {
      clearTimeout(dismissTimer)
      dismissTimer = null
    }
  }

  const stopTicks = (run: ActiveDictation) => {
    if (run.tickTimer) {
      clearInterval(run.tickTimer)
      run.tickTimer = null
    }
  }

  const scheduleDismiss = (ms: number, runGen: number) => {
    clearDismiss()
    dismissTimer = setTimeout(() => {
      if (generation === runGen && !active) deps.publish(null)
    }, ms)
  }

  const showError = (bot: Bot | null, message: string, runGen: number, text = '', holdMs = errorHoldMs) => {
    deps.publish(pillState(bot, 'error', { text, error: message }))
    scheduleDismiss(holdMs, runGen)
  }

  const cancel = (hidePill = true) => {
    generation += 1
    startingDictation = false
    clearDismiss()
    const current = active
    active = null
    if (current) {
      stopTicks(current)
      void deps.getDictation()?.cancel(current.id).catch(() => {})
    }
    if (hidePill) deps.publish(null)
  }

  const sendToBot = async (bot: Bot, rawText: string, runGen: number) => {
    const text = rawText.trim()
    if (!text) {
      showError(bot, 'No speech was detected.', runGen)
      return
    }
    lastBotId = bot.id
    deps.publish(pillState(bot, 'sending', { text }))
    try {
      await deps.sendBotMessage(bot.id, text)
      if (generation !== runGen) return
      deps.publish(pillState(bot, 'sent', { text }))
      scheduleDismiss(sentHoldMs, runGen)
    } catch (error) {
      if (generation !== runGen) return
      showError(bot, error instanceof Error ? error.message : 'Could not send message.', runGen, text)
    }
  }

  const stopAndSend = () => {
    const current = active
    if (!current || current.phase !== 'listening') return
    stopTicks(current)
    current.phase = 'transcribing'
    deps.publish(pillState(current.bot, 'transcribing', { text: current.text }))
    void deps.getDictation()?.stop(current.id).catch((error) => {
      if (active !== current) return
      active = null
      showError(current.bot, error instanceof Error ? error.message : 'Dictation could not finish.', current.generation, current.text)
    })
  }

  const runVoice = async () => {
    cancel(true)
    const runGen = generation
    deps.focusMain()
    const bot = await deps.resolvePrimaryBot()
    if (generation !== runGen) return
    if (!bot) {
      deps.openRoute('/bots')
      deps.toast('No primary bot found yet.', 'danger')
      return
    }
    deps.openRoute(`/bots/${bot.id}`)
    const voice = deps.getVoice()
    if (voice.phase !== 'off') {
      if (voice.sessionId === bot.id) {
        if (voice.phase === 'error') voice.start()
        return
      }
      deps.toast('End your active voice session before starting another.', 'danger')
      return
    }
    if (deps.isCloud()) {
      deps.toast('Live voice is unavailable in Cloud workspaces. Switch Hey Jaz to Quick Dictate in Settings → Voice.', 'danger')
      return
    }
    const available = await deps.isVoiceAvailable().catch(() => false)
    if (generation !== runGen) return
    if (!available) {
      deps.toast('Configure a voice provider in Settings → Voice to start live voice.', 'danger')
      return
    }
    deps.getVoice().connect(bot.id)
  }

  const runDictate = async () => {
    if (active) {
      if (active.phase === 'listening') stopAndSend()
      return
    }
    if (startingDictation) return

    const runGen = ++generation
    startingDictation = true
    clearDismiss()
    try {
      if (deps.getVoice().phase !== 'off') {
        showError(null, 'End live voice before starting quick dictation.', runGen)
        return
      }
      const bot = await deps.resolvePrimaryBot()
      if (generation !== runGen) return
      if (!bot) {
        showError(null, 'No primary bot found.', runGen)
        return
      }
      lastBotId = bot.id
      const dictation = deps.getDictation()
      if (!dictation) {
        showError(bot, 'Native dictation is unavailable here. Use Shortcuts Dictate Text with jaz://assistant?text=...', runGen)
        return
      }
      const availability = await dictation.availability().catch(() => ({
        available: false as const,
        reason: 'Native dictation is unavailable.',
      }))
      if (generation !== runGen) return
      if (!availability.available) {
        showError(bot, availability.reason, runGen)
        return
      }

      const run: ActiveDictation = {
        id: newId(),
        generation: runGen,
        bot,
        phase: 'starting',
        text: '',
        level: 0,
        endpoint: null,
        tickTimer: null,
      }
      active = run
      deps.publish(pillState(bot, 'starting'))
      await dictation.start(run.id)
      if (generation !== runGen) {
        void dictation.cancel(run.id).catch(() => {})
      }
    } catch (error) {
      if (generation !== runGen) return
      const bot = active?.bot ?? null
      active = null
      showError(bot, error instanceof Error ? error.message : 'Dictation could not start.', runGen)
    } finally {
      if (generation === runGen) startingDictation = false
    }
  }

  const runDirectSend = async (text: string) => {
    cancel(false)
    const runGen = generation
    const bot = await deps.resolvePrimaryBot()
    if (generation !== runGen) return
    if (!bot) {
      showError(null, 'No primary bot found.', runGen, text)
      return
    }
    await sendToBot(bot, text, runGen)
  }

  const handleAction = async (action: AssistantAction) => {
    if (action.mode === 'send') {
      await runDirectSend(action.text)
      return
    }
    const mode = action.mode === 'default' ? deps.getWakeMode() : action.mode
    if (mode === 'voice') await runVoice()
    else await runDictate()
  }

  const handlePillCommand = (type: 'send' | 'cancel' | 'open') => {
    if (type === 'send') {
      stopAndSend()
    } else if (type === 'cancel') {
      cancel(true)
    } else {
      const targetId = active?.bot.id ?? lastBotId
      cancel(true)
      if (targetId) {
        deps.focusMain()
        deps.openRoute(`/bots/${targetId}`)
      }
    }
  }

  const handleDictationEvent = (id: string, event: DictationEvent) => {
    const current = active
    if (!current || current.id !== id) return

    const applyDecision = (decision: 'continue' | 'stop' | 'no_speech') => {
      if (decision === 'stop') {
        stopAndSend()
      } else if (decision === 'no_speech') {
        cancel(false)
        showError(current.bot, "Didn't catch anything.", generation, '', sentHoldMs)
      }
    }

    switch (event.type) {
      case 'status':
        if (event.phase === 'downloading') {
          current.phase = 'downloading'
          deps.publish(pillState(current.bot, 'downloading', { text: current.text }))
        } else if (event.phase === 'recording') {
          current.phase = 'listening'
          current.endpoint = new DictationEndpoint(now())
          stopTicks(current)
          current.tickTimer = setInterval(() => {
            if (active !== current || current.phase !== 'listening' || !current.endpoint) return
            applyDecision(current.endpoint.tick(now()))
          }, tickMs)
          deps.publish(pillState(current.bot, 'listening', { text: current.text, level: current.level }))
        } else if (event.phase === 'transcribing') {
          stopTicks(current)
          current.phase = 'transcribing'
          deps.publish(pillState(current.bot, 'transcribing', { text: current.text }))
        }
        return
      case 'level':
        if (current.phase !== 'listening') return
        current.level = event.level
        deps.publish(pillState(current.bot, 'listening', { text: current.text, level: current.level }))
        if (current.endpoint) applyDecision(current.endpoint.onLevel(event.level, now()))
        return
      case 'result':
        if (current.phase !== 'listening') return
        current.text = event.text
        deps.publish(pillState(current.bot, 'listening', { text: current.text, level: current.level }))
        if (current.endpoint) applyDecision(current.endpoint.onTranscript(event.text, now()))
        return
      case 'complete': {
        stopTicks(current)
        active = null
        const finalText = event.text.trim() || current.text.trim()
        void sendToBot(current.bot, finalText, current.generation)
        return
      }
      case 'error':
        stopTicks(current)
        active = null
        showError(current.bot, event.message, current.generation, current.text)
        return
    }
  }

  return {
    handleAction,
    handlePillCommand,
    handleDictationEvent,
    cancel,
  }
}
