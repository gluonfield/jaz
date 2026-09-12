import { Sparkles } from 'lucide-react'
import type { MouseEventHandler, ReactNode } from 'react'

export type ComposerVariant = 'card' | 'launcher'

// What a layout arranges: the composer's interaction state plus its pieces.
// Layouts own every variant-specific class, including the gating that keeps
// the controls inert under the dictation bar.
export type ComposerLayoutProps = {
  focused?: boolean
  effectsEnabled?: boolean
  dragging?: boolean
  dictating?: boolean
  /** footer caption; the card layout has no footer */
  hint?: string
  textarea: ReactNode
  /** context chips and attachments; renders nothing when the draft has none */
  extras?: ReactNode
  tools: ReactNode
  actions: ReactNode
  error?: ReactNode
  dictationBar?: ReactNode
  /** a click on the surface itself, outside any control (focus the input) */
  onSurfaceClick?: () => void
}

const surfaceClick =
  (onSurfaceClick?: () => void): MouseEventHandler<HTMLDivElement> =>
  (event) => {
    if ((event.target as HTMLElement).closest('button, textarea, input')) return
    onSurfaceClick?.()
  }

// Card, agent-council style: the surface tone IS the card, no border; the
// textarea sits above one toolbar row carrying both control groups.
export function CardLayout({
  focused = false,
  effectsEnabled = true,
  dragging = false,
  dictating = false,
  textarea,
  extras,
  tools,
  actions,
  error,
  dictationBar,
  onSurfaceClick,
}: ComposerLayoutProps) {
  const ring = effectsEnabled ? '' : focused ? 'ring-2 ring-primary' : 'ring-1 ring-border'
  const lift = dragging ? 'shadow-[0_0_0_1px_var(--color-primary),0_10px_35px_rgba(0,0,0,0.16)]' : ''
  return (
    <div
      className={`relative flex cursor-text flex-col gap-1.5 rounded-[12px] bg-surface p-2.5 transition-shadow ${ring} ${lift}`}
      onClick={surfaceClick(onSurfaceClick)}
    >
      {extras}
      <div>{textarea}</div>
      {error ? <div className="pl-2">{error}</div> : null}
      <div className="relative">
        <div
          className={`flex min-h-10 items-center justify-between gap-2.5 max-sm:items-end ${dictating ? 'invisible' : ''}`}
          inert={dictating}
        >
          {tools}
          {actions}
        </div>
        {dictating ? <div className="absolute inset-x-0 bottom-0">{dictationBar}</div> : null}
      </div>
    </div>
  )
}

// Launcher, the spotlight's geometry: one input row behind a leading mark, the
// controls in a footer under a hairline, the whole card ringed and floating.
export function LauncherLayout({
  focused = false,
  effectsEnabled = true,
  dragging = false,
  dictating = false,
  hint,
  textarea,
  extras,
  tools,
  actions,
  error,
  dictationBar,
  onSurfaceClick,
}: ComposerLayoutProps) {
  const ring = dragging || (!effectsEnabled && focused) ? 'ring-primary' : 'ring-border/60'
  return (
    <div
      className={`relative flex cursor-text flex-col rounded-[18px] bg-surface shadow-[0_18px_50px_-12px_rgba(0,0,0,0.45)] ring-1 ${ring}`}
      onClick={surfaceClick(onSurfaceClick)}
    >
      <div className="flex items-center gap-3 px-4 py-3">
        <Sparkles size={20} className="shrink-0 text-primary" />
        <div className="min-w-0 flex-1">{textarea}</div>
        <div className={dictating ? 'invisible' : ''} inert={dictating}>
          {actions}
        </div>
      </div>
      <div className="flex flex-col gap-1.5 px-2.5 pb-3 empty:hidden">{extras}</div>
      {error ? <div className="px-4 pb-2">{error}</div> : null}
      <div className="relative border-t border-border/40 px-3 py-2">
        <div className={`flex items-center gap-2.5 ${dictating ? 'invisible' : ''}`} inert={dictating}>
          {tools}
          {hint ? <span className="ml-auto shrink-0 pr-1 text-[12px] text-ink-3 max-sm:hidden">{hint}</span> : null}
        </div>
        {dictating ? <div className="absolute inset-x-3 top-1/2 -translate-y-1/2">{dictationBar}</div> : null}
      </div>
    </div>
  )
}

// Per-variant inputs for the slots the composer builds itself: the send/stop
// button size, the textarea's type, and the focus comet's corner radius.
export const LAYOUTS = {
  card: { Layout: CardLayout, radius: 12, actionSize: 'md', textClass: undefined, minHeightClass: undefined },
  launcher: {
    Layout: LauncherLayout,
    radius: 18,
    actionSize: 'lg',
    textClass: 'text-[15px] leading-6',
    minHeightClass: 'min-h-6',
  },
} as const
