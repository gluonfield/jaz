import { useCallback, useEffect, useRef, useState } from 'react'
import {
  type MusicPreviewCategory,
  type PreviewTrack,
  pickRandomPreviewTrack,
} from '@/lib/music/itunesPreview'

type PreviewPlayerStatus = 'idle' | 'loading' | 'playing' | 'error'

export type PreviewPlayerState = {
  status: PreviewPlayerStatus
  activeCategoryId: string | null
  track: PreviewTrack | null
  progress: number
  duration: number
  error: string | null
}

const IDLE_STATE: PreviewPlayerState = {
  status: 'idle',
  activeCategoryId: null,
  track: null,
  progress: 0,
  duration: 30,
  error: null,
}

function audioDuration(audio: HTMLAudioElement): number {
  return Number.isFinite(audio.duration) && audio.duration > 0 ? audio.duration : 30
}

export function usePreviewPlayer() {
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const cleanupRef = useRef<() => void>(() => {})
  const requestIdRef = useRef(0)
  const playRef = useRef<(category: MusicPreviewCategory, excludeTrackId?: string) => Promise<void>>(
    async () => {},
  )
  const [state, setState] = useState<PreviewPlayerState>(IDLE_STATE)

  const cleanupAudio = useCallback(() => {
    cleanupRef.current()
    cleanupRef.current = () => {}
    if (!audioRef.current) return
    audioRef.current.pause()
    audioRef.current.src = ''
    audioRef.current = null
  }, [])

  const stop = useCallback(() => {
    requestIdRef.current += 1
    cleanupAudio()
    setState(IDLE_STATE)
  }, [cleanupAudio])

  const play = useCallback(
    async (category: MusicPreviewCategory, excludeTrackId?: string) => {
      const requestId = requestIdRef.current + 1
      requestIdRef.current = requestId
      cleanupAudio()
      setState({
        ...IDLE_STATE,
        status: 'loading',
        activeCategoryId: category.id,
      })

      try {
        const track = await pickRandomPreviewTrack(category, excludeTrackId)
        if (requestIdRef.current !== requestId) return

        const audio = new Audio(track.previewUrl)
        audio.preload = 'metadata'
        audioRef.current = audio

        const updateProgress = () => {
          if (requestIdRef.current !== requestId) return
          const duration = audioDuration(audio)
          setState((current) =>
            current.activeCategoryId === category.id
              ? {
                  ...current,
                  duration,
                  progress: Math.min(1, audio.currentTime / duration),
                }
              : current,
          )
        }
        const handleEnded = () => {
          if (requestIdRef.current !== requestId) return
          // genre radio: roll straight into another preview from this category
          void playRef.current(category, track.id)
        }
        const handleError = () => {
          if (requestIdRef.current !== requestId) return
          cleanupAudio()
          setState({
            ...IDLE_STATE,
            status: 'error',
            activeCategoryId: category.id,
            error: "Couldn't play this preview",
          })
        }

        audio.addEventListener('timeupdate', updateProgress)
        audio.addEventListener('durationchange', updateProgress)
        audio.addEventListener('loadedmetadata', updateProgress)
        audio.addEventListener('ended', handleEnded)
        audio.addEventListener('error', handleError)
        cleanupRef.current = () => {
          audio.removeEventListener('timeupdate', updateProgress)
          audio.removeEventListener('durationchange', updateProgress)
          audio.removeEventListener('loadedmetadata', updateProgress)
          audio.removeEventListener('ended', handleEnded)
          audio.removeEventListener('error', handleError)
        }

        setState({
          status: 'playing',
          activeCategoryId: category.id,
          track,
          progress: 0,
          duration: 30,
          error: null,
        })
        await audio.play()
        updateProgress()
      } catch (error) {
        if (requestIdRef.current !== requestId) return
        cleanupAudio()
        setState({
          ...IDLE_STATE,
          status: 'error',
          activeCategoryId: category.id,
          error: (error as Error).message,
        })
      }
    },
    [cleanupAudio],
  )

  useEffect(() => {
    playRef.current = play
  }, [play])

  useEffect(
    () => () => {
      requestIdRef.current += 1
      cleanupAudio()
    },
    [cleanupAudio],
  )

  return { state, play, stop }
}
