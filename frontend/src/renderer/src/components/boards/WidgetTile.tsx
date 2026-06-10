import { Link } from '@tanstack/react-router'
import { ExternalLink, Play, X } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import {
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useRef,
  useState,
} from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { reportWidgetError, reportWidgetLayout, widgetContentUrl } from '@/lib/api/boards'
import { runLoopNow } from '@/lib/api/loops'
import type { BoardItem } from '@/lib/api/types'
import { hasTime, relativeTime } from '@/lib/format/time'

function TileStatusDot({ item }: { item: BoardItem }) {
  if (item.loop_last_run_status === 'running' || item.loop_last_run_status === 'starting') {
    return <span title="Running" className="size-1.5 shrink-0 animate-pulse rounded-full bg-running" />
  }
  if (item.loop_last_run_status === 'error') {
    return <span title="Last run failed" className="size-1.5 shrink-0 rounded-full bg-danger" />
  }
  if (item.last_error) {
    return (
      <span
        title={`Widget error: ${item.last_error}`}
        className="size-1.5 shrink-0 rounded-full bg-danger"
      />
    )
  }
  if (item.loop_status === 'paused') {
    return <span title="Paused" className="size-1.5 shrink-0 rounded-full bg-ink-3/50" />
  }
  return <span title="Active" className="size-1.5 shrink-0 rounded-full bg-primary" />
}

interface Layer {
  src: string
  ready: boolean
  // Created by a version bump (fresh data), as opposed to a theme/zoom reload.
  fresh: boolean
}

const WIPE_DURATION = 0.65
const WIPE_EASE = [0.65, 0, 0.35, 1] as const

// Ghost preview of the widget to come: skeleton blocks breathing while a wave
// rolls through the bars. The dot goes rainbow while the first run is live.
function FirstRunPlaceholder({ running }: { running: boolean }) {
  const reduce = useReducedMotion()
  const bars = [42, 68, 52, 84, 62]
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 overflow-hidden px-4">
      <div aria-hidden className="w-full max-w-[180px] space-y-2">
        <div className="flex items-end gap-3">
          <div className="h-7 w-12 animate-pulse rounded-md bg-surface-2" />
          <div className="h-3 w-16 animate-pulse rounded-full bg-surface-2 [animation-delay:200ms]" />
        </div>
        <div className="flex h-10 items-end gap-1.5">
          {bars.map((height, index) => (
            <motion.div
              key={index}
              className="flex-1 rounded-t-[3px] bg-primary/25"
              style={{ height: `${height * 0.45}%` }}
              animate={
                reduce
                  ? undefined
                  : { height: [`${height * 0.45}%`, `${height}%`, `${height * 0.45}%`] }
              }
              transition={{
                duration: 2.4,
                repeat: Number.POSITIVE_INFINITY,
                ease: 'easeInOut',
                delay: index * 0.18,
              }}
            />
          ))}
        </div>
        <div className="h-2.5 w-full animate-pulse rounded-full bg-surface-2 [animation-delay:400ms]" />
        <div className="h-2.5 w-3/4 animate-pulse rounded-full bg-surface-2 [animation-delay:600ms]" />
      </div>
      <p className="flex items-center gap-1.5 text-[11px] text-ink-3">
        <span
          className={`size-1.5 shrink-0 rounded-full bg-primary ${
            running ? 'jaz-shimmer' : 'animate-pulse'
          }`}
        />
        {running ? 'First run in progress…' : 'Waiting for the first run'}
      </p>
    </div>
  )
}

export function WidgetTile({
  item,
  theme,
  scale,
  dragging,
  onHeaderPointerDown,
  onResizePointerDown,
  onRemove,
}: {
  item: BoardItem
  theme: 'light' | 'dark'
  scale: number
  dragging?: boolean
  onHeaderPointerDown: (e: ReactPointerEvent) => void
  onResizePointerDown: (e: ReactPointerEvent) => void
  onRemove: () => void
}) {
  const reduce = useReducedMotion()
  const src = widgetContentUrl(item.widget_id, item.current_version, theme, scale)

  // Double-buffered content: the old document stays visible until the new one
  // has loaded. Fresh versions are then wiped in by the rainbow scanline;
  // theme/zoom reloads quietly crossfade — no blank flash either way.
  const [layers, setLayers] = useState<Layer[]>(() =>
    item.current_version > 0 ? [{ src, ready: true, fresh: false }] : [],
  )
  const framesRef = useRef(new Map<string, HTMLIFrameElement>())
  // One report per loaded version: a render-loop error would otherwise spam
  // the backend (and the loop's next-run prompt) on every poll.
  const reportedRef = useRef('')
  const layoutReportedRef = useRef('')
  // A fresh publish (version bump) gets the rainbow sweep once the new
  // content is actually on screen.
  const lastVersionRef = useRef(item.current_version)
  const pulsePendingRef = useRef(false)
  const [pulse, setPulse] = useState(0)

  useEffect(() => {
    if (item.current_version > lastVersionRef.current) pulsePendingRef.current = true
    lastVersionRef.current = item.current_version
  }, [item.current_version])

  useEffect(() => {
    if (item.current_version <= 0) {
      setLayers([])
      return
    }
    setLayers((current) => {
      const top = current[current.length - 1]
      if (top && top.src === src) return current
      // A still-loading layer is superseded by the newest target.
      return [
        ...current.filter((layer) => layer.ready),
        { src, ready: false, fresh: pulsePendingRef.current },
      ]
    })
  }, [src, item.current_version])

  const onLayerLoad = (loaded: string) => {
    setLayers((current) =>
      current.map((layer) => (layer.src === loaded ? { ...layer, ready: true } : layer)),
    )
    if (pulsePendingRef.current) {
      pulsePendingRef.current = false
      setPulse((p) => p + 1)
    }
    // Drop covered layers once the crossfade/wipe is over.
    window.setTimeout(() => {
      setLayers((current) => (current.length > 1 ? current.slice(-1) : current))
    }, 800)
  }

  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const frames = [...framesRef.current.values()]
      if (!frames.some((frame) => frame.contentWindow === event.source)) return
      const msg = event.data as {
        type?: string
        href?: string
        message?: string
        dead_space_pct?: number
        overflow_px?: number
        clipped?: number
      }
      if (msg?.type === 'jaz:link' && typeof msg.href === 'string') {
        window.open(msg.href, '_blank')
      }
      if (msg?.type === 'jaz:error' && typeof msg.message === 'string') {
        const key = `${item.widget_id}@${item.current_version}`
        if (reportedRef.current === key) return
        reportedRef.current = key
        void reportWidgetError(item.widget_id, msg.message).catch(() => {})
      }
      if (msg?.type === 'jaz:layout' && typeof msg.dead_space_pct === 'number') {
        const layout = {
          dead_space_pct: msg.dead_space_pct,
          overflow_px: msg.overflow_px ?? 0,
          clipped: msg.clipped ?? 0,
        }
        // Dedupe on version + measurement, not just version: a resize that
        // changes the layout must replace the stale report, while identical
        // re-measures (polls, crossfade layers) stay silent.
        const key = `${item.widget_id}@${item.current_version}:${JSON.stringify(layout)}`
        if (layoutReportedRef.current === key) return
        layoutReportedRef.current = key
        void reportWidgetLayout(item.widget_id, layout).catch(() => {})
      }
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [item.widget_id, item.current_version])

  useEffect(() => {
    for (const frame of framesRef.current.values()) {
      frame.contentWindow?.postMessage({ type: 'jaz:theme', theme }, '*')
    }
  }, [theme])

  const paused = item.loop_status === 'paused'
  const updated = hasTime(item.widget_updated_at) ? relativeTime(item.widget_updated_at) : ''

  return (
    <div
      className={`group relative flex h-full w-full flex-col overflow-hidden rounded-card bg-surface ring-1 transition-shadow duration-150 ${
        dragging ? 'shadow-lg ring-primary/50' : 'ring-border'
      } ${paused ? 'opacity-70' : ''}`}
    >
      <div
        onPointerDown={onHeaderPointerDown}
        className="flex h-8 shrink-0 cursor-grab select-none items-center gap-1.5 px-2.5 active:cursor-grabbing"
      >
        <TileStatusDot item={item} />
        <span className="min-w-0 flex-1 truncate text-[12px] font-medium text-ink" title={item.loop_name}>
          {item.title}
        </span>
        <span className="shrink-0 text-[11px] tabular-nums text-ink-3 group-hover:hidden">
          {updated}
        </span>
        <div className="hidden shrink-0 items-center group-hover:flex">
          <IconButton
            variant="ghost"
            size="xs"
            aria-label="Run loop now"
            title="Run loop now"
            onClick={() => void runLoopNow(item.loop_id).catch(() => {})}
          >
            <Play size={12} />
          </IconButton>
          {window.jaz?.windowKind === 'board' ? (
            // Board windows never navigate themselves; the loop opens in the
            // main app window.
            <IconButton
              variant="ghost"
              size="xs"
              aria-label="Open loop in Jaz"
              title="Open loop in Jaz"
              onClick={() => window.jaz.openInMain(`/loops/${item.loop_id}`)}
            >
              <ExternalLink size={12} />
            </IconButton>
          ) : (
            <Link
              to="/loops/$loopId"
              params={{ loopId: item.loop_id }}
              aria-label="Open loop"
              title="Open loop"
              className="grid size-6 place-items-center rounded-full text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
            >
              <ExternalLink size={12} />
            </Link>
          )}
          <IconButton
            variant="ghost"
            size="xs"
            aria-label="Remove from board"
            title="Remove from board"
            onClick={onRemove}
          >
            <X size={12} />
          </IconButton>
        </div>
      </div>
      {item.current_version > 0 && layers.length > 0 ? (
        <div className="relative min-h-0 flex-1">
          {layers.map((layer) => {
            // Fresh data is wiped in left-to-right behind the rainbow
            // scanline; theme/zoom reloads keep the quiet crossfade.
            const wipe = layer.fresh && !reduce
            return (
              <motion.div
                key={layer.src}
                className="absolute inset-0"
                initial={false}
                animate={
                  wipe
                    ? {
                        opacity: layer.ready ? 1 : 0,
                        clipPath: layer.ready ? 'inset(0 0% 0 0)' : 'inset(0 100% 0 0)',
                      }
                    : {
                        opacity: layer.ready ? 1 : 0,
                        scale: layer.ready ? 1 : reduce ? 1 : 0.985,
                        y: layer.ready ? 0 : reduce ? 0 : 6,
                      }
                }
                transition={
                  reduce
                    ? { duration: 0 }
                    : wipe
                      ? {
                          clipPath: { duration: WIPE_DURATION, ease: WIPE_EASE },
                          opacity: { duration: 0 },
                        }
                      : { duration: 0.4, ease: [0.22, 1, 0.36, 1] }
                }
              >
                <iframe
                  ref={(el) => {
                    if (el) framesRef.current.set(layer.src, el)
                    else framesRef.current.delete(layer.src)
                  }}
                  title={item.title}
                  sandbox="allow-scripts"
                  src={layer.src}
                  onLoad={() => onLayerLoad(layer.src)}
                  className={`h-full w-full border-0 ${dragging ? 'pointer-events-none' : ''}`}
                />
              </motion.div>
            )
          })}
          {pulse > 0 && !reduce ? (
            // The scanline itself: a vertical rainbow band whose center rides
            // the wipe's reveal edge, comet-tailed to the left, then gone —
            // rainbow only in motion.
            <div
              key={pulse}
              aria-hidden
              className="pointer-events-none absolute inset-0 z-10 overflow-hidden"
            >
              <motion.div
                className="absolute inset-0"
                style={{
                  background:
                    'linear-gradient(180deg, var(--color-rainbow-1), var(--color-rainbow-2), var(--color-rainbow-3), var(--color-rainbow-4), var(--color-rainbow-5))',
                  maskImage:
                    'linear-gradient(90deg, transparent 30%, black 50%, transparent 54%)',
                  WebkitMaskImage:
                    'linear-gradient(90deg, transparent 30%, black 50%, transparent 54%)',
                }}
                initial={{ x: '-50%', opacity: 0 }}
                animate={{ x: '50%', opacity: [0, 0.85, 0.85, 0] }}
                transition={{
                  x: { duration: WIPE_DURATION, ease: WIPE_EASE },
                  opacity: { duration: WIPE_DURATION, times: [0, 0.12, 0.82, 1] },
                }}
              />
            </div>
          ) : null}
        </div>
      ) : (
        <FirstRunPlaceholder
          running={
            item.loop_last_run_status === 'running' || item.loop_last_run_status === 'starting'
          }
        />
      )}
      {item.last_error ? (
        <div
          className="shrink-0 truncate bg-danger-soft px-2.5 py-1 text-[11px] text-danger"
          title={item.last_error}
        >
          {item.last_error}
        </div>
      ) : null}
      <div
        onPointerDown={onResizePointerDown}
        aria-hidden
        className="absolute right-0 bottom-0 z-10 size-4 cursor-nwse-resize opacity-0 transition-opacity duration-150 group-hover:opacity-100"
      >
        <span className="absolute right-1 bottom-1 size-2 rounded-br border-r-2 border-b-2 border-ink-3" />
      </div>
    </div>
  )
}
