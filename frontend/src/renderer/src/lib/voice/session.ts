import { getSessionMessagesPage } from '@/lib/api/sessions'
import { saveVoiceTranscript, sendVoiceTask } from '@/lib/api/liveVoice'
import { VoiceConnection } from '@/lib/voice/connection'
import { openSessionEvents } from '@/lib/api/sse'
import type { VoiceTask } from '@/lib/voice/delegation'
import { VoiceTaskStream } from '@/lib/voice/taskStream'
import type { VoiceEvent } from '@/lib/voice/protocol'
import { VoiceRecorder } from '@/lib/voice/recorder'
import { VoiceTranscriptWriter } from '@/lib/voice/writer'
import type { VoiceStatus } from '@shared/voice'

export type VoiceState = VoiceStatus & {
  analyser: AnalyserNode | null
}

export const initialVoiceState: VoiceState = { phase: 'off', muted: false, speakerMuted: false, working: false, error: '', analyser: null }

export class VoiceSession {
  private state = initialVoiceState
  private connection?: VoiceConnection
  private stop = () => {}
  private transcript: VoiceTranscriptWriter

  get phase() {
    return this.state.phase
  }

  constructor(private sessionId: string, private onState: (state: VoiceState) => void, private refresh: () => void) {
    this.transcript = new VoiceTranscriptWriter(
      (messages) => saveVoiceTranscript(sessionId, messages),
      () => {
        this.update({})
        this.refresh()
      },
    )
  }

  private update(change: Partial<VoiceState>) {
    this.state = { ...this.state, ...change }
    this.onState({ ...this.state, error: [this.state.error, this.transcript.error].filter(Boolean).join(' ') })
  }

  start = () => {
    this.stop()
    this.connection?.dispose()
    this.connection = undefined
    const abort = new AbortController()
    const recorder = new VoiceRecorder()
    const seen = new Set<string>()
    const pending = new Map<string, Extract<VoiceEvent, { type: 'delegation' }>>()
    let submit = Promise.resolve()
    let undeliveredContext = ''
    let followError = ''
    const stream = new VoiceTaskStream({
      read: () => getSessionMessagesPage(this.sessionId, { turns: 32 }, abort.signal),
      subscribe: (after, onEvent, onConnection) => openSessionEvents(this.sessionId, after, onEvent, onConnection),
    }, recorder.callId, (text, speak, id) => connection.append(text, speak, id), (working, error) => {
      this.update({ working, ...(error || this.state.error === followError ? { error } : {}) })
      followError = error
    })
    this.stop = () => {
      void this.transcript.flush()
      abort.abort()
      stream.stop()
    }
    this.update({ ...initialVoiceState, phase: 'connecting' })

    const delegate = (event: Extract<VoiceEvent, { type: 'delegation' }>) => {
      if (seen.has(event.id)) {
        return
      }
      const request = recorder.request(event)
      if (!request) {
        pending.set(event.id, event)
        return
      }
      pending.delete(event.id)
      seen.add(event.id)
      submit = submit.then(async () => {
        if (abort.signal.aborted) {
          return
        }
        undeliveredContext = [undeliveredContext, request.context || `User: ${request.text}`].filter(Boolean).join('\n')
        const contexts = [{ type: 'voice' as const, id: recorder.callId, request_id: event.id, text: undeliveredContext }]
        const task: VoiceTask = { id: event.id, callId: recorder.callId }
        await sendVoiceTask(this.sessionId, request.text, contexts, abort.signal)
        undeliveredContext = ''
        if (abort.signal.aborted) {
          return
        }
        this.update({ error: '', working: true })
        connection.append(`Agent request accepted: ${JSON.stringify({ request: request.text, status: 'running' })}`, false, event.id)
        stream.follow(task)
        this.refresh()
      }).catch((error: Error) => {
        if (abort.signal.aborted) {
          return
        }
        connection.append(`The agent request failed: ${error.message}`, true, event.id)
        this.update({ error: error.message })
      })
    }

    let connection: VoiceConnection
    try {
      connection = new VoiceConnection((event) => {
        if (this.connection !== connection) {
          return
        }
        if (event.type === 'closed' || event.type === 'error') {
          const error = event.type === 'error' && !abort.signal.aborted ? event.message : ''
          this.stop()
          connection.dispose()
          this.connection = undefined
          this.update({ ...initialVoiceState, phase: error ? 'error' : 'off', error })
          return
        }
        if (event.type === 'transcript' || event.type === 'transcript_done') {
          const message = recorder.accept(event)
          if (message) {
            this.transcript.append(message, event.type === 'transcript_done')
          }
        }
        if (abort.signal.aborted) {
          return
        }
        switch (event.type) {
          case 'ready': {
            this.update({ phase: 'listening', analyser: connection.analyser })
            stream.start()
            break
          }
          case 'transcript':
          case 'transcript_done':
            for (const delegation of pending.values()) {
              delegate(delegation)
            }
            break
          case 'delegation':
            delegate(event)
            break
        }
      })
      this.connection = connection
      void connection.start(() => stream.context()).catch((error: Error) => {
        if (abort.signal.aborted) {
          return
        }
        connection.dispose()
        this.connection = undefined
        this.stop()
        this.update({ phase: 'error', analyser: null, error: error.message })
      })
    } catch (error) {
      this.stop()
      this.update({ ...initialVoiceState, phase: 'error', error: `Voice couldn't start: ${(error as Error).message}` })
    }
  }

  end = () => {
    if (!this.connection) {
      this.dismiss()
      return
    }
    this.stop()
    this.update({ phase: 'ending', muted: true })
    this.connection?.end()
  }

  dismiss = () => {
    this.stop()
    this.connection?.dispose()
    this.connection = undefined
    this.update(initialVoiceState)
  }

  mute = () => {
    const muted = !this.state.muted
    this.connection?.mute(muted)
    this.update({ muted })
  }

  muteSpeaker = () => {
    const speakerMuted = !this.state.speakerMuted
    this.connection?.muteSpeaker(speakerMuted)
    this.update({ speakerMuted })
  }

  dispose() {
    this.onState = () => {}
    this.stop()
    this.connection?.dispose()
    this.connection = undefined
  }
}

export type VoiceHandle = VoiceState & Pick<VoiceSession, 'start' | 'end' | 'dismiss' | 'mute' | 'muteSpeaker'>
