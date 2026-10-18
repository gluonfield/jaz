import { MessageSquareText, Mic, MicOff, RotateCcw, Volume2, VolumeX, X } from 'lucide-react'
import { type HTMLMotionProps, motion } from 'motion/react'
import { useEffect, useRef } from 'react'
import type { BotAvatar as Face } from '@shared/bots'
import { BotAvatar } from '@/components/bots/BotAvatar'
import { VOICE_COLOR, VoiceVisualizer } from '@/components/session/VoiceVisualizer'
import { botInk } from '@/lib/bots'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { VoiceLevels } from '@/lib/voice/audioLevel'
import type { VoiceHandle } from '@/lib/voice/session'

const WAVE_DOTS = 12

// Voice mode in a thread: the voice pill over the composer, and what the
// connection is doing when it is not simply live.
export function VoiceMode({ voice, face }: { voice: VoiceHandle; face: Face | null }) {
  if (voice.phase === 'off') {
    return voice.error ? <p role="alert" className="mb-3 text-center text-xs text-danger">{voice.error}</p> : null
  }
  const status = voice.phase === 'connecting' ? 'Connecting voice…'
    : voice.phase === 'ending' ? 'Ending voice…'
    : voice.phase === 'error' ? 'Voice disconnected'
    : ''

  return (
    <div className="mb-3 flex flex-col items-center" aria-label="Voice conversation">
      <VoicePill voice={voice} face={face} />
      {status ? <p role="status" aria-atomic="true" className="mt-1.5 text-xs text-ink-2">{status}</p> : null}
      {voice.error ? <p role="alert" className="mt-1 max-w-sm text-center text-xs text-danger">{voice.error}</p> : null}
    </div>
  )
}

// Voice as one pill: the face of whoever is talking (a bot's own, or Jaz's), a
// dotted line in its colour that moves with the sound, and the controls,
// ending in red.
export function VoicePill({ voice, face, level = 0, outputLevel = 0, onReturn, transcript }: {
  voice: VoiceHandle
  face: Face | null
  level?: number
  outputLevel?: number
  onReturn?: () => void
  transcript?: { shown: boolean; toggle: () => void }
}) {
  const avatar = face
    ? <BotAvatar avatar={face} size={34} mood={voice.phase === 'connecting' || voice.activity ? 'working' : 'idle'} live />
    : <VoiceVisualizer voice={voice} size={44} level={level} outputLevel={outputLevel} />
  return (
    <div className="flex items-center gap-2 rounded-full border border-border bg-bg p-1.5 shadow-(--shadow-raised) dark:bg-surface">
      {onReturn ? (
        <button
          type="button"
          aria-label="Return to voice chat"
          title={voice.error || 'Open voice chat · Drag to move'}
          // A pointer click belongs to the floating pill's drag gesture, so the
          // button itself answers only the keyboard.
          onClick={(event) => {
            if (event.detail === 0) {
              onReturn()
            }
          }}
          className="grid size-10 shrink-0 place-items-center rounded-full"
        >
          {avatar}
        </button>
      ) : (
        <span className="grid size-10 shrink-0 place-items-center">{avatar}</span>
      )}
      <VoiceWave voice={voice} color={face ? botInk(face.color) : VOICE_COLOR} level={level} outputLevel={outputLevel} />
      <div data-voice-control className="flex items-center gap-1.5">
        {transcript ? (
          <PillButton label={transcript.shown ? 'Hide transcript' : 'Show transcript'} pressed={transcript.shown} onClick={transcript.toggle}>
            <MessageSquareText size={17} />
          </PillButton>
        ) : null}
        {voice.phase === 'error' ? (
          <PillButton label="Reconnect voice" onClick={voice.start}>
            <RotateCcw size={17} />
          </PillButton>
        ) : (
          <PillButton label={voice.muted ? 'Unmute microphone' : 'Mute microphone'} pressed={voice.muted} disabled={voice.phase !== 'listening'} onClick={voice.mute}>
            {voice.muted ? <MicOff size={17} /> : <Mic size={17} />}
          </PillButton>
        )}
        <PillButton label={voice.speakerMuted ? 'Unmute speaker' : 'Mute speaker'} pressed={voice.speakerMuted} disabled={voice.phase !== 'listening'} onClick={voice.muteSpeaker}>
          {voice.speakerMuted ? <VolumeX size={17} /> : <Volume2 size={17} />}
        </PillButton>
        <PillButton label="Close voice mode" danger disabled={voice.phase === 'ending'} onClick={voice.end}>
          <X size={18} />
        </PillButton>
      </div>
    </div>
  )
}

function PillButton({ label, pressed, danger = false, ...props }: {
  label: string
  pressed?: boolean
  danger?: boolean
} & HTMLMotionProps<'button'>) {
  return (
    <motion.button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={pressed}
      whileTap={props.disabled ? undefined : { scale: 0.96 }}
      className={`grid size-10 shrink-0 cursor-pointer place-items-center rounded-full transition-colors duration-150 disabled:cursor-default disabled:opacity-50 ${
        danger ? 'bg-danger-solid text-white hover:bg-danger-solid/90' : 'bg-ink/8 text-ink-2 hover:bg-ink/12 hover:text-ink aria-pressed:bg-ink/15 aria-pressed:text-ink'
      }`}
      {...props}
    />
  )
}

// A dotted line that rises into a wave while anyone speaks.
function VoiceWave({ voice, color, level, outputLevel }: {
  voice: VoiceHandle
  color: string
  level: number
  outputLevel: number
}) {
  const reducedMotion = useReducedEffectsMotion()
  const dots = useRef<(HTMLSpanElement | null)[]>([])
  const input = useRef({ voice, level, outputLevel })

  useEffect(() => {
    input.current = { voice, level, outputLevel }
  }, [voice, level, outputLevel])

  useEffect(() => {
    if (reducedMotion) {
      return
    }
    const levels = new VoiceLevels()
    let loudness = 0
    let raf = 0
    const draw = (now: number) => {
      const { voice, level, outputLevel } = input.current
      const { input: heard, output: spoken } = levels.read(voice, level, outputLevel)
      loudness += (Math.min(1, Math.max(heard, spoken) * 2.5) - loudness) * 0.25
      dots.current.forEach((dot, index) => {
        if (dot) {
          dot.style.height = `${4 + loudness * 18 * (0.5 + 0.5 * Math.sin(now / 150 + index * 0.9))}px`
        }
      })
      raf = requestAnimationFrame(draw)
    }
    raf = requestAnimationFrame(draw)
    return () => cancelAnimationFrame(raf)
  }, [reducedMotion])

  return (
    <div aria-hidden className="mx-1 flex h-6 w-24 shrink-0 items-center justify-between">
      {Array.from({ length: WAVE_DOTS }, (_, index) => (
        <span
          key={index}
          ref={(dot) => {
            dots.current[index] = dot
          }}
          className="h-1 w-1 rounded-full opacity-60"
          style={{ background: color }}
        />
      ))}
    </div>
  )
}
