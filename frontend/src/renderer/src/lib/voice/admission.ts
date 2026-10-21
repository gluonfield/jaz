import type { VoiceTask } from '@/lib/voice/delegation'
import type { VoiceEvent } from '@/lib/voice/protocol'
import type { VoiceRecorder } from '@/lib/voice/recorder'
import type { VoiceQuestionContext } from '@/lib/voice/question'

type Turn = {
  task: VoiceTask
  question: VoiceQuestionContext | undefined
  final?: string
  delegation?: Extract<VoiceEvent, { type: 'delegation' }>
  submitted?: boolean
}

export class VoiceAdmission {
  private turns = new Map<string, Turn>()
  private consumed = new Set<string>()
  private activeId?: string

  constructor(
    private recorder: VoiceRecorder,
    private question: () => VoiceQuestionContext | undefined,
    private submit: (task: VoiceTask, text: string, context: string) => void,
  ) {}

  begin(id: string) {
    this.activeId = id
    return this.capture(id)
  }

  private capture(id: string) {
    let turn = this.turns.get(id)
    if (!turn) {
      const question = this.question()
      turn = { task: { id, callId: this.recorder.callId }, question: question && !this.consumed.has(question.id) ? question : undefined }
      this.turns.set(id, turn)
    }
    return turn
  }

  transcript(event: Extract<VoiceEvent, { type: 'transcript' | 'transcript_done' }>, messageId: string) {
    if (event.role !== 'user') {
      return
    }
    const id = event.userTurnId ?? this.activeId ?? messageId
    const turn = this.capture(id)
    if (event.type === 'transcript_done') {
      turn.final = event.text
      this.admit(turn)
      if (this.activeId === id) {
        this.activeId = undefined
      }
    }
  }

  delegate(event: Extract<VoiceEvent, { type: 'delegation' }>) {
    const turn = this.capture(event.userTurnId ?? event.id)
    turn.task.delegationId ??= event.id
    turn.delegation = event
    this.admit(turn)
  }

  accepted(id: string) {
    const question = this.turns.get(id)?.question
    if (question) {
      this.consumed.add(question.id)
    }
  }

  private admit(turn: Turn) {
    if (turn.submitted || (!turn.question && !turn.delegation) || (turn.question && turn.final === undefined && (!turn.delegation || turn.delegation.turnBased))) {
      return
    }
    const request = this.recorder.request({ ...turn.delegation, type: 'delegation', id: turn.task.id, ...(turn.question && turn.final !== undefined ? { text: turn.final } : {}) })
    if (!request) {
      return
    }
    turn.submitted = true
    this.submit(turn.task, request.text, [turn.question?.text, request.context || `User: ${request.text}`].filter(Boolean).join('\n'))
  }
}
