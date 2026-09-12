import { connectVoice, type VoiceProvider } from '@/lib/api/liveVoice'
import { decodeVoiceEvent, voiceContextEvents, type VoiceEvent } from '@/lib/voice/protocol'
import { requestMicrophone } from '@/lib/voice/microphone'

export class VoiceConnection {
  private peer = new RTCPeerConnection()
  private channel = this.peer.createDataChannel('oai-events')
  private abort = new AbortController()
  private microphone?: MediaStream
  private audio = new Audio()
  private context = new AudioContext()
  private provider?: VoiceProvider
  private ready = false
  private ending = false
  private closed = false
  private timer?: ReturnType<typeof setTimeout>
  readonly analyser = this.context.createAnalyser()

  constructor(private onEvent: (event: VoiceEvent) => void) {
    this.audio.autoplay = true
    this.analyser.fftSize = 256
    this.channel.onmessage = ({ data }: MessageEvent<string>) => {
      try {
        const event = decodeVoiceEvent(data)
        if (!event || this.closed) return
        if (event.type === 'ready') {
          this.ready = true
          clearTimeout(this.timer)
        }
        if (event.type === 'closed') this.dispose()
        this.onEvent(event)
      } catch {
        this.fail('The voice provider sent an invalid event.')
      }
    }
    this.channel.onclose = () => {
      if (this.closed) return
      if (this.ending) {
        this.dispose()
        this.onEvent({ type: 'closed' })
      } else this.fail('Voice disconnected. Start voice again to reconnect.')
    }
    this.peer.onconnectionstatechange = () => {
      if (this.peer.connectionState === 'failed') this.fail('Voice disconnected. Start voice again to reconnect.')
    }
    this.peer.ontrack = ({ track }) => {
      if (track.kind !== 'audio' || this.closed) return
      const stream = new MediaStream([track])
      this.audio.srcObject = stream
      this.context.createMediaStreamSource(stream).connect(this.analyser)
      void this.audio.play().catch(() => this.fail('Audio playback was blocked. Start voice again to allow playback.'))
    }
  }

  async start(loadContext: () => Promise<string>) {
    this.timer = setTimeout(() => this.fail('Connecting voice timed out. Please try again.'), 45_000)
    const [chatContext] = await Promise.all([loadContext(), this.prepareMicrophone()])
    if (this.closed || this.ending) return
    const sdp = this.peer.localDescription?.sdp
    if (!sdp) throw new Error('Could not prepare the microphone connection.')
    const result = await connectVoice(sdp, chatContext, this.abort.signal)
    if (this.closed || this.ending) return
    this.provider = result.provider
    await this.peer.setRemoteDescription({ type: 'answer', sdp: result.sdp })
  }

  private async prepareMicrophone() {
    await this.context.resume()
    if (this.closed || this.ending) return
    const microphone = await requestMicrophone()
    if (this.closed || this.ending) {
      microphone.getTracks().forEach((track) => track.stop())
      return
    }
    this.microphone = microphone
    for (const track of microphone.getAudioTracks()) {
      this.peer.addTrack(track, microphone)
      track.onended = () => this.fail('Microphone disconnected. Start voice again to reconnect.')
    }
    this.context.createMediaStreamSource(microphone).connect(this.analyser)
    await this.peer.setLocalDescription(await this.peer.createOffer())
    await this.gatherICE()
    if (this.closed || this.ending) return
  }

  append(text: string, speak = false, delegationId?: string) {
    if (!this.ready || this.ending || this.closed || !this.provider) return
    for (const event of voiceContextEvents(this.provider, text, speak, delegationId)) {
      this.channel.send(JSON.stringify(event))
    }
  }

  mute(muted: boolean) {
    this.microphone?.getAudioTracks().forEach((track) => { track.enabled = !muted })
  }

  muteSpeaker(muted: boolean) {
    this.audio.muted = muted
  }

  end() {
    if (this.closed || this.ending) return
    this.ending = true
    this.mute(true)
    this.audio.muted = true
    clearTimeout(this.timer)
    if (this.ready && this.channel.readyState === 'open') {
      this.channel.send(JSON.stringify({ type: 'session.close' }))
      this.timer = setTimeout(() => {
        this.dispose()
        this.onEvent({ type: 'closed' })
      }, 3000)
    } else {
      this.dispose()
      this.onEvent({ type: 'closed' })
    }
  }

  dispose() {
    if (this.closed) return
    this.closed = true
    clearTimeout(this.timer)
    this.abort.abort()
    this.microphone?.getTracks().forEach((track) => track.stop())
    this.audio.pause()
    this.audio.srcObject = null
    this.channel.close()
    this.peer.close()
    void this.context.close()
  }

  private fail(message: string) {
    if (this.closed) return
    this.dispose()
    this.onEvent({ type: 'error', message })
  }

  private gatherICE(): Promise<void> {
    if (this.peer.iceGatheringState === 'complete') return Promise.resolve()
    return new Promise((resolve, reject) => {
      const cleanup = () => {
        this.peer.removeEventListener('icegatheringstatechange', changed)
        this.abort.signal.removeEventListener('abort', aborted)
        clearTimeout(timeout)
      }
      const changed = () => {
        if (this.peer.iceGatheringState !== 'complete') return
        cleanup()
        resolve()
      }
      const aborted = () => {
        cleanup()
        reject(new DOMException('Voice ended', 'AbortError'))
      }
      const timeout = setTimeout(() => {
        cleanup()
        reject(new Error('Microphone connection timed out.'))
      }, 10_000)
      this.peer.addEventListener('icegatheringstatechange', changed)
      this.abort.signal.addEventListener('abort', aborted, { once: true })
      changed()
      if (this.abort.signal.aborted) aborted()
    })
  }
}
