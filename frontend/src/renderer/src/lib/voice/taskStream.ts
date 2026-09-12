import type { SessionEvent, SessionMessages } from '@/lib/api/types'
import { coalesceSessionEvents, mergeSessionEvent } from '@/lib/sessionEvents'
import { taskFinished, voiceAgentRunning, voiceReplyChunks, voiceTaskUpdate, type VoiceTask } from '@/lib/voice/delegation'
import { voiceChatContext } from '@/lib/voice/transcript'

type Source = {
  read: () => Promise<SessionMessages>
  subscribe: (after: number, onEvent: (event: SessionEvent) => void, onConnection: (connected: boolean) => void) => () => void
}

export class VoiceTaskStream {
  private snapshot?: SessionMessages
  private tasks: VoiceTask[] = []
  private stopEvents = () => {}
  private reading?: Promise<void>
  private dirty = false
  private stopped = false
  private lastContext = ''
  private retry?: ReturnType<typeof setTimeout>

  constructor(
    private source: Source,
    private callId: string,
    private append: (text: string, speak: boolean, id?: string) => void,
    private onState: (working: boolean, error: string) => void,
  ) {}

  async context() {
    this.snapshot = await this.source.read()
    this.snapshot.events = coalesceSessionEvents(this.snapshot.events)
    this.lastContext = voiceChatContext(this.snapshot)
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
      if (!event.type.startsWith('acp')) {
        return
      }
      this.apply(this.snapshot, event)
      this.deliver(this.tasks.filter((task) => task.user))
    }, (connected) => {
      if (connected) {
        void this.refresh()
      } else if (!this.stopped) {
        this.onState(this.tasks.length > 0, 'Reconnecting to agent updates…')
      }
    })
  }

  follow(task: VoiceTask) {
    this.tasks.push(task)
    void this.refresh()
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
        snapshot.events = coalesceSessionEvents(snapshot.events)
        for (const event of [...this.snapshot?.events ?? []].sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))) {
          if ((event.seq ?? 0) > snapshot.latest_event_seq) {
            this.apply(snapshot, { ...event, projection_op: 'replace' })
          }
        }
        this.snapshot = snapshot
        this.deliver(following.filter((task) => this.tasks.includes(task)))
        this.onState(this.tasks.length > 0, '')
      } catch (error) {
        if (!this.stopped) {
          this.onState(this.tasks.length > 0, `Couldn't follow agent work: ${(error as Error).message}`)
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
    snapshot.acp_tool_calls = event.acp?.tool_calls ?? snapshot.acp_tool_calls
  }

  private deliver(tasks: VoiceTask[]) {
    const snapshot = this.snapshot!
    for (const task of [...tasks]) {
      const update = voiceTaskUpdate(task, snapshot)
      const finished = taskFinished(update)
      const chunks = update.state === 'failed' || update.state === 'cancelled' ? [] : voiceReplyChunks(task, snapshot, update.state === 'completed')
      for (const chunk of chunks) {
        this.append(chunk, true, task.id)
      }
      const progress = `${update.state}:${update.text}`
      if (progress !== task.progress) {
        task.progress = progress
        const hasOutput = Boolean(task.output?.size)
        const speak = update.state === 'approval' || update.state === 'failed' || update.state === 'cancelled' || (update.state === 'completed' && !hasOutput)
        const text = update.state === 'completed' && hasOutput ? 'Agent work completed.' : `Agent task ${update.state}: ${update.text.slice(0, 12000)}`
        this.append(text, speak, task.id)
      }
      if (finished) {
        this.tasks.splice(this.tasks.indexOf(task), 1)
        this.onState(this.tasks.length > 0, '')
      }
    }
    if (!this.tasks.length && !voiceAgentRunning(snapshot)) {
      const context = voiceChatContext(snapshot, this.callId)
      if (context !== this.lastContext) {
        this.append(context, false)
        this.lastContext = context
      }
    }
  }

  stop() {
    this.stopped = true
    this.stopEvents()
    clearTimeout(this.retry)
  }
}
