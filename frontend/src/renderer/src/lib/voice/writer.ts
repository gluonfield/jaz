import type { VoiceMessage } from '@/lib/api/types'

export class VoiceTranscriptWriter {
  private pending = new Map<string, VoiceMessage>()
  private saving?: Promise<void>
  private timer?: ReturnType<typeof setTimeout>
  private retryDelay = 1000
  error = ''

  constructor(private save: (messages: VoiceMessage[]) => Promise<void>, private changed: () => void) {}

  append(message: VoiceMessage, final = false) {
    this.pending.set(`${message.call_id}:${message.id}`, message)
    clearTimeout(this.timer)
    if (final) {
      void this.flush()
    } else {
      this.timer = setTimeout(() => void this.flush(), 250)
    }
  }

  flush(): Promise<void> {
    clearTimeout(this.timer)
    if (!this.pending.size) {
      return this.saving ?? Promise.resolve()
    }
    this.saving ??= this.drain().finally(() => {
      this.saving = undefined
      if (this.pending.size && !this.error) {
        void this.flush()
      }
    })
    return this.saving
  }

  private async drain() {
    try {
      while (this.pending.size) {
        const batch: [string, VoiceMessage][] = []
        let bytes = 2
        for (const entry of this.pending.entries()) {
          const size = new TextEncoder().encode(JSON.stringify(entry[1])).length + 1
          if (batch.length && (batch.length === 32 || bytes + size > 256 * 1024)) {
            break
          }
          batch.push(entry)
          bytes += size
        }
        await this.save(batch.map(([, message]) => message))
        for (const [key, message] of batch) {
          if (this.pending.get(key) === message) {
            this.pending.delete(key)
          }
        }
        this.retryDelay = 1000
        this.error = ''
        this.changed()
      }
    } catch (error) {
      this.error = `Couldn't save voice conversation: ${(error as Error).message}. Retrying…`
      this.timer = setTimeout(() => void this.flush(), this.retryDelay)
      this.retryDelay = Math.min(this.retryDelay * 2, 30000)
      this.changed()
    }
  }
}
