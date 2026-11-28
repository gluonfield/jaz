import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { AppWindow, ExternalLink, GripVertical, Pencil, Play, X } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import {
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useRef,
  useState,
} from 'react'
import { AppRenderer } from '@/components/apps/AppRenderer'
import { IconButton } from '@/components/ui/IconButton'
import { SCANLINE_BACKGROUND, SCANLINE_MASK } from '@/components/ui/rainbow'
import type { JazApp } from '@/lib/api/apps'
import { reportTile, type BoardTile as Tile, type TileLayoutReport } from '@/lib/api/boards'
import { activeRunStatus, loopsQuery, runLoopNow, TONE_DOT } from '@/lib/api/loops'
import { clientRuntime } from '@/lib/clientRuntime'
import { hasTime, relativeTime } from '@/lib/format/time'

function TileStatusDot({ tile }: { tile: Tile }) {
  if (activeRunStatus(tile.loop?.last_run_status)) {
    return <span title="Running" className={`size-1.5 shrink-0 rounded-full ${TONE_DOT.running}`} />
  }
  if (tile.loop?.last_run_status === 'error') {
    return <span title="Last run failed" className={`size-1.5 shrink-0 rounded-full ${TONE_DOT.failed}`} />
  }
  if (tile.last_error) {
    return (
      <span
        title={`App error: ${tile.last_error}`}
        className={`size-1.5 shrink-0 rounded-full ${TONE_DOT.failed}`}
      />
    )
  }
  return null
}

interface Layer {
  key: string
  app: JazApp
  ready: boolean
  // A version published while the tile was on screen.
  fresh: boolean
}

function postScale(stack: HTMLElement | null, scale: number) {
  for (const frame of stack?.querySelectorAll('iframe') ?? []) {
    frame.contentWindow?.postMessage({ type: 'jaz:scale', scale }, '*')
  }
}

const WIPE_DURATION = 0.65
const WIPE_EASE = [0.65, 0, 0.35, 1] as const

// Ghost preview of the app to come: skeleton blocks breathing while a wave
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
      aria-label="Open bot"
      title="Open bot"
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

export function BoardTile({
  tile,
  scale,
  dragging,
  onHeaderPointerDown,
  onResizePointerDown,
  onRemove,
  onEdit,
  onOpenedInMain,
}: {
  tile: Tile
  scale: number
  dragging?: boolean
  onHeaderPointerDown?: (e: ReactPointerEvent) => void
  onResizePointerDown?: (e: ReactPointerEvent) => void
  onRemove?: () => void
  onEdit?: () => void
  // Fired when a board window hands the loop off to the main app, with the
  // click point so the board can flash a cue there.
  onOpenedInMain: (x: number, y: number) => void
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const reduce = useReducedMotion()
  const { app, loop } = tile
  const version = app.current_version
  // A loop's app has no version until the loop's first run publishes one.
  const contentKey = version > 0 ? `${app.id}:${version}` : ''
  const reportsFeedback = Boolean(loop)

  // Double-buffered content: the old version stays visible until the new one
  // has loaded, then the rainbow scanline wipes the new one in.
  const [layers, setLayers] = useState<Layer[]>([])
  const stackRef = useRef<HTMLDivElement>(null)
  // One report per loaded version: a render-loop error would otherwise spam
  // the backend (and the loop's next-run prompt) on every poll.
  const reportedRef = useRef('')
  const layoutReportedRef = useRef('')
  const lastVersionRef = useRef(version)
  const [pulse, setPulse] = useState(0)

  useEffect(() => {
    if (!contentKey) return
    const fresh = version > lastVersionRef.current
    lastVersionRef.current = version
    setLayers((current) =>
      current.at(-1)?.key === contentKey
        ? current
        : [...current.filter((layer) => layer.ready), { key: contentKey, app, ready: false, fresh }],
    )
  }, [app, contentKey, version])

  // Fires again whenever a layer's document reloads (a theme change), and a
  // reloaded document is back at zoom 1.
  const onLayerReady = (layer: Layer) => {
    postScale(stackRef.current, scale)
    if (layer.ready) return
    setLayers((current) =>
      current.map((entry) => (entry.key === layer.key ? { ...entry, ready: true } : entry)),
    )
    if (layer.fresh) setPulse((p) => p + 1)
    window.setTimeout(() => {
      setLayers((current) => (current.length > 1 ? current.slice(-1) : current))
    }, 800)
  }

  useEffect(() => {
    // Only a loop reads rendering feedback, in its next run's prompt.
    if (!reportsFeedback) return
    const onMessage = (event: MessageEvent) => {
      const frames = [...(stackRef.current?.querySelectorAll('iframe') ?? [])]
      if (!frames.some((frame) => frame.contentWindow === event.source)) return
      const msg = event.data as {
        type?: string
        message?: string
        dead_space_pct?: number
        overflow_px?: number
        clipped?: number
        img_errors?: number
      }
      if (msg?.type === 'jaz:artifact-error' && typeof msg.message === 'string') {
        if (reportedRef.current === contentKey) return
        reportedRef.current = contentKey
        void reportTile(app.id, { error: msg.message }).catch(() => {})
      }
      if (msg?.type === 'jaz:artifact-layout' && typeof msg.dead_space_pct === 'number') {
        const layout: TileLayoutReport = {
          dead_space_pct: msg.dead_space_pct,
          overflow_px: msg.overflow_px ?? 0,
          clipped: msg.clipped ?? 0,
          img_errors: msg.img_errors ?? 0,
        }
        // Dedupe on version + measurement, not just version: a resize that
        // changes the layout must replace the stale report, while identical
        // re-measures (polls, crossfade layers) stay silent.
        const key = `${contentKey}:${JSON.stringify(layout)}`
        if (layoutReportedRef.current === key) return
        layoutReportedRef.current = key
        void reportTile(app.id, { layout }).catch(() => {})
      }
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [app.id, contentKey, reportsFeedback])

  useEffect(() => postScale(stackRef.current, scale), [scale])

  const updated = hasTime(app.updated_at) ? relativeTime(app.updated_at) : ''
  // A tile's loop is a bot's routine; open the bot that owns it.
  const openBot = async (e: ReactMouseEvent) => {
    const { clientX, clientY } = e
    const loops = await queryClient.ensureQueryData(loopsQuery)
    const botId = loops.find((candidate) => candidate.id === loop?.id)?.bot_id
    if (clientRuntime.windowKind === 'board') {
      clientRuntime.openInMain?.(botId ? `/bots/${botId}` : '/bots')
      onOpenedInMain(clientX, clientY)
    } else if (botId) {
      void navigate({ to: '/bots/$botId', params: { botId } })
    } else {
      void navigate({ to: '/bots' })
    }
  }

  const hasActions = Boolean(contentKey || onEdit || onRemove)

  return (
    <div
      className={`group relative flex h-full w-full flex-col overflow-hidden rounded-card border border-border bg-surface transition-shadow duration-150 ${
        dragging ? 'shadow-lg ring-1 ring-primary/50' : ''
      }`}
    >
      <div
        onPointerDown={onHeaderPointerDown}
        className={`flex h-8 shrink-0 select-none items-center gap-1.5 border-b border-border px-2.5 transition-colors duration-150 ${
          onHeaderPointerDown ? 'cursor-grab hover:bg-surface-2/60 active:cursor-grabbing' : ''
        }`}
      >
        {onHeaderPointerDown ? (
          <GripVertical
            size={13}
            aria-hidden
            className="-ml-1 shrink-0 text-ink-3 transition-colors duration-150 group-hover:text-ink-2"
          />
        ) : null}
        {loop ? (
          <TileStatusDot tile={tile} />
        ) : (
          <AppWindow size={12} className="shrink-0 text-primary" aria-hidden />
        )}
        <span className="min-w-0 flex-1 truncate text-[12px] font-medium text-ink" title={loop?.name || app.title}>
          {app.title}
        </span>
        <span
          className={`shrink-0 text-[11px] tabular-nums text-ink-3 ${
            hasActions ? 'group-hover:hidden' : ''
          }`}
        >
          {updated}
        </span>
        {hasActions ? (
          <div className="hidden shrink-0 items-center group-hover:flex">
            {contentKey ? (
              <IconButton
                variant="ghost"
                size="xs"
                aria-label="Open app"
                title="Open app"
                onClick={(e) => {
                  if (clientRuntime.windowKind === 'board') {
                    clientRuntime.openInMain?.(`/apps/${app.id}`)
                    onOpenedInMain(e.clientX, e.clientY)
                  } else {
                    void navigate({ to: '/apps/$appId', params: { appId: app.id } })
                  }
                }}
              >
                <ExternalLink size={12} />
              </IconButton>
            ) : null}
            {loop && onEdit ? (
              <>
                <IconButton
                  variant="ghost"
                  size="xs"
                  aria-label="Run loop now"
                  title="Run loop now"
                  onClick={() => void runLoopNow(loop.id).catch(() => {})}
                >
                  <Play size={12} />
                </IconButton>
                <IconButton
                  variant="ghost"
                  size="xs"
                  aria-label="Edit loop"
                  title="Edit loop"
                  onClick={onEdit}
                >
                  <Pencil size={12} />
                </IconButton>
              </>
            ) : null}
            {onRemove ? (
              <IconButton
                variant="ghost"
                size="xs"
                aria-label="Remove from board"
                title="Remove from board"
                onClick={onRemove}
              >
                <X size={12} />
              </IconButton>
            ) : null}
          </div>
        ) : null}
      </div>
      {contentKey ? (
        <div
          ref={stackRef}
          className={`relative m-2.5 min-h-0 flex-1 ${dragging ? 'pointer-events-none' : ''}`}
        >
          {layers.map((layer) => {
            // A fresh version is wiped in left-to-right behind the rainbow
            // scanline; the first load quietly fades in.
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
                <AppRenderer app={layer.app} fill onReady={() => onLayerReady(layer)} />
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
        <FirstRunPlaceholder onClick={openBot} running={activeRunStatus(loop?.last_run_status)} />
      )}
      {onResizePointerDown ? (
        <div
          onPointerDown={onResizePointerDown}
          aria-hidden
          className="absolute right-0 bottom-0 z-10 size-4 cursor-nwse-resize opacity-0 transition-opacity duration-150 group-hover:opacity-100"
        >
          <span className="absolute right-1 bottom-1 size-2 rounded-br border-r-2 border-b-2 border-ink-3" />
        </div>
      ) : null}
    </div>
  )
}
