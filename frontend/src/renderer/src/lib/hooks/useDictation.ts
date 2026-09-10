import { useCallback, useEffect, useRef, useState } from 'react'
import type { DictationAvailability, DictationPhase } from '@shared/dictation'

export function useDictation({
  identity,
  disabled,
  onComplete,
}: {
  identity: string
  disabled: boolean
  onComplete: (text: string, send: boolean) => void
}) {
  const api = typeof window === 'undefined' ? undefined : window.jaz?.dictation
  const [availability, setAvailability] = useState<DictationAvailability>({ available: false, reason: '' })
  const [phase, setPhase] = useState<DictationPhase | null>(null)
  const [transcript, setTranscript] = useState('')
  const [levels, setLevels] = useState<number[]>([])
  const [error, setError] = useState('')
  const session = useRef<{ id: string; identity: string; send: boolean } | null>(null)
  const current = useRef({ identity, disabled, onComplete })
  current.current = { identity, disabled, onComplete }

  const cancel = useCallback(() => {
    const active = session.current
    session.current = null
    setPhase(null)
    setTranscript('')
    if (active) {
      void api?.cancel(active.id).catch(() => {})
    }
  }, [api])

  useEffect(() => {
    if (!api) {
      return
    }
    let mounted = true
    void api.availability().then((value) => {
      if (mounted) {
        setAvailability(value)
      }
    }).catch(() => {
      if (mounted) {
        setAvailability({ available: false, reason: 'Native dictation is unavailable.' })
      }
    })
    const unsubscribe = api.onEvent((id, event) => {
      const active = session.current
      if (!active || active.id !== id || active.identity !== current.current.identity || current.current.disabled) {
        return
      }
      switch (event.type) {
        case 'status':
          setPhase(event.phase)
          break
        case 'level':
          setLevels((previous) => [...previous.slice(-71), event.level])
          break
        case 'result':
          setTranscript(event.text)
          break
        case 'complete':
          session.current = null
          setPhase(null)
          setTranscript('')
          if (event.text.trim()) {
            current.current.onComplete(event.text, active.send)
          } else {
            setError('No speech was detected. Try again.')
          }
          break
        case 'error':
          session.current = null
          setPhase(null)
          setTranscript('')
          setError(event.message)
          break
      }
    })
    return () => {
      mounted = false
      unsubscribe()
      const active = session.current
      session.current = null
      if (active) {
        void api.cancel(active.id).catch(() => {})
      }
    }
  }, [api])

  useEffect(() => {
    cancel()
    setError('')
  }, [identity, disabled, cancel])

  const start = async () => {
    if (!api || !availability.available || disabled || session.current) {
      return
    }
    const active = { id: crypto.randomUUID(), identity, send: false }
    session.current = active
    setPhase('starting')
    setTranscript('')
    setLevels([])
    setError('')
    try {
      await api.start(active.id)
    } catch (error) {
      if (session.current !== active) {
        return
      }
      session.current = null
      setPhase(null)
      setError(error instanceof Error ? error.message : 'Dictation could not start.')
    }
  }

  const stop = async (send = false) => {
    const active = session.current
    if (!active || !api || phase !== 'recording') {
      return
    }
    active.send = send
    setPhase('transcribing')
    try {
      await api.stop(active.id)
    } catch (error) {
      if (session.current !== active) {
        return
      }
      cancel()
      setError(error instanceof Error ? error.message : 'Dictation could not finish.')
    }
  }

  return { showButton: Boolean(api), availability, phase, transcript, levels, error, start, stop, cancel, dismissError: () => setError('') }
}
