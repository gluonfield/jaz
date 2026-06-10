import { useEffect, useRef, useState } from 'react'
import { X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { EqBars } from '@/components/home/MusicBubbles'
import { RAINBOW_BEAM } from '@/components/session/Composer'
import { IconButton } from '@/components/ui/IconButton'
import type { PixelFieldShapeFrame } from '@/components/ui/PixelField.types'

const VIDEO_ID = 'rcd_SQZDlnk'
const CARD_WIDTH = 440

type VideoMeta = { title: string; author: string }

let cachedMeta: VideoMeta | null = null

// The rocket glyph is the launch pad: hovering asks the home field lifecycle to
// swell the construction, and clicking opens the launch track in a now-playing
// style card.
export function RocketVideo({
  frame,
  onHoverChange,
  onOpenChange,
}: {
  frame: PixelFieldShapeFrame | null
  onHoverChange?: (hovered: boolean) => void
  onOpenChange?: (open: boolean) => void
}) {
  const reducedMotion = useReducedMotion()
  const [open, setOpen] = useState(false)
  const [meta, setMeta] = useState<VideoMeta | null>(cachedMeta)
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

  // release the parent's hover/open flags if the field moves on mid-state
  useEffect(
    () => () => {
      onHoverChange?.(false)
      onOpenChange?.(false)
    },
    [onHoverChange, onOpenChange],
  )

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      setOpen(false)
      onOpenChange?.(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onOpenChange])

  useEffect(() => {
    if (!open || cachedMeta) return
    let stale = false
    ;(async () => {
      try {
        const url = new URL('https://www.youtube.com/oembed')
        url.searchParams.set('url', `https://www.youtube.com/watch?v=${VIDEO_ID}`)
        url.searchParams.set('format', 'json')
        const response = await fetch(url)
        if (!response.ok) throw new Error(`oEmbed failed with ${response.status}`)
        const body = (await response.json()) as { title?: string; author_name?: string }
        cachedMeta = { title: body.title ?? 'Launch soundtrack', author: body.author_name ?? 'YouTube' }
      } catch {
        cachedMeta = { title: 'Launch soundtrack', author: 'YouTube' }
      }
      if (!stale) setMeta(cachedMeta)
    })()
    return () => {
      stale = true
    }
  }, [open])

  if (!frame) return null

  const hit = Math.max(120, frame.scale * 1.4)
  const x =
    open && size
      ? Math.min(
          Math.max(frame.cx, CARD_WIDTH / 2 + 16),
          Math.max(size.w - CARD_WIDTH / 2 - 16, CARD_WIDTH / 2 + 16),
        )
      : frame.cx
  const y = open && size ? Math.min(Math.max(frame.cy, 170), Math.max(size.h - 170, 170)) : frame.cy

  return (
    <div ref={containerRef} className={`pointer-events-none absolute inset-0 ${open ? 'z-[3]' : 'z-[1]'}`}>
      <AnimatePresence>
        {open ? (
          <div
            key="player"
            className="absolute z-30"
            style={{ left: x, top: y, transform: 'translate(-50%, -50%)' }}
          >
            <motion.div
              className="pointer-events-auto relative"
              initial={reducedMotion ? { opacity: 0 } : { opacity: 0, scale: 0.85, filter: 'blur(8px)' }}
              animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
              exit={reducedMotion ? { opacity: 0 } : { opacity: 0, scale: 0.9, filter: 'blur(6px)' }}
              transition={{ type: 'spring', duration: 0.45, bounce: 0 }}
            >
              <motion.span
                aria-hidden
                className="pointer-events-none absolute -inset-[2px]"
                initial={{ opacity: 0 }}
                animate={{
                  opacity: 1,
                  ...(reducedMotion ? {} : { '--ring-angle': ['0deg', '360deg'] }),
                }}
                transition={{
                  opacity: { duration: 0.3, ease: 'easeOut', delay: 0.35 },
                  '--ring-angle': { duration: 3.2, ease: 'linear', repeat: Infinity },
                }}
              >
                <span
                  className="absolute -inset-[6px] rounded-[22px] opacity-60 blur-[12px]"
                  style={{ background: RAINBOW_BEAM }}
                />
                <span className="absolute -inset-[1px] rounded-[15px]" style={{ background: RAINBOW_BEAM }} />
              </motion.span>

              <div className="relative w-[440px] overflow-hidden rounded-[12px] bg-surface shadow-[0_18px_44px_rgba(0,0,0,0.2)] ring-1 ring-border/80">
                <div className="aspect-video w-full bg-black">
                  {/* chromeless: controls off and pointer-events none so hover
                      never summons YouTube's overlay — close with X or Esc */}
                  <iframe
                    className="pointer-events-none size-full"
                    src={`https://www.youtube.com/embed/${VIDEO_ID}?autoplay=1&controls=0&rel=0&fs=0&disablekb=1&iv_load_policy=3&playsinline=1`}
                    title={meta?.title ?? 'Launch soundtrack'}
                    allow="autoplay; encrypted-media"
                  />
                </div>
                <div className="flex items-center gap-2.5 p-2.5">
                  <div className="min-w-0 flex-1">
                    {meta ? (
                      <>
                        <p className="truncate text-[12px] leading-tight font-medium text-ink">
                          {meta.title}
                        </p>
                        <p className="mt-0.5 truncate text-[11px] leading-tight text-ink-3">{meta.author}</p>
                      </>
                    ) : (
                      <>
                        <div className="h-2.5 w-36 animate-pulse rounded-full bg-surface-2" />
                        <div className="mt-1.5 h-2 w-24 animate-pulse rounded-full bg-surface-2" />
                      </>
                    )}
                  </div>
                  <EqBars />
                  <IconButton
                    size="xs"
                    aria-label="Close launch video"
                    title="Close"
                    onClick={() => {
                      setOpen(false)
                      onOpenChange?.(false)
                    }}
                  >
                    <X size={13} />
                  </IconButton>
                </div>
              </div>
            </motion.div>
          </div>
        ) : (
          <motion.button
            key="pad"
            type="button"
            aria-label="Play the launch track"
            title="Play the launch track"
            className="pointer-events-auto absolute z-10 cursor-pointer rounded-full"
            style={{
              left: frame.cx,
              top: frame.cy,
              width: hit,
              height: hit,
              transform: 'translate(-50%, -50%)',
            }}
            initial={false}
            exit={{ opacity: 0 }}
            onMouseEnter={() => onHoverChange?.(true)}
            onMouseLeave={() => onHoverChange?.(false)}
            onFocus={() => onHoverChange?.(true)}
            onBlur={() => onHoverChange?.(false)}
            onClick={() => {
              onHoverChange?.(false)
              setOpen(true)
              onOpenChange?.(true)
            }}
          />
        )}
      </AnimatePresence>
    </div>
  )
}
