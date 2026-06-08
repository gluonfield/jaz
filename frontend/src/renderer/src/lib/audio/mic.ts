// One microphone stream for a whole voice session: it stays open while we
// record discrete utterances off it, and exposes a live analyser so the UI can
// react to the actual input level (level meter + silence detection / barge-in).
export class Mic {
  private stream: MediaStream | null = null
  private ctx: AudioContext | null = null
  private source: MediaStreamAudioSourceNode | null = null
  private recorder: MediaRecorder | null = null
  private chunks: Blob[] = []
  private timeBuf = new Float32Array(0)

  analyser: AnalyserNode | null = null

  async open(): Promise<void> {
    if (this.stream) return
    // echo cancellation lets the open mic coexist with playback for barge-in.
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    })
    this.ctx = new AudioContext()
    await this.ctx.resume()
    this.source = this.ctx.createMediaStreamSource(this.stream)
    this.analyser = this.ctx.createAnalyser()
    this.analyser.fftSize = 1024
    this.analyser.smoothingTimeConstant = 0.8
    this.source.connect(this.analyser)
    this.timeBuf = new Float32Array(this.analyser.fftSize)
  }

  // Root-mean-square amplitude of the current frame, ~0 (silence) to ~1 (loud).
  level(): number {
    if (!this.analyser) return 0
    this.analyser.getFloatTimeDomainData(this.timeBuf)
    let sum = 0
    for (let i = 0; i < this.timeBuf.length; i++) sum += this.timeBuf[i] * this.timeBuf[i]
    return Math.sqrt(sum / this.timeBuf.length)
  }

  get capturing(): boolean {
    return this.recorder?.state === 'recording'
  }

  beginCapture(): void {
    if (!this.stream) throw new Error('mic not open')
    if (this.capturing) return
    this.chunks = []
    this.recorder = new MediaRecorder(this.stream, { mimeType: 'audio/webm;codecs=opus' })
    this.recorder.ondataavailable = (e) => {
      if (e.data.size > 0) this.chunks.push(e.data)
    }
    this.recorder.start()
  }

  endCapture(): Promise<Blob> {
    return new Promise((resolve, reject) => {
      const recorder = this.recorder
      if (!recorder || recorder.state === 'inactive') {
        reject(new Error('not capturing'))
        return
      }
      recorder.onstop = () => {
        const blob = new Blob(this.chunks, { type: 'audio/webm' })
        this.recorder = null
        this.chunks = []
        resolve(blob)
      }
      recorder.stop()
    })
  }

  cancelCapture(): void {
    if (this.recorder && this.recorder.state !== 'inactive') {
      this.recorder.onstop = null
      this.recorder.stop()
    }
    this.recorder = null
    this.chunks = []
  }

  close(): void {
    this.cancelCapture()
    this.stream?.getTracks().forEach((track) => track.stop())
    this.stream = null
    void this.ctx?.close()
    this.ctx = null
    this.source = null
    this.analyser = null
  }
}
