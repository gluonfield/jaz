import { answerSessionInteractiveResponse, getSessionMessagesPage } from '@/lib/api/sessions'
import { ApiError } from '@/lib/api/response'
import { saveVoiceDiagnostics, saveVoiceTranscript, sendVoiceTask } from '@/lib/api/liveVoice'
import { VoiceConnection } from '@/lib/voice/connection'
import { openSessionEvents } from '@/lib/api/sse'
import type { VoiceTask } from '@/lib/voice/delegation'
import { VoiceTaskStream } from '@/lib/voice/taskStream'
import { voiceQuestionAnswer, type VoiceEvent } from '@/lib/voice/protocol'
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
    const seen = new Set<string>()
    const pending = new Map<string, Extract<VoiceEvent, { type: 'delegation' }>>()
    let submit = Promise.resolve()
    let undeliveredContext = ''
    let followError = ''
    const answered = new Set<string>()
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

    const delegate = (event: Extract<VoiceEvent, { type: 'delegation' }>) => {
      const turnId = event.userTurnId ?? event.id
      if (seen.has(turnId)) {
        return
      }
      const user = recorder.lines.findLast((line) => line.role === 'user')
      const answer = voiceQuestionAnswer(event.text)
      const answerKey = answer && user && `${answer.request_id}:${answer.question_id}:${user.id}`
      if (answerKey && answered.has(answerKey)) {
        seen.add(turnId)
        return
      }
      const request = recorder.request(answer ? { ...event, text: answer.answer } : event)
      if (!request) {
        pending.set(turnId, event)
        return
      }
      pending.delete(turnId)
      seen.add(turnId)
      if (answerKey) {
        answered.add(answerKey)
      }
      submit = submit.then(async () => {
        if (abort.signal.aborted) {
          return
        }
        undeliveredContext = [undeliveredContext, request.context || `User: ${request.text}`].filter(Boolean).join('\n')
        trace('admission_start', { request_id: event.id, user_turn_id: event.userTurnId, detail: answer ? 'question answer' : 'agent input' })
        if (answer && stream.hasQuestion(answer.request_id, answer.question_id)) {
          try {
            await answerSessionInteractiveResponse(this.sessionId, { request_id: answer.request_id, answers: { [answer.question_id]: { answers: [answer.answer] } } })
            undeliveredContext = ''
            trace('question_answered', { request_id: event.id, detail: answer.request_id })
            connection.append(`User answered the agent's question: ${JSON.stringify(answer.answer)}. The agent can now continue.`, false)
            await stream.refresh()
            return
          } catch (error) {
            if (!(error instanceof ApiError) || error.status !== 400) {
              throw error
            }
            await stream.refresh()
            if (stream.hasQuestion(answer.request_id, answer.question_id)) {
              throw error
            }
            trace('question_stale', { request_id: event.id, detail: answer.request_id })
          }
        }
        const contexts = [{ type: 'voice' as const, id: recorder.callId, request_id: event.id, text: undeliveredContext }]
        const task: VoiceTask = { id: event.id, callId: recorder.callId }
        await sendVoiceTask(this.sessionId, request.text, contexts, abort.signal)
        trace('admitted', { request_id: event.id, user_turn_id: event.userTurnId })
        undeliveredContext = ''
        if (abort.signal.aborted) {
          return
        }
        this.update({ error: '' })
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
            if (!transcripts.has(message.id)) {
              trace('transcript_started', { event_id: message.id, detail: message.role })
              transcripts.add(message.id)
            }
            if (event.type === 'transcript_done') {
              trace('transcript_final', { event_id: message.id, detail: message.role })
            }
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
