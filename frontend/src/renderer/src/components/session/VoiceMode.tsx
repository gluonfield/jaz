import { useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { streamSessionMessage } from '@/lib/api/stream'
import { speakStream, transcribeAudio } from '@/lib/api/voice'
import { StreamingPlayer } from '@/lib/audio/player'
import { Recorder } from '@/lib/audio/recorder'
import { keys } from '@/lib/query/keys'

type Phase = 'idle' | 'listening' | 'thinking' | 'speaking'

const STATUS: Record<Phase, string> = {
  idle: 'Tap to talk',
  listening: 'Listening — tap when you’re done',
  thinking: 'Thinking…',
  speaking: 'Tap to interrupt',
}

const RAINBOW_CONIC =
  'conic-gradient(from 0deg, var(--color-rainbow-1), var(--color-rainbow-2), var(--color-rainbow-3), var(--color-rainbow-4), var(--color-rainbow-5), var(--color-rainbow-1))'

const GLOW_OPACITY: Record<Phase, number> = {
  idle: 0.35,
  listening: 0.75,
  thinking: 0.55,
  speaking: 0.65,
}

const SPIN_SECONDS: Record<Phase, number> = { idle: 14, listening: 6, thinking: 2.4, speaking: 8 }

export function VoiceMode({ sessionId, onExit }: { sessionId: string; onExit: () => void }) {
  const queryClient = useQueryClient()
  const reducedMotion = useReducedMotion()
  const [phase, setPhase] = useState<Phase>('idle')
  const [userText, setUserText] = useState('')
  const [assistantText, setAssistantText] = useState('')
  const [error, setError] = useState('')

  const recorderRef = useRef<Recorder | null>(null)
  const playerRef = useRef<StreamingPlayer | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const exitedRef = useRef(false)
  if (!recorderRef.current) recorderRef.current = new Recorder()
  if (!playerRef.current) playerRef.current = new StreamingPlayer()

  useEffect(() => {
    // Refs survive StrictMode's simulated unmount; re-arm on (re)mount.
    exitedRef.current = false
    return () => {
      exitedRef.current = true
      recorderRef.current?.cancel()
      playerRef.current?.stop()
      abortRef.current?.abort()
    }
  }, [])

  const refreshTranscript = () => {
    queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
    queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
  }

  const runExchange = async () => {
    const recorder = recorderRef.current!
    const player = playerRef.current!
    const controller = new AbortController()
    abortRef.current = controller

    setPhase('thinking')
    try {
      const blob = await recorder.stop()
      const text = (await transcribeAudio(blob, controller.signal)).trim()
      if (exitedRef.current) return
      if (!text) {
        setError('Didn’t catch that — try again.')
        setPhase('idle')
        return
      }
      setUserText(text)

      let reply = ''
      await streamSessionMessage({
        sessionId,
        message: text,
        voice: true,
        signal: controller.signal,
        onEvent: (event) => {
          if (event.type === 'delta' && event.delta) {
            reply += event.delta
            setAssistantText(reply)
          }
          if (event.type === 'error' && event.error) throw new Error(event.error)
        },
      })
      if (exitedRef.current) return
      refreshTranscript()
      if (!reply.trim()) {
        setPhase('idle')
        return
      }

      setPhase('speaking')
      const res = await speakStream(reply, controller.signal)
      if (exitedRef.current) {
        res.body?.cancel()
        return
      }
      await player.play(res.body!, () => {
        if (!exitedRef.current) setPhase('idle')
      })
    } catch (err) {
      if (exitedRef.current || controller.signal.aborted) return
      setError(err instanceof Error ? err.message : String(err))
      setPhase('idle')
    }
  }

  const handleTap = async () => {
    const recorder = recorderRef.current!
    const player = playerRef.current!
    switch (phase) {
      case 'idle':
        setError('')
        setUserText('')
        setAssistantText('')
        try {
          await recorder.start()
          setPhase('listening')
        } catch {
          setError('Microphone unavailable — check system permissions.')
        }
        break
      case 'listening':
        void runExchange()
        break
      case 'speaking':
        player.stop()
        setPhase('idle')
        break
      case 'thinking':
        break
    }
  }

  return (
    <div className="relative flex h-full flex-col items-center justify-center px-10 pb-16">
      <button
        type="button"
        aria-label="Exit voice mode"
        title="Exit voice mode"
        onClick={onExit}
        className="absolute top-1 right-4 grid size-9 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      >
        <X size={18} />
      </button>

      <motion.button
        type="button"
        aria-label={STATUS[phase]}
        onClick={handleTap}
        whileTap={{ scale: 0.96 }}
        animate={
          reducedMotion
            ? {}
            : phase === 'listening'
              ? { scale: [1, 1.07, 1] }
              : phase === 'speaking'
                ? { scale: [1, 1.04, 1] }
                : { scale: [1, 1.02, 1] }
        }
        transition={{
          duration: phase === 'listening' ? 1.1 : phase === 'speaking' ? 0.8 : 4,
          repeat: Infinity,
          ease: 'easeInOut',
        }}
        className="relative size-44 cursor-pointer rounded-full"
      >
        <motion.div
          aria-hidden
          className="absolute -inset-4 rounded-full blur-2xl"
          style={{ background: RAINBOW_CONIC }}
          animate={
            reducedMotion
              ? { opacity: GLOW_OPACITY[phase] }
              : { opacity: GLOW_OPACITY[phase], rotate: 360 }
          }
          transition={{
            opacity: { duration: 0.4 },
            rotate: { duration: SPIN_SECONDS[phase], repeat: Infinity, ease: 'linear' },
          }}
        />
        <motion.div
          aria-hidden
          className="absolute inset-0 rounded-full"
          style={{ background: RAINBOW_CONIC }}
          animate={reducedMotion ? {} : { rotate: 360 }}
          transition={{ duration: SPIN_SECONDS[phase], repeat: Infinity, ease: 'linear' }}
        />
        <div aria-hidden className="absolute inset-[4px] rounded-full bg-bg" />
        <AnimatePresence>
          {phase === 'listening' && !reducedMotion ? (
            <motion.div
              aria-hidden
              className="absolute inset-0 rounded-full border-2 border-primary"
              initial={{ scale: 1, opacity: 0.6 }}
              animate={{ scale: 1.45, opacity: 0 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 1.4, repeat: Infinity, ease: 'easeOut' }}
            />
          ) : null}
        </AnimatePresence>
      </motion.button>

      <motion.p
        key={phase}
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        className="pt-8 text-sm font-medium text-ink-2"
      >
        {STATUS[phase]}
      </motion.p>

      <div className="flex min-h-32 w-full max-w-[480px] flex-col items-center gap-3 pt-6 text-center">
        {userText ? <p className="text-[13px] text-ink-3">“{userText}”</p> : null}
        {assistantText ? (
          <p className="text-[15px] leading-relaxed text-ink select-text">{assistantText}</p>
        ) : null}
        {error ? <p className="text-[13px] text-danger">{error}</p> : null}
      </div>
    </div>
  )
}
