// Streaming MP3 playback over MediaSource Extensions: chunks append as they
// arrive, so audio starts before the response finishes downloading.
export class StreamingPlayer {
  private audio: HTMLAudioElement | null = null
  private objectUrl: string | null = null
  private stopped = false

  async play(body: ReadableStream<Uint8Array>, onEnded?: () => void): Promise<void> {
    this.stop()
    this.stopped = false

    const mediaSource = new MediaSource()
    const audio = new Audio()
    this.audio = audio
    this.objectUrl = URL.createObjectURL(mediaSource)
    audio.src = this.objectUrl
    audio.onended = () => {
      this.cleanup()
      onEnded?.()
    }

    await new Promise<void>((resolve) => {
      mediaSource.addEventListener('sourceopen', () => resolve(), { once: true })
    })
    const buffer = mediaSource.addSourceBuffer('audio/mpeg')
    const append = (chunk: Uint8Array) =>
      new Promise<void>((resolve, reject) => {
        buffer.addEventListener('updateend', () => resolve(), { once: true })
        try {
          buffer.appendBuffer(chunk as BufferSource)
        } catch (err) {
          reject(err)
        }
      })

    const reader = body.getReader()
    let started = false
    try {
      for (;;) {
        const { done, value } = await reader.read()
        if (this.stopped) return
        if (done) break
        if (value && value.length > 0) {
          await append(value)
          if (!started) {
            started = true
            void audio.play()
          }
        }
      }
      if (mediaSource.readyState === 'open') mediaSource.endOfStream()
      if (!started) {
        this.cleanup()
        onEnded?.()
      }
    } catch (err) {
      this.cleanup()
      if (!this.stopped) throw err
    }
  }

  stop(): void {
    this.stopped = true
    this.cleanup()
  }

  private cleanup(): void {
    if (this.audio) {
      this.audio.onended = null
      this.audio.pause()
      this.audio.src = ''
      this.audio = null
    }
    if (this.objectUrl) {
      URL.revokeObjectURL(this.objectUrl)
      this.objectUrl = null
    }
  }
}
