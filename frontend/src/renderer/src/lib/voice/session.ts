import { getSessionMessagesPage } from '@/lib/api/sessions'
import { saveVoiceDiagnostics, saveVoiceTranscript, sendVoiceTask } from '@/lib/api/liveVoice'
import { VoiceConnection } from '@/lib/voice/connection'
import { openSessionEvents } from '@/lib/api/sse'
import { VoiceAdmission } from '@/lib/voice/admission'
import { VoiceTaskStream } from '@/lib/voice/taskStream'
import { VoiceRecorder } from '@/lib/voice/recorder'
import { VoiceTranscriptWriter } from '@/lib/voice/writer'
import type { VoiceStatus } from '@shared/voice'
import type { VoiceDiagnostic, VoiceTrace } from '@/lib/voice/diagnostics'

export type VoiceState = VoiceStatus & {
  analyser: AnalyserNode | null
  outputAnalyser: AnalyserNode | null
}

export const initialVoiceState: VoiceState = { phase: 'off', muted: false, speakerMuted: false, activity: null, error: '', transcript: [], analyser: null, outputAnalyser: null }

export class VoiceSession {
  private state = initialVoiceState
  private connection?: VoiceConnection
  private stop = () => {}
  private writer: VoiceTranscriptWriter
  private diagnostics: VoiceTranscriptWriter<VoiceDiagnostic>

  get phase() {
    return this.state.phase
  }

  constructor(private sessionId: string, private onState: (state: VoiceState) => void, private refresh: () => void) {
    this.diagnostics = new VoiceTranscriptWriter((records) => saveVoiceDiagnostics(sessionId, records), () => {})
    this.writer = new VoiceTranscriptWriter(
      (messages) => saveVoiceTranscript(sessionId, messages),
      () => {
        this.update({})
        this.refresh()
      },
    )
  }

  private update(change: Partial<VoiceState>) {
    this.state = { ...this.state, ...change }
    this.onState({ ...this.state, error: [this.state.error, this.writer.error].filter(Boolean).join(' ') })
  }

  start = () => {
    this.stop()
    this.connection?.dispose()
    this.connection = undefined
    const abort = new AbortController()
    const recorder = new VoiceRecorder()
    const started = performance.now()
    const trace: VoiceTrace = (stage, detail) => this.diagnostics.append({ id: crypto.randomUUID(), call_id: recorder.callId, stage, at: new Date().toISOString(), elapsed_ms: performance.now() - started, ...detail })
    const transcripts = new Set<string>()
    let submit = Promise.resolve()
    let followError = ''
    const stream = new VoiceTaskStream({
      read: () => getSessionMessagesPage(this.sessionId, { turns: 32 }, abort.signal),
      subscribe: (after, onEvent, onConnection) => openSessionEvents(this.sessionId, after, onEvent, onConnection),
    }, recorder.callId, (text, speak, id, sourceSeq) => connection.append(text, speak, id, sourceSeq), (activity, error) => {
      this.update({ activity, ...(error || this.state.error === followError ? { error } : {}) })
      followError = error
    })
    this.stop = () => {
      void this.writer.flush()
      void this.diagnostics.flush()
      abort.abort()
      stream.stop()
    }
    this.update({ ...initialVoiceState, phase: 'connecting' })

    const admission = new VoiceAdmission(recorder, () => stream.question(), (task, text, context) => {
      submit = submit.then(async () => {
        if (abort.signal.aborted) {
          return
        }
        trace('admission_start', { request_id: task.id, user_turn_id: task.id, detail: 'agent input' })
        const contexts = [{ type: 'voice' as const, id: task.callId, request_id: task.id, text: context }]
        for (let attempt = 0; ; attempt++) {
          try {
            await sendVoiceTask(this.sessionId, text, contexts, abort.signal)
            break
          } catch (error) {
            if (attempt || abort.signal.aborted || (error instanceof Error && 'status' in error && Number(error.status) < 500)) {
              throw error
            }
          }
        }
        trace('admitted', { request_id: task.id, user_turn_id: task.id })
        if (abort.signal.aborted) {
          return
        }
        this.update({ error: '' })
        connection.append(`Agent request accepted: ${JSON.stringify({ request: text, status: 'admitted' })}`, false, task.delegationId)
        stream.follow(task)
        this.refresh()
      }).catch((error: Error) => {
        if (abort.signal.aborted) {
          return
        }
        connection.append(`Could not confirm agent input: ${error.message}`, true, task.delegationId)
        this.update({ error: error.message })
      })
    })

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
            if (!transcripts.has(message.id)) {
              trace('transcript_started', { event_id: message.id, detail: message.role })
              transcripts.add(message.id)
            }
            if (event.type === 'transcript_done') {
              trace('transcript_final', { event_id: message.id, detail: message.role })
            }
            admission.transcript(event, message.id)
            this.writer.append(message, event.type === 'transcript_done')
            this.update({ transcript: recorder.lines })
          }
        }
        if (abort.signal.aborted) {
          return
        }
        switch (event.type) {
          case 'ready': {
            this.update({ phase: 'listening', analyser: connection.analyser, outputAnalyser: connection.outputAnalyser })
            stream.start()
            break
          }
          case 'turn_started':
            if (event.role === 'user') {
              admission.begin(event.userTurnId)
            }
            break
          case 'delegation':
            admission.delegate(event)
            break
        }
      }, trace)
      this.connection = connection
      void connection.start(this.sessionId, () => stream.context()).catch((error: Error) => {
        if (abort.signal.aborted) {
          return
        }
        connection.dispose()
        this.connection = undefined
        this.stop()
        this.update({ ...initialVoiceState, phase: 'error', error: error.message })
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
