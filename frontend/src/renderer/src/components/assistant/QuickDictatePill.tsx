import { ArrowUp, Check, LoaderCircle, Mic, X } from 'lucide-react'
import { type CSSProperties, useCallback, useEffect, useState } from 'react'
import { flushSync } from 'react-dom'
import type { QuickDictateState } from '@shared/assistant'
import { ISLAND_CAPSULE_WIDTH, ISLAND_RETRACT_MS } from '@shared/notifications'
import { BotIcon } from '@/components/bots/BotAvatar'
import { clientRuntime } from '@/lib/clientRuntime'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import '@/components/notifications/island.css'

const FLARE = 6
const SPRING = 'cubic-bezier(0.2, 0.1, 0.25, 1)'
const WAVE_BARS = 5

const PHASE_LABEL: Record<QuickDictateState['phase'], { text: string; color: string }> = {
  starting: { text: 'Starting', color: 'rgba(255,255,255,0.65)' },
  downloading: { text: 'Preparing', color: 'rgba(255,255,255,0.65)' },
  listening: { text: 'Listening', color: 'oklch(0.75 0.16 250)' },
  transcribing: { text: 'Finishing', color: 'rgba(255,255,255,0.7)' },
  sending: { text: 'Sending', color: 'rgba(255,255,255,0.7)' },
  sent: { text: 'Sent', color: 'oklch(0.74 0.15 150)' },
  error: { text: 'Failed', color: 'oklch(0.7 0.18 25)' },
}

export function QuickDictatePill() {
  const [state, setState] = useState<QuickDictateState | null>(null)
  const [open, setOpen] = useState(false)

  useEffect(() => {
    const api = clientRuntime.assistant
    if (!api) return
    return api.subscribeQuickDictate((next) => {
      setOpen(Boolean(next))
      if (next) setState(next)
    })
  }, [])

  return state ? <Pill state={state} open={open} /> : null
}

function Pill({ state, open }: { state: QuickDictateState; open: boolean }) {
  const command = clientRuntime.assistant!.sendQuickDictateCommand
  const reduced = useReducedEffectsMotion()
  const [drop, setDrop] = useState(0)
  const [fit, setFit] = useState(0)

  const measureBody = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    const observer = new ResizeObserver(() => setDrop(node.offsetHeight))
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  const measureBand = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    const observer = new ResizeObserver(() => flushSync(() => setFit(node.offsetWidth)))
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  const notch = state.notch
  const band = notch?.height ?? 34
  const gap = notch?.width ?? 0
  const width = ISLAND_CAPSULE_WIDTH - 2 * FLARE
  const closed = notch ? gap - 2 * FLARE : band
  const shown = open || reduced
  const even = notch !== null
  const showBody = Boolean(state.text || state.error || state.phase === 'listening' || state.phase === 'sent')
  const card = showBody && shown ? drop : 0
  const radius = card ? 16 : notch ? 10 : band / 2
  const fade = notch ? 0 : 150
  const close = ISLAND_RETRACT_MS
  const transition = reduced
    ? 'opacity 150ms'
    : open
      ? `width 420ms ${SPRING}, height 420ms ${SPRING} 60ms, border-radius 420ms ${SPRING} 60ms, --island-open 280ms ${SPRING} 90ms, opacity ${fade}ms`
      : `width ${close}ms ${SPRING}, height ${close - 80}ms ${SPRING}, border-radius ${close - 80}ms ${SPRING}, --island-open 120ms, opacity ${fade}ms ${close - fade}ms`

  const status = PHASE_LABEL[state.phase]
  const canOpen = Boolean(state.botId) && (state.phase === 'sent' || state.phase === 'error')

  return (
    <div
      onPointerEnter={() => command({ type: 'hover', inside: true })}
      onPointerLeave={() => command({ type: 'hover', inside: false })}
      className={`group absolute top-0 left-1/2 w-(--width) -translate-x-1/2 text-white starting:w-(--closed) starting:opacity-0 starting:[--island-open:0] ${reduced ? 'duration-150!' : ''} ${open ? '' : 'pointer-events-none opacity-0'} ${shown ? '[--island-open:1]' : ''}`}
      style={
        {
          '--width': `${shown ? fit : closed}px`,
          '--closed': `${closed}px`,
          height: band + card,
          borderRadius: notch ? `0 0 ${radius}px ${radius}px` : radius,
          transition,
        } as CSSProperties
      }
    >
      {notch && (
        <>
          <span className="absolute top-0 right-full size-1.5 bg-[radial-gradient(circle_at_0_100%,transparent_6px,#000_6.5px)]" />
          <span className="absolute top-0 left-full size-1.5 bg-[radial-gradient(circle_at_100%_100%,transparent_6px,#000_6.5px)]" />
        </>
      )}
      <div className="absolute inset-0 overflow-hidden rounded-[inherit] bg-black shadow-[0_12px_24px_-10px_rgb(0_0_0/calc(var(--island-open)*0.5))] *:opacity-(--island-open) *:blur-[calc((1-var(--island-open))*4px)]">
        <div
          className="absolute top-0 left-1/2 grid min-w-90 -translate-x-1/2"
          style={{ width: even ? 'max-content' : width }}
        >
          <div
            ref={measureBand}
            className="grid items-center text-[12px]"
            style={{ height: band, gridTemplateColumns: `1fr ${gap}px ${even ? '1fr' : 'max-content'}` }}
          >
            <button
              type="button"
              onClick={() => canOpen && command({ type: 'open' })}
              className={`flex min-w-0 items-center gap-1.5 overflow-hidden pr-1.5 pl-3 text-left ${canOpen ? 'cursor-pointer' : 'cursor-default'}`}
            >
              {state.avatar ? (
                <BotIcon avatars={[state.avatar]} size={18} mood={state.phase === 'error' ? 'failed' : undefined} live={state.phase === 'listening'} />
              ) : (
                <Mic size={14} className="shrink-0 text-white/80" />
              )}
              {!even && <span className="truncate font-medium">{state.botName}</span>}
            </button>
            <span className="col-start-3 flex min-w-12 items-center justify-end gap-1.5 pr-3 pl-1.5">
              {state.phase === 'listening' ? (
                <LevelBars level={state.level} />
              ) : state.phase === 'starting' || state.phase === 'downloading' || state.phase === 'transcribing' || state.phase === 'sending' ? (
                <LoaderCircle size={12} className="animate-spin text-white/70" />
              ) : state.phase === 'sent' ? (
                <Check size={13} style={{ color: status.color }} />
              ) : null}
              <span className="font-semibold whitespace-nowrap" style={{ color: status.color }}>
                {status.text}
              </span>
            </span>
          </div>

          {showBody && (
            <div ref={measureBody} className="grid gap-2 px-3 pt-1 pb-2.5 contain-inline-size">
              <div className="flex items-start justify-between gap-2.5">
                <button
                  type="button"
                  onClick={() => canOpen && command({ type: 'open' })}
                  className={`min-w-0 flex-1 text-left ${canOpen ? 'cursor-pointer' : 'cursor-default'}`}
                >
                  {even && <p className="truncate text-[12px] font-medium text-white/60">{state.botName}</p>}
                  {state.error ? (
                    <p className="line-clamp-3 text-[13px] leading-[1.4] text-red-300">{state.error}</p>
                  ) : state.text ? (
                    <p className="line-clamp-3 text-[13px] leading-[1.4] text-white/95">{state.text}</p>
                  ) : state.phase === 'listening' ? (
                    <p className="text-[13px] leading-[1.4] text-white/55">Speak now — pauses auto-send…</p>
                  ) : (
                    <p className="text-[13px] leading-[1.4] text-white/75">Sent to {state.botName}</p>
                  )}
                </button>

                {state.phase === 'listening' || state.phase === 'starting' || state.phase === 'downloading' ? (
                  <div className="flex shrink-0 items-center gap-1.5 pt-0.5">
                    <button
                      type="button"
                      aria-label="Cancel dictation"
                      title="Cancel"
                      onClick={() => command({ type: 'cancel' })}
                      className="grid size-6 cursor-pointer place-items-center rounded-full bg-white/12 text-white/80 hover:bg-white/20 hover:text-white"
                    >
                      <X size={12} />
                    </button>
                    {state.phase === 'listening' && (
                      <button
                        type="button"
                        aria-label="Send now"
                        title="Send now"
                        onClick={() => command({ type: 'send' })}
                        className="grid size-6 cursor-pointer place-items-center rounded-full bg-white text-black hover:bg-white/90"
                      >
                        <ArrowUp size={13} strokeWidth={2.5} />
                      </button>
                    )}
                  </div>
                ) : (
                  <button
                    type="button"
                    aria-label="Dismiss"
                    onClick={() => command({ type: 'cancel' })}
                    className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-full bg-white/10 text-white/70 hover:bg-white/20 hover:text-white"
                  >
                    <X size={11} />
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function LevelBars({ level }: { level: number }) {
  const scaled = Math.min(1, level * 6)
  return (
    <span aria-hidden className="flex h-3.5 items-center gap-0.5">
      {Array.from({ length: WAVE_BARS }, (_, index) => {
        const shape = 1 - Math.abs(index - 2) * 0.22
        const height = Math.max(3, Math.round(3 + scaled * 11 * shape))
        return (
          <span
            key={index}
            className="w-0.5 rounded-full bg-sky-400 transition-[height] duration-75"
            style={{ height }}
          />
        )
      })}
    </span>
  )
}
