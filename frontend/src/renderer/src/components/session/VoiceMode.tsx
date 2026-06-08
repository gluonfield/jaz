import { useQueryClient } from '@tanstack/react-query'
import { Mic, Pause, X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { streamSessionMessage } from '@/lib/api/stream'
import { speakStream, transcribeAudio } from '@/lib/api/voice'
import { Mic as Microphone } from '@/lib/audio/mic'
import { StreamingPlayer } from '@/lib/audio/player'
import { keys } from '@/lib/query/keys'
import { VoiceVisualizer } from './VoiceVisualizer'

type Phase = 'connecting' | 'listening' | 'thinking' | 'speaking' | 'paused' | 'error'

type Turn = { id: number; role: 'user' | 'assistant'; text: string }

const STATUS: Record<Phase, string> = {
  connecting: 'Starting microphone…',
  listening: 'Listening — just start talking',
  thinking: 'Thinking…',
  speaking: 'Speaking — talk over me to cut in',
  paused: 'Paused',
  error: 'Something went wrong',
}

// Voice-activity thresholds (RMS, 0..1). Hysteresis + a hangover window let the
// loop decide on its own when you've finished a sentence.
const SPEECH_ON = 0.045
const SILENCE_HANGOVER_MS = 1100
const MIN_SPEECH_MS = 250
const MAX_UTTERANCE_MS = 30_000
// Barge-in: louder + sustained, and only after a short guard so the assistant's
// own opening syllable (bleeding through echo cancellation) can't trigger it.
const BARGE_ON = 0.08
const BARGE_SUSTAIN_MS = 320
const BARGE_GUARD_MS = 500

export function VoiceMode({ sessionId, onExit }: { sessionId: string; onExit: () => void }) {
  const queryClient = useQueryClient()
  const reducedMotion = useReducedMotion()
  const [phase, setPhase] = useState<Phase>('connecting')
  const [turns, setTurns] = useState<Turn[]>([])
  const [live, setLive] = useState('') // streaming assistant reply, pre-commit
  const [error, setError] = useState('')

  const micRef = useRef<Microphone | null>(null)
  const playerRef = useRef<StreamingPlayer | null>(null)
  if (!micRef.current) micRef.current = new Microphone()
  if (!playerRef.current) playerRef.current = new StreamingPlayer()

  // Loop state lives in refs so the rAF tick reads it without re-subscribing.
  const phaseRef = useRef<Phase>('connecting')
  const exitedRef = useRef(false)
  const busyRef = useRef(false)
  const controllerRef = useRef<AbortController | null>(null)
  const hadSpeechRef = useRef(false)
  const captureStartRef = useRef(0)
  const lastVoiceRef = useRef(0)
  const speakStartRef = useRef(0)
  const bargeStartRef = useRef(0)
  const turnSeq = useRef(0)
  const captionsRef = useRef<HTMLDivElement>(null)
  const controlRef = useRef<{ pause: () => void; resume: () => void } | null>(null)

  const setPhaseBoth = (p: Phase) => {
    phaseRef.current = p
    setPhase(p)
  }

  const pushTurn = (role: Turn['role'], text: string) =>
    setTurns((prev) => [...prev, { id: turnSeq.current++, role, text }])

  // keep the caption rail pinned to the newest line
  useEffect(() => {
    const el = captionsRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [turns, live])

  useEffect(() => {
    exitedRef.current = false
    const mic = micRef.current!
    const player = playerRef.current!

    const beginListening = () => {
      if (exitedRef.current) return
      busyRef.current = false
      hadSpeechRef.current = false
      const now = performance.now()
      captureStartRef.current = now
      lastVoiceRef.current = now
      try {
        mic.beginCapture()
        setPhaseBoth('listening')
      } catch {
        fail('Microphone unavailable — check system permissions.')
      }
    }

    const fail = (message: string) => {
      controllerRef.current?.abort()
      mic.cancelCapture()
      player.stop()
      setError(message)
      setPhaseBoth('error')
    }

    const finishUtterance = async () => {
      if (busyRef.current || exitedRef.current) return
      busyRef.current = true
      setPhaseBoth('thinking')
      const controller = new AbortController()
      controllerRef.current = controller
      try {
        const blob = await mic.endCapture()
        const text = (await transcribeAudio(blob, controller.signal)).trim()
        if (exitedRef.current) return
        if (!text) {
          beginListening() // heard noise, not words — keep the floor open
          return
        }
        pushTurn('user', text)
        setLive('')

        let reply = ''
        await streamSessionMessage({
          sessionId,
          message: text,
          voice: true,
          signal: controller.signal,
          onEvent: (event) => {
            if (event.type === 'delta' && event.delta) {
              reply += event.delta
              setLive(reply)
            }
            if (event.type === 'error' && event.error) throw new Error(event.error)
          },
        })
        if (exitedRef.current) return
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        if (!reply.trim()) {
          setLive('')
          beginListening()
          return
        }
        pushTurn('assistant', reply)
        setLive('')

        setPhaseBoth('speaking')
        speakStartRef.current = performance.now()
        bargeStartRef.current = 0
        const res = await speakStream(reply, controller.signal)
        if (exitedRef.current) {
          res.body?.cancel()
          return
        }
        await player.play(res.body!, () => {
          if (!exitedRef.current && phaseRef.current === 'speaking') beginListening()
        })
      } catch (err) {
        if (exitedRef.current || controller.signal.aborted) return
        fail(err instanceof Error ? err.message : String(err))
      }
    }

    // single rAF: silence detection while listening, barge-in while speaking
    let raf = 0
    const tick = (now: number) => {
      raf = requestAnimationFrame(tick)
      const p = phaseRef.current
      if (p === 'listening' && mic.capturing) {
        const lvl = mic.level()
        if (lvl > SPEECH_ON) {
          if (!hadSpeechRef.current && now - captureStartRef.current > 120)
            hadSpeechRef.current = true
          lastVoiceRef.current = now
        }
        const sayLen = now - captureStartRef.current
        const silentFor = now - lastVoiceRef.current
        if (
          (hadSpeechRef.current && silentFor > SILENCE_HANGOVER_MS && sayLen > MIN_SPEECH_MS) ||
          (hadSpeechRef.current && sayLen > MAX_UTTERANCE_MS)
        ) {
          void finishUtterance()
        }
      } else if (p === 'speaking') {
        const lvl = mic.level()
        if (now - speakStartRef.current > BARGE_GUARD_MS && lvl > BARGE_ON) {
          if (!bargeStartRef.current) bargeStartRef.current = now
          else if (now - bargeStartRef.current > BARGE_SUSTAIN_MS) {
            controllerRef.current?.abort()
            player.stop()
            beginListening() // cut in: the user is talking
          }
        } else {
          bargeStartRef.current = 0
        }
      }
    }

    ;(async () => {
      try {
        await mic.open()
        if (exitedRef.current) {
          mic.close()
          return
        }
        raf = requestAnimationFrame(tick)
        beginListening()
      } catch {
        setError('Microphone unavailable — check system permissions.')
        setPhaseBoth('error')
      }
    })()

    // expose controls to the JSX via the outer refs
    controlRef.current = {
      pause: () => {
        controllerRef.current?.abort()
        mic.cancelCapture()
        player.stop()
        busyRef.current = false
        setPhaseBoth('paused')
      },
      resume: () => {
        setError('')
        if (mic.analyser) beginListening()
        else
          void (async () => {
            try {
              await mic.open()
              beginListening()
            } catch {
              fail('Microphone unavailable — check system permissions.')
            }
          })()
      },
    }

    return () => {
      exitedRef.current = true
      cancelAnimationFrame(raf)
      controllerRef.current?.abort()
      mic.close()
      player.stop()
      controlRef.current = null
    }
  }, [sessionId, queryClient])

  const togglePause = () => {
    if (phase === 'paused' || phase === 'error') controlRef.current?.resume()
    else controlRef.current?.pause()
  }

  // Space toggles the mic, Escape leaves.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onExit()
      } else if (e.code === 'Space' && !(e.target as HTMLElement)?.closest('input, textarea')) {
        e.preventDefault()
        togglePause()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const paused = phase === 'paused' || phase === 'error'
  const vizAnalyser =
    phase === 'listening'
      ? micRef.current?.analyser ?? null
      : phase === 'speaking'
        ? playerRef.current?.analyser ?? null
        : null

  return (
    <div className="relative flex h-full flex-col items-center">
      <button
        type="button"
        aria-label="Exit voice mode"
        title="Exit voice mode (Esc)"
        onClick={onExit}
        className="absolute top-2 right-4 z-10 grid size-9 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      >
        <X size={18} />
      </button>

      {/* caption rail — the running conversation, newest pinned to the orb */}
      <div
        ref={captionsRef}
        className="flex w-full max-w-[560px] flex-1 flex-col justify-end gap-3 overflow-y-auto px-6 pt-14 pb-4"
        style={{
          maskImage: 'linear-gradient(to bottom, transparent, black 12%)',
          WebkitMaskImage: 'linear-gradient(to bottom, transparent, black 12%)',
        }}
      >
        {turns.slice(-8).map((turn) => (
          <p
            key={turn.id}
            className={
              turn.role === 'user'
                ? 'self-end rounded-2xl rounded-br-md bg-surface-2 px-3.5 py-2 text-right text-[13px] text-ink-2 select-text'
                : 'text-[15px] leading-relaxed text-ink select-text'
            }
          >
            {turn.text}
          </p>
        ))}
        {live ? (
          <p className="text-[15px] leading-relaxed text-ink-2 select-text">{live}</p>
        ) : null}
      </div>

      {/* the orb */}
      <div className="relative grid shrink-0 place-items-center">
        <VoiceVisualizer analyser={vizAnalyser} phase={phase} reducedMotion={reducedMotion} />
        {phase === 'thinking' ? (
          <motion.div
            aria-hidden
            className="absolute size-9 rounded-full border-2 border-primary border-t-transparent"
            animate={reducedMotion ? {} : { rotate: 360 }}
            transition={{ duration: 0.8, repeat: Infinity, ease: 'linear' }}
          />
        ) : null}
      </div>

      <AnimatePresence mode="wait">
        <motion.p
          key={phase === 'error' ? `err:${error}` : phase}
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={{ duration: 0.18 }}
          className={`shrink-0 pt-3 text-sm font-medium ${
            phase === 'error' ? 'text-danger' : 'text-ink-2'
          }`}
        >
          {phase === 'error' ? error : STATUS[phase]}
        </motion.p>
      </AnimatePresence>

      {/* controls */}
      <div className="flex shrink-0 flex-col items-center gap-2 pt-6 pb-12">
        <motion.button
          type="button"
          aria-label={paused ? 'Resume listening' : 'Pause'}
          title={paused ? 'Resume (Space)' : 'Pause (Space)'}
          onClick={togglePause}
          whileTap={{ scale: 0.94 }}
          className={`grid size-14 cursor-pointer place-items-center rounded-full shadow-sm transition-colors duration-150 ${
            paused
              ? 'bg-primary text-on-primary hover:bg-primary-strong'
              : 'bg-surface-2 text-ink hover:bg-surface-2/70'
          }`}
        >
          {paused ? <Mic size={22} /> : <Pause size={20} />}
        </motion.button>
        <p className="text-[11px] text-ink-3">
          {paused ? 'Tap to resume' : 'Hands-free · Space to pause · Esc to exit'}
        </p>
      </div>
    </div>
  )
}
