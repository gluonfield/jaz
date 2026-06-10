import { type CSSProperties, useEffect, useMemo, useRef, useState } from 'react'
import { AlertCircle, Play, Square } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { MUSIC_BUBBLE_CATEGORIES } from '@/components/home/musicBubbleConfig'
import { RAINBOW_BEAM } from '@/components/session/Composer'
import { IconButton } from '@/components/ui/IconButton'
import type { PixelFieldShapeFrame } from '@/components/ui/PixelField'
import { type PreviewPlayerState, usePreviewPlayer } from '@/lib/music/usePreviewPlayer'

const placements = [
  { angle: -138 },
  { angle: -103 },
  { angle: -68 },
  { angle: -31 },
  { angle: 5 },
  { angle: 41 },
  { angle: 78 },
  { angle: 116 },
  { angle: 154 },
  { angle: 194 },
]

// Desynced so the orbit never reads as a single choreographed wave.
const FLOAT_SECONDS = [6.4, 5.3, 7.2, 5.7, 6.8, 5.1, 7.5, 5.9, 6.1, 6.9]

const EQ_BARS = [
  { height: 6, duration: 0.92, delay: -0.36 },
  { height: 13, duration: 1.18, delay: -0.82 },
  { height: 9, duration: 0.74, delay: -0.18 },
  { height: 11, duration: 1.04, delay: -0.55 },
]

export function EqBars() {
  return (
    <span aria-hidden className="flex h-[14px] items-end gap-[2.5px]">
      {EQ_BARS.map((bar, i) => (
        <span
          key={i}
          className="music-eq-bar w-[3px] rounded-full bg-primary"
          style={
            {
              height: bar.height,
              '--eq-duration': `${bar.duration}s`,
              '--eq-delay': `${bar.delay}s`,
              '--eq-hue-delay': `${i * -1.15}s`,
            } as CSSProperties
          }
        />
      ))}
    </span>
  )
}

// Orbit positions clamped to the container pile up when the glyph sits near
// an edge; a few relaxation passes spread vertical neighbors back apart.
function orbitPositions(
  frame: PixelFieldShapeFrame,
  width: number,
  height: number,
): Array<{ x: number; y: number; angleRad: number }> {
  const radius = Math.max(92, frame.scale * 0.68)
  const clampX = (v: number) => Math.min(Math.max(v, 80), Math.max(width - 80, 80))
  const clampY = (v: number) => Math.min(Math.max(v, 44), Math.max(height - 44, 44))
  const points = MUSIC_BUBBLE_CATEGORIES.map((_, index) => {
    const angleRad = (placements[index % placements.length].angle / 180) * Math.PI
    return {
      x: clampX(frame.cx + Math.cos(angleRad) * radius * 1.24),
      y: clampY(frame.cy + Math.sin(angleRad) * radius * 0.96),
      angleRad,
    }
  })
  for (let pass = 0; pass < 8; pass++) {
    for (let i = 0; i < points.length; i++) {
      for (let j = i + 1; j < points.length; j++) {
        const dx = points[j].x - points[i].x
        const dy = points[j].y - points[i].y
        if (Math.abs(dx) >= 118 || Math.abs(dy) >= 44) continue
        const push = (44 - Math.abs(dy)) / 2
        const dir = dy === 0 ? 1 : Math.sign(dy)
        points[i].y = clampY(points[i].y - dir * push)
        points[j].y = clampY(points[j].y + dir * push)
      }
    }
  }
  return points
}

function remainingLabel(state: PreviewPlayerState): string {
  const remaining = Math.max(0, Math.ceil(state.duration * (1 - state.progress)))
  return `${Math.floor(remaining / 60)}:${String(remaining % 60).padStart(2, '0')}`
}

function NowPlayingContent({
  state,
  playing,
  stopLabel,
  onStop,
}: {
  state: PreviewPlayerState
  playing: boolean
  stopLabel: string
  onStop: () => void
}) {
  const track = state.track
  return (
    <motion.div
      layout
      className="flex w-full min-w-0 flex-col gap-2"
      initial={{ opacity: 0, filter: 'blur(4px)' }}
      animate={{ opacity: 1, filter: 'blur(0px)' }}
      exit={{ opacity: 0, filter: 'blur(4px)' }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
    >
      <motion.div
        key={track?.id ?? 'pending'}
        className="flex min-w-0 items-center gap-2.5"
        initial={{ opacity: 0, filter: 'blur(3px)' }}
        animate={{ opacity: 1, filter: 'blur(0px)' }}
        transition={{ duration: 0.2, ease: 'easeOut' }}
      >
        {track ? (
          track.artworkUrl ? (
            <img
              src={track.artworkUrl.replace('100x100', '200x200')}
              alt=""
              className="size-10 shrink-0 rounded-[8px] object-cover outline outline-1 -outline-offset-1 outline-black/10 dark:outline-white/10"
            />
          ) : (
            <div className="size-10 shrink-0 rounded-[8px] bg-primary-soft" />
          )
        ) : (
          <div className="size-10 shrink-0 animate-pulse rounded-[8px] bg-surface-2" />
        )}
        <div className="min-w-0 flex-1">
          {track ? (
            // the track itself links out: click title or artist to search YouTube
            <a
              href={`https://www.youtube.com/results?search_query=${encodeURIComponent(
                `${track.artistName} ${track.trackName}`,
              )}`}
              target="_blank"
              rel="noreferrer"
              aria-label={`Search YouTube for ${track.artistName} ${track.trackName}`}
              title="Find on YouTube"
              className="group/link block min-w-0 cursor-pointer"
            >
              <p className="truncate text-[12px] leading-tight font-medium text-ink decoration-ink/40 underline-offset-2 group-hover/link:underline">
                {track.trackName}
              </p>
              <p className="mt-0.5 truncate text-[11px] leading-tight text-ink-3">{track.artistName}</p>
            </a>
          ) : (
            <>
              <div className="h-2.5 w-28 animate-pulse rounded-full bg-surface-2" />
              <div className="mt-1.5 h-2 w-20 animate-pulse rounded-full bg-surface-2" />
            </>
          )}
        </div>
        {/* live bars plus the explicit stop, mirroring the video card */}
        <span className="flex shrink-0 items-center gap-1">
          <EqBars />
          <IconButton size="xs" aria-label={stopLabel} title="Stop" onClick={onStop}>
            <Square size={10} fill="currentColor" strokeWidth={0} />
          </IconButton>
        </span>
      </motion.div>
      <div className="flex items-center gap-2">
        <div className="h-1 min-w-0 flex-1 overflow-hidden rounded-full bg-ink/10">
          <div
            className="h-full origin-left rounded-full bg-primary transition-transform duration-200 ease-linear"
            style={{ transform: `scaleX(${playing ? state.progress : 0})` }}
          />
        </div>
        <span className="w-7 shrink-0 text-right text-[10px] tabular-nums text-ink-3">
          {playing ? remainingLabel(state) : '0:30'}
        </span>
      </div>
    </motion.div>
  )
}

function ErrorContent({
  message,
  retryLabel,
  onRetry,
}: {
  message: string | null
  retryLabel: string
  onRetry: () => void
}) {
  return (
    <motion.div
      layout
      className="flex w-full min-w-0 items-center gap-2"
      initial={{ opacity: 0, filter: 'blur(4px)' }}
      animate={{ opacity: 1, filter: 'blur(0px)' }}
      exit={{ opacity: 0, filter: 'blur(4px)' }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
    >
      <AlertCircle size={14} className="shrink-0 text-danger" />
      <span className="min-w-0 flex-1 truncate text-left text-[12px] leading-snug text-danger">
        {message ?? "Couldn't find a preview"}
      </span>
      <button
        type="button"
        aria-label={retryLabel}
        onClick={onRetry}
        className="shrink-0 cursor-pointer rounded-full bg-ink/10 px-2 py-0.5 text-[10px] font-medium text-ink-2 transition-colors duration-150 hover:bg-ink/15 hover:text-ink"
      >
        retry
      </button>
    </motion.div>
  )
}

export function MusicBubbles({
  frame,
  onPlaybackActiveChange,
}: {
  frame: PixelFieldShapeFrame | null
  onPlaybackActiveChange?: (active: boolean) => void
}) {
  const reducedMotion = useReducedMotion()
  const { state, play, stop } = usePreviewPlayer()
  const playbackActive = state.status === 'loading' || state.status === 'playing'
  const containerRef = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState<{ w: number; h: number } | null>(null)

  const mounted = frame !== null
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const update = () => setSize({ w: el.clientWidth, h: el.clientHeight })
    update()
    const observer = new ResizeObserver(update)
    observer.observe(el)
    return () => observer.disconnect()
  }, [mounted])

  const positions = useMemo(
    () => (frame && size ? orbitPositions(frame, size.w, size.h) : null),
    [frame, size],
  )

  useEffect(() => {
    onPlaybackActiveChange?.(playbackActive)
  }, [onPlaybackActiveChange, playbackActive])

  useEffect(
    () => () => {
      onPlaybackActiveChange?.(false)
    },
    [onPlaybackActiveChange],
  )

  useEffect(() => {
    if (state.status !== 'error') return
    const timer = window.setTimeout(() => stop(), 5000)
    return () => window.clearTimeout(timer)
  }, [state.status, stop])

  if (!frame) return null

  return (
    <motion.div
      ref={containerRef}
      aria-label="Music previews"
      className="pointer-events-none absolute inset-0 z-[1]"
      initial={reducedMotion ? false : { opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.16, ease: 'easeOut' }}
    >
      {positions
        ? MUSIC_BUBBLE_CATEGORIES.map((category, index) => {
            const { x, y, angleRad } = positions[index]
            const active = state.activeCategoryId === category.id
            const busy = active && state.status === 'loading'
            const playing = active && state.status === 'playing'
            const errored = active && state.status === 'error'
            const expanded = active && state.status !== 'idle'
            const receded = playbackActive && !active
            // the card is wider than a pill: pull it further in from the edges
            const px = expanded && size ? Math.min(Math.max(x, 136), Math.max(size.w - 136, 136)) : x
            const py = expanded && size ? Math.min(Math.max(y, 72), Math.max(size.h - 72, 72)) : y

            return (
              <div
                key={category.id}
                className={`absolute ${expanded ? 'z-30' : 'z-10'}`}
                style={{
                  left: `${px}px`,
                  top: `${py}px`,
                  transform: 'translate(-50%, -50%)',
                }}
              >
                <motion.div
                  initial={reducedMotion ? false : 'hidden'}
                  animate={receded ? 'receded' : 'shown'}
                  whileHover={{ opacity: 1, scale: 1 }}
                  variants={{
                    hidden: {
                      opacity: 0,
                      x: -Math.cos(angleRad) * 26,
                      y: -Math.sin(angleRad) * 26,
                      scale: 0.7,
                      filter: 'blur(8px)',
                    },
                    shown: {
                      opacity: 1,
                      x: 0,
                      y: 0,
                      scale: 1,
                      filter: 'blur(0px)',
                      transition: reducedMotion
                        ? { duration: 0 }
                        : {
                            delay: index * 0.045,
                            type: 'spring',
                            duration: 0.5,
                            bounce: 0,
                          },
                    },
                    receded: {
                      opacity: 0.65,
                      x: 0,
                      y: 0,
                      scale: 0.96,
                      filter: 'blur(0px)',
                      transition: reducedMotion
                        ? { duration: 0 }
                        : { type: 'spring', duration: 0.45, bounce: 0 },
                    },
                  }}
                >
                  <div
                    className="music-float relative"
                    style={
                      {
                        '--float-duration': `${FLOAT_SECONDS[index % FLOAT_SECONDS.length]}s`,
                        '--float-delay': `${index * -0.83}s`,
                        animationPlayState: expanded ? 'paused' : 'running',
                      } as CSSProperties
                    }
                  >
                    {/* comet ring while music is alive — same beam as the composer focus ring */}
                    <AnimatePresence>
                      {busy || playing ? (
                        <motion.span
                          key="ring"
                          aria-hidden
                          className="pointer-events-none absolute -inset-[2px]"
                          initial={{ opacity: 0 }}
                          animate={{
                            opacity: 1,
                            ...(reducedMotion ? {} : { '--ring-angle': ['0deg', '360deg'] }),
                          }}
                          exit={{ opacity: 0 }}
                          transition={{
                            opacity: {
                              duration: 0.3,
                              ease: 'easeOut',
                              delay: 0.3,
                            },
                            '--ring-angle': {
                              duration: 3.2,
                              ease: 'linear',
                              repeat: Infinity,
                            },
                          }}
                        >
                          <span
                            className="absolute -inset-[6px] rounded-[22px] opacity-60 blur-[12px]"
                            style={{ background: RAINBOW_BEAM }}
                          />
                          <span
                            className="absolute -inset-[1px] rounded-[15px]"
                            style={{ background: RAINBOW_BEAM }}
                          />
                        </motion.span>
                      ) : null}
                    </AnimatePresence>

                    <motion.div
                      layout
                      whileTap={expanded ? undefined : { scale: 0.97 }}
                      whileHover={reducedMotion || expanded ? undefined : { y: -2 }}
                      style={{ borderRadius: expanded ? 12 : 999 }}
                      transition={{
                        layout: reducedMotion
                          ? { duration: 0 }
                          : { type: 'spring', duration: 0.5, bounce: 0 },
                      }}
                      className={`pointer-events-auto relative flex items-center overflow-hidden ring-1 transition-[box-shadow,background-color] duration-300 select-none ${
                        expanded
                          ? `w-[252px] bg-surface p-2.5 text-left shadow-[0_18px_44px_rgba(0,0,0,0.2)] ${
                              errored ? 'ring-danger/30' : 'ring-border/80'
                            }`
                          : 'bg-surface/85 text-[12px] font-medium text-ink-2 shadow-[0_10px_24px_rgba(0,0,0,0.1)] ring-border/70 backdrop-blur-[2px] hover:bg-surface hover:text-ink hover:shadow-[0_14px_30px_rgba(0,0,0,0.14)]'
                      }`}
                    >
                      <AnimatePresence mode="popLayout" initial={false}>
                        {expanded ? (
                          errored ? (
                            <ErrorContent
                              key="error"
                              message={state.error}
                              retryLabel={`Retry ${category.label} preview`}
                              onRetry={() => void play(category)}
                            />
                          ) : (
                            <NowPlayingContent
                              key="card"
                              state={state}
                              playing={playing}
                              stopLabel={`Stop ${category.label} preview`}
                              onStop={stop}
                            />
                          )
                        ) : (
                          <motion.button
                            key="pill"
                            type="button"
                            layout
                            aria-label={`Play ${category.label} preview`}
                            title={`Play ${category.label}`}
                            onClick={() => void play(category)}
                            className="flex h-8 cursor-pointer items-center gap-1.5 px-3.5"
                            initial={{ opacity: 0 }}
                            animate={{ opacity: 1 }}
                            exit={{ opacity: 0 }}
                            transition={{ duration: 0.12 }}
                          >
                            <Play
                              size={11}
                              fill="currentColor"
                              strokeWidth={0}
                              className="shrink-0 text-primary"
                            />
                            <span className="whitespace-nowrap">{category.label}</span>
                          </motion.button>
                        )}
                      </AnimatePresence>
                    </motion.div>
                  </div>
                </motion.div>
              </div>
            )
          })
        : null}
    </motion.div>
  )
}
