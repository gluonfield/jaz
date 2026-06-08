// Streaming MP3 playback over MediaSource Extensions: chunks append as they
// arrive, so audio starts before the response finishes downloading.
export class StreamingPlayer {
  private audio: HTMLAudioElement | null = null
  private objectUrl: string | null = null
  private stopped = false

  // WebAudio tap so the UI can visualize the assistant's voice. Optional: if
  // routing fails the element still plays, just without a live level.
  private ctx: AudioContext | null = null
  private srcNode: MediaElementAudioSourceNode | null = null
  analyser: AnalyserNode | null = null
  private freqBuf = new Uint8Array(0)

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
    this.attachAnalyser(audio)

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

  // Loudness of the current playback frame, 0..1, for the visualizer.
  level(): number {
    if (!this.analyser) return 0
    if (this.freqBuf.length !== this.analyser.frequencyBinCount) {
      this.freqBuf = new Uint8Array(this.analyser.frequencyBinCount)
    }
    this.analyser.getByteFrequencyData(this.freqBuf)
    let sum = 0
    for (let i = 0; i < this.freqBuf.length; i++) sum += this.freqBuf[i]
    return sum / this.freqBuf.length / 255
  }

  stop(): void {
    this.stopped = true
    this.cleanup()
  }

  private attachAnalyser(audio: HTMLAudioElement): void {
    try {
      this.ctx = new AudioContext()
      void this.ctx.resume()
      const src = this.ctx.createMediaElementSource(audio)
      this.srcNode = src
      this.analyser = this.ctx.createAnalyser()
      this.analyser.fftSize = 1024
      this.analyser.smoothingTimeConstant = 0.82
      src.connect(this.analyser)
      this.analyser.connect(this.ctx.destination)
    } catch {
      // Keep audio audible even if the WebAudio graph couldn't be built.
      try {
        this.srcNode?.connect(this.ctx?.destination as AudioNode)
      } catch {
        // nothing more to do; element playback is the fallback
      }
      this.analyser = null
    }
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
    this.srcNode?.disconnect()
    this.srcNode = null
    this.analyser?.disconnect()
    this.analyser = null
    if (this.ctx) {
      void this.ctx.close()
      this.ctx = null
    }
  }
}
