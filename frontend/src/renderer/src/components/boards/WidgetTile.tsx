import { useNavigate } from '@tanstack/react-router'
import { GripVertical, Pencil, Play, X } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import {
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useRef,
  useState,
} from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { SCANLINE_BACKGROUND, SCANLINE_MASK } from '@/components/ui/rainbow'
import {
  fetchWidgetContent,
  reportWidgetError,
  reportWidgetLayout,
  type WidgetLayoutReport,
} from '@/lib/api/boards'
import { runLoopNow } from '@/lib/api/loops'
import type { BoardItem } from '@/lib/api/types'
import { buildArtifactDocument, buildArtifactThemeCSS } from '@/lib/artifacts'
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
  return null
}

interface Layer {
  key: string
  html: string
  ready: boolean
  // Created by a version bump (fresh data), as opposed to a theme/zoom reload.
  fresh: boolean
}

const WIPE_DURATION = 0.65
const WIPE_EASE = [0.65, 0, 0.35, 1] as const

// Ghost preview of the widget to come: skeleton blocks breathing while a wave
// rolls through the bars. The dot goes rainbow while the first run is live.
function FirstRunPlaceholder({
  running,
  onClick,
}: {
  running: boolean
  onClick: (e: ReactMouseEvent) => void
}) {
  const reduce = useReducedMotion()
  const bars = [42, 68, 52, 84, 62]
  return (
    <button
      type="button"
      aria-label="Open loop"
      title="Open loop"
      onClick={onClick}
      className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 overflow-hidden px-4 transition-colors duration-150 hover:bg-surface-2/60"
    >
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
        {running ? <span className="jaz-shimmer size-1.5 shrink-0 rounded-full bg-running" /> : null}
        {running ? 'First run in progress…' : 'Waiting for the first run'}
      </p>
    </button>
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
  onOpenedInMain,
}: {
  item: BoardItem
  theme: 'light' | 'dark'
  scale: number
  dragging?: boolean
  onHeaderPointerDown: (e: ReactPointerEvent) => void
  onResizePointerDown: (e: ReactPointerEvent) => void
  onRemove: () => void
  // Fired when a board window hands the loop off to the main app, with the
  // click point so the board can flash a cue there.
  onOpenedInMain: (x: number, y: number) => void
}) {
  const navigate = useNavigate()
  const reduce = useReducedMotion()
  // Keyed by version + theme: a new version or theme change builds a fresh
  // document and crossfades; zoom changes are applied live (postMessage) and
  // deliberately stay out of the key so they never reload the tile.
  const contentKey =
    item.current_version > 0 ? `${item.widget_id}:${item.current_version}:${theme}` : ''

  // Double-buffered content: the old document stays visible until the new one
  // has loaded. Fresh versions are then wiped in by the rainbow scanline;
  // theme/zoom reloads quietly crossfade — no blank flash either way.
  const [layers, setLayers] = useState<Layer[]>([])
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
    const key = contentKey
    const controller = new AbortController()
    const fresh = pulsePendingRef.current
    void fetchWidgetContent(item.widget_id, item.current_version, controller.signal)
      .then((fragment) => {
        // Wrap the raw fragment in the shared artifact document — same design
        // system, theme, and CSP as inline artifacts. measureLayout adds the
        // tile telemetry bridge.
        const html = buildArtifactDocument(
          { title: item.title, code: fragment, loadingMessages: [] },
          buildArtifactThemeCSS(theme === 'dark'),
          { measureLayout: true },
        )
        setLayers((current) => {
          const top = current[current.length - 1]
          if (top && top.key === key) return current
          return [
            ...current.filter((layer) => layer.ready),
            { key, html, ready: false, fresh },
          ]
        })
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === 'AbortError') return
        console.error(err)
      })
    return () => controller.abort()
  }, [contentKey, item.widget_id, item.current_version, item.title, theme])

  const onLayerLoad = (loaded: string, fresh: boolean) => {
    // A freshly built document loads at zoom 1; push the board's scale in now.
    framesRef.current.get(loaded)?.contentWindow?.postMessage({ type: 'jaz:scale', scale }, '*')
    setLayers((current) =>
      current.map((layer) => (layer.key === loaded ? { ...layer, ready: true } : layer)),
    )
    if (fresh) {
      pulsePendingRef.current = false
      setPulse((p) => p + 1)
    }
    window.setTimeout(() => {
      setLayers((current) => (current.length > 1 ? current.slice(-1) : current))
    }, 800)
  }

  useEffect(() => {
    setLayers((current) => {
      const top = current[current.length - 1]
      return top && top.key === contentKey ? current : current.filter((layer) => layer.ready)
    })
  }, [contentKey])

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
        img_errors?: number
      }
      if (msg?.type === 'jaz:artifact-link' && typeof msg.href === 'string') {
        window.open(msg.href, '_blank')
      }
      if (msg?.type === 'jaz:artifact-error' && typeof msg.message === 'string') {
        const key = `${item.widget_id}@${item.current_version}`
        if (reportedRef.current === key) return
        reportedRef.current = key
        void reportWidgetError(item.widget_id, msg.message).catch(() => {})
      }
      if (msg?.type === 'jaz:artifact-layout' && typeof msg.dead_space_pct === 'number') {
        const layout: WidgetLayoutReport = {
          dead_space_pct: msg.dead_space_pct,
          overflow_px: msg.overflow_px ?? 0,
          clipped: msg.clipped ?? 0,
          img_errors: msg.img_errors ?? 0,
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

  // Zoom is applied live to every frame — theme, by contrast, is baked into the
  // rebuilt document (contentKey), so it needs no message here.
  useEffect(() => {
    for (const frame of framesRef.current.values()) {
      frame.contentWindow?.postMessage({ type: 'jaz:scale', scale }, '*')
    }
  }, [scale])

  const updated = hasTime(item.widget_updated_at) ? relativeTime(item.widget_updated_at) : ''
  const openLoop = (e: ReactMouseEvent) => {
    if (window.jaz?.windowKind === 'board') {
      window.jaz.openInMain(`/loops/${item.loop_id}`)
      onOpenedInMain(e.clientX, e.clientY)
      return
    }
    void navigate({ to: '/loops/$loopId', params: { loopId: item.loop_id } })
  }

  return (
    <div
      className={`group relative flex h-full w-full flex-col overflow-hidden rounded-card bg-surface transition-shadow duration-150 ${
        dragging ? 'shadow-lg ring-1 ring-primary/50' : ''
      }`}
    >
      <div
        onPointerDown={onHeaderPointerDown}
        className="flex h-8 shrink-0 cursor-grab select-none items-center gap-1.5 px-2.5 transition-colors duration-150 hover:bg-surface-2/60 active:cursor-grabbing"
      >
        <GripVertical
          size={13}
          aria-hidden
          className="-ml-1 shrink-0 text-ink-3 transition-colors duration-150 group-hover:text-ink-2"
        />
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
          <IconButton
            variant="ghost"
            size="xs"
            aria-label={window.jaz?.windowKind === 'board' ? 'Open loop in Jaz' : 'Open loop'}
            title={window.jaz?.windowKind === 'board' ? 'Open loop in Jaz' : 'Open loop'}
            onClick={openLoop}
          >
            <Pencil size={12} />
          </IconButton>
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
                key={layer.key}
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
                    if (el) framesRef.current.set(layer.key, el)
                    else framesRef.current.delete(layer.key)
                  }}
                  title={item.title}
                  sandbox="allow-scripts"
                  srcDoc={layer.html}
                  onLoad={() => onLayerLoad(layer.key, layer.fresh)}
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
                  background: SCANLINE_BACKGROUND,
                  maskImage: SCANLINE_MASK,
                  WebkitMaskImage: SCANLINE_MASK,
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
          onClick={openLoop}
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
