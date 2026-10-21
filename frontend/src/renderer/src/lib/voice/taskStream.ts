import type { SessionEvent, SessionMessages } from '@/lib/api/types'
import { coalesceSessionEvents, mergeSessionEvent } from '@/lib/sessionEvents'
import { taskFinished, voiceAgentRunning, voiceReplyChunks, voiceTaskActivity, voiceTaskUpdate, type VoiceTask } from '@/lib/voice/delegation'
import { voiceChatContext, voiceToolContext } from '@/lib/voice/transcript'
import type { VoiceWorkActivity } from '@shared/voice'

type Source = {
  read: () => Promise<SessionMessages>
  subscribe: (after: number, onEvent: (event: SessionEvent) => void, onConnection: (connected: boolean) => void) => () => void
}

export class VoiceTaskStream {
  private snapshot?: SessionMessages
  private tasks: VoiceTask[] = []
  private completedTask?: VoiceTask
  private stopEvents = () => {}
  private reading?: Promise<void>
  private dirty = false
  private stopped = false
  private lastContext = ''
  private lastTools = ''
  private status?: { activity: VoiceWorkActivity; error: string }
  private retry?: ReturnType<typeof setTimeout>
  private outputTimer?: ReturnType<typeof setTimeout>

  constructor(
    private source: Source,
    private callId: string,
    private append: (text: string, speak: boolean, id?: string, sourceSeq?: number) => void,
    private onState: (activity: VoiceWorkActivity, error: string) => void,
  ) {}

  async context() {
    this.snapshot = await this.source.read()
    this.snapshot.events = coalesceSessionEvents(this.snapshot.events)
    this.lastContext = voiceChatContext(this.snapshot)
    this.lastTools = voiceToolContext(this.snapshot)
    return this.lastContext
  }

  start() {
    if (this.stopped || !this.snapshot) {
      return
    }
    this.stopEvents = this.source.subscribe(this.snapshot.latest_event_seq, (event) => {
      if (this.stopped || !this.snapshot || event.session_id !== this.snapshot.session.id || (event.acp?.id && event.acp.id !== event.session_id)) {
        return
      }
      if (event.type === 'session' || event.type === 'assistant' || event.permission) {
        void this.refresh()
        return
      }
      if (!event.type.startsWith('acp') && !event.room_message) {
        return
      }
      this.apply(this.snapshot, event)
      if (event.type === 'acp_message') {
        this.publish()
        this.outputTimer ??= setTimeout(() => {
          this.outputTimer = undefined
          this.deliver(this.tasks.filter((task) => task.user))
        }, 200)
      } else {
        this.deliver(this.tasks.filter((task) => task.user))
      }
    }, (connected) => {
      if (connected) {
        void this.refresh()
      } else if (!this.stopped) {
        this.publish('Reconnecting to agent updates…')
      }
    })
  }

  follow(task: VoiceTask) {
    this.completedTask = undefined
    this.tasks.push(task)
    this.publish()
    void this.refresh()
  }

  hasQuestion(requestId: string, questionId: string) {
    return this.snapshot?.acp_permissions?.some((permission) => permission.id === requestId && permission.status === 'pending' && permission.questions?.some((question) => question.id === questionId)) ?? false
  }

  refresh(): Promise<void> {
    this.dirty = true
    if (this.reading) {
      return this.reading
    }
    this.reading = this.read().finally(() => {
      this.reading = undefined
    })
    return this.reading
  }

  private async read() {
    while (this.dirty && !this.stopped) {
      this.dirty = false
      clearTimeout(this.retry)
      const following = [...this.tasks]
      try {
        const snapshot = await this.source.read()
        if (this.stopped) {
          return
        }
        if (this.snapshot?.session.source_type === 'bot' && snapshot.session.title !== this.snapshot.session.title) {
          this.append(`Current chat identity: the user renamed you to ${JSON.stringify(snapshot.session.title)}. This is now your own name.`, false)
        }
        snapshot.events = coalesceSessionEvents(snapshot.events)
        for (const event of [...this.snapshot?.events ?? []].sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))) {
          if ((event.seq ?? 0) > snapshot.latest_event_seq) {
            this.apply(snapshot, { ...event, projection_op: 'replace' })
          }
        }
        this.snapshot = snapshot
        this.deliver(following.filter((task) => this.tasks.includes(task)))
      } catch (error) {
        if (!this.stopped) {
          this.publish(`Couldn't follow agent work: ${(error as Error).message}`)
          this.retry = setTimeout(() => void this.refresh(), 1000)
        }
      }
    }
  }

  private apply(snapshot: SessionMessages, event: SessionEvent) {
    if (event.seq && event.seq <= snapshot.latest_event_seq) {
      return
    }
    snapshot.events = mergeSessionEvent(snapshot.events, event)
    snapshot.latest_event_seq = Math.max(snapshot.latest_event_seq, event.seq ?? 0)
    snapshot.acp_state = event.acp?.state ?? snapshot.acp_state
    snapshot.acp_permissions = event.acp?.permissions ?? snapshot.acp_permissions
  }

  private deliver(tasks: VoiceTask[]) {
    const snapshot = this.snapshot!
    const tools = voiceToolContext(snapshot)
    if (tools !== this.lastTools) {
      this.append(tools, false)
      this.lastTools = tools
    }
    for (const task of [...tasks]) {
      const update = voiceTaskUpdate(task, snapshot)
      const finished = taskFinished(update)
      const chunks = !finished || update.state === 'completed' ? voiceReplyChunks(task, snapshot) : []
      for (const chunk of chunks) {
        this.append(chunk.text, true, task.id, chunk.seq)
      }
      const progress = `${update.state}:${update.text}`
      if (progress !== task.progress) {
        task.progress = progress
        const hasOutput = Boolean(task.output?.size)
        const speak = update.state === 'approval' || update.state === 'failed' || update.state === 'cancelled' || (update.state === 'waiting' && !hasOutput) || (update.state === 'completed' && !hasOutput)
        const text = update.state === 'completed' && hasOutput ? 'Agent work completed.' : `Agent task ${update.state}: ${update.text.slice(0, 12000)}`
        this.append(text, speak, task.id)
      }
      if (finished) {
        if (update.state === 'completed') {
          this.completedTask = task
        }
        this.tasks.splice(this.tasks.indexOf(task), 1)
      }
    }
    if (!this.tasks.length && snapshot.session.source_type === 'bot' && this.completedTask) {
      for (const chunk of voiceReplyChunks(this.completedTask, snapshot)) {
        this.append(chunk.text, true, this.completedTask.id, chunk.seq)
      }
    }
    this.publish()
    if (!this.tasks.length && !voiceAgentRunning(snapshot)) {
      const context = voiceChatContext(snapshot, this.callId)
      if (context !== this.lastContext) {
        this.append(context, false)
        this.lastContext = context
      }
    }
  }

  private publish(error = '') {
    const task = this.tasks.at(-1)
    const activity = task ? voiceTaskActivity(task, this.snapshot!) : null
    if (activity !== this.status?.activity || error !== this.status?.error) {
      this.status = { activity, error }
      this.onState(activity, error)
    }
  }

  stop() {
    this.stopped = true
    this.stopEvents()
    clearTimeout(this.retry)
    clearTimeout(this.outputTimer)
  }
}
