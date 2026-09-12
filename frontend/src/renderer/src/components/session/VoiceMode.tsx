import { Mic, MicOff, RotateCcw, Volume2, VolumeX, X } from 'lucide-react'
import { IconButton } from '@/components/ui/IconButton'
import { VoiceVisualizer } from '@/components/session/VoiceVisualizer'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import type { VoiceHandle } from '@/lib/voice/session'

export function VoiceMode({ voice, size, level, outputLevel, compact = false, reducedMotion: reducedOverride }: { voice: VoiceHandle; size?: number; level?: number; outputLevel?: number; compact?: boolean; reducedMotion?: boolean }) {
  const reducedMotion = useReducedEffectsMotion()
  if (voice.phase === 'off') {
    return voice.error ? <p role="alert" className="mb-3 text-center text-xs text-danger">{voice.error}</p> : null
  }
  const status = voice.phase === 'connecting' ? 'Connecting voice…'
    : voice.phase === 'ending' ? 'Ending voice…'
    : voice.phase === 'error' ? 'Voice disconnected'
    : voice.muted ? 'Microphone muted' : ''
  const label = compact && voice.error ? voice.error : status

  return (
    <div className="mb-3 flex flex-col items-center" aria-label="Voice conversation">
      <div className="pointer-events-none">
        <VoiceVisualizer voice={voice} reducedMotion={reducedOverride ?? reducedMotion} size={size} level={level} outputLevel={outputLevel} />
      </div>
      <p title={voice.error || status} className="mt-1 flex h-5 max-w-[164px] items-center text-xs text-ink-2">
        {label ? <span role={compact && voice.error ? 'alert' : 'status'} aria-atomic="true" className="truncate">{label}</span> : null}
      </p>
      {voice.error && !compact ? <p role="alert" className="mt-1 max-w-sm text-center text-xs text-danger">{voice.error}</p> : null}
    </div>
  )
}

export function VoiceControls({ voice, floating = false }: { voice: VoiceHandle; floating?: boolean }) {
  const microphoneLabel = voice.muted ? 'Unmute microphone' : 'Mute microphone'
  const speakerLabel = voice.speakerMuted ? 'Unmute speaker' : 'Mute speaker'
  const hitArea = floating
    ? 'size-10! bg-ink! text-bg! hover:bg-ink/85! shadow-sm [-webkit-app-region:no-drag]'
    : 'relative after:absolute after:-inset-1'
  return (
    <>
      {voice.phase === 'error' ? (
        <IconButton size="md" className={hitArea} title="Reconnect voice" aria-label="Reconnect voice" onClick={voice.start}>
          <RotateCcw size={16} />
        </IconButton>
      ) : (
        <IconButton size="md" className={`${hitArea} ${voice.muted ? 'bg-surface-2 text-ink' : ''}`} title={microphoneLabel} aria-label={microphoneLabel} aria-pressed={voice.muted} disabled={voice.phase !== 'listening'} onClick={voice.mute}>
          {voice.muted ? <MicOff size={17} /> : <Mic size={17} />}
        </IconButton>
      )}
      <IconButton size="md" className={`${hitArea} bg-ink! text-bg! hover:bg-ink/85!`} title="Close voice mode" aria-label="Close voice mode" disabled={voice.phase === 'ending'} onClick={voice.end}>
        <X size={17} className="text-bg" />
      </IconButton>
      <IconButton size="md" className={`${hitArea} ${voice.speakerMuted ? 'bg-surface-2 text-ink' : ''}`} title={speakerLabel} aria-label={speakerLabel} aria-pressed={voice.speakerMuted} disabled={voice.phase !== 'listening'} onClick={voice.muteSpeaker}>
        {voice.speakerMuted ? <VolumeX size={17} /> : <Volume2 size={17} />}
      </IconButton>
    </>
  )
}
