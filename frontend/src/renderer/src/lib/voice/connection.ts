import { getVoiceSettings } from '@/lib/api/liveVoice'
import type { VoiceEvent } from '@/lib/voice/protocol'
import { OpenAIVoiceSignaling } from '@/lib/voice/openaiSignaling'
import { ClaudeVoiceSignaling } from '@/lib/voice/claudeSignaling'
import { requestMicrophone } from '@/lib/voice/microphone'
import type { VoiceTrace } from '@/lib/voice/diagnostics'
import type { VoiceSignaling } from '@/lib/voice/signaling'

export class VoiceConnection {
  private peer = new RTCPeerConnection()
  private signaling?: VoiceSignaling
  private abort = new AbortController()
  private microphone?: MediaStream
  private audio = new Audio()
  private context = new AudioContext()
  private ending = false
  private closed = false
  private timer?: ReturnType<typeof setTimeout>
  readonly analyser = this.context.createAnalyser()
  readonly outputAnalyser = this.context.createAnalyser()

  constructor(private onEvent: (event: VoiceEvent) => void, private trace: VoiceTrace) {
    this.audio.autoplay = true
    for (const type of ['playing', 'waiting', 'pause', 'ended']) {
      this.audio.addEventListener(type, () => this.trace(`playback_${type}`, { detail: this.audio.muted ? 'muted media element' : 'media element' }))
    }
    this.analyser.fftSize = 256
    this.outputAnalyser.fftSize = 256
    this.peer.onconnectionstatechange = () => {
      if (this.peer.connectionState === 'failed') this.fail('Voice disconnected. Start voice again to reconnect.')
    }
    this.peer.ontrack = ({ track }) => {
      if (track.kind !== 'audio' || this.closed) return
      const stream = new MediaStream([track])
      this.audio.srcObject = stream
      this.context.createMediaStreamSource(stream).connect(this.outputAnalyser)
      void this.audio.play().catch(() => this.fail('Audio playback was blocked. Start voice again to allow playback.'))
    }
  }

  async start(sessionId: string, loadContext: () => Promise<string>) {
    this.timer = setTimeout(() => this.fail('Connecting voice timed out. Please try again.'), 45_000)
    const [settings, chatContext] = await Promise.all([getVoiceSettings(), loadContext(), this.prepareMicrophone()])
    if (this.closed || this.ending) {
      return
    }
    if (settings.provider === 'claude') {
      const provider = settings.providers.find((provider) => provider.id === 'claude')
      if (!provider?.available || !settings.models?.some((model) => model.id === settings.model)) {
        throw new Error(provider?.reason ?? 'Choose an available Claude voice model in Settings → Voice.')
      }
    }
    const event = (event: VoiceEvent) => {
      if (this.closed) {
        return
      }
      if (event.type === 'ready') {
        clearTimeout(this.timer)
      }
      if (event.type === 'closed' || event.type === 'error') {
        this.dispose()
      }
      this.onEvent(event)
    }
    this.signaling = settings.provider === 'claude'
      ? new ClaudeVoiceSignaling(this.peer, event, this.trace, settings.voice)
      : new OpenAIVoiceSignaling(this.peer, event, this.trace)
    if (this.closed || this.ending) {
      return
    }
    await this.peer.setLocalDescription(await this.peer.createOffer())
    await this.gatherICE()
    if (this.closed || this.ending) {
      return
    }
    const sdp = this.peer.localDescription?.sdp
    if (!sdp) {
      throw new Error('Could not prepare the microphone connection.')
    }
    await this.signaling.start(sessionId, sdp, chatContext)
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
  }

  append(text: string, speak = false, delegationId?: string, sourceSeq?: number): void {
    this.signaling?.append(text, speak, delegationId, sourceSeq)
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
    if (this.signaling) {
      this.signaling.end()
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
    this.signaling?.dispose()
    this.peer.close()
    void this.context.close()
  }

  private fail(message: string) {
    if (this.closed) return
    this.trace('connection_failed', { detail: message })
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
