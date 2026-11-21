import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { hasKeyboardFocus } from '@/lib/dom/keyboardFocus'
import { layoutPoint, layoutRect, layoutViewport } from '@/lib/dom/zoom'

const ANCHOR = '[title]:not(iframe, webview), [data-tooltip]'
const SHOW_DELAY = 150
const WARM = 400
const GAP = 6
const MARGIN = 8
const WIDE = 240
const KEYS: Record<string, string> = { Meta: '⌘', Shift: '⇧', Alt: '⌥', Control: '⌃', Enter: '↵', Escape: 'Esc' }

type Tip = { anchor: HTMLElement; text: string; keys: string; x?: number; instant: boolean }

function truncated(el: Element): boolean {
  return [el, ...el.querySelectorAll('*')].some((node) => node.scrollWidth > node.clientWidth || node.scrollHeight > node.clientHeight)
}

// The app's one tooltip, drawn for every element that has a `title`, with its
// `aria-keyshortcuts` beside the label. While the pointer is on an element its
// title is parked in `data-tooltip`, which keeps the browser's own slow
// tooltip from following; it returns when the pointer leaves. A title that
// repeats text already fully visible shows nothing. Moving on to another
// control within WARM ms skips the delay and the animation.
export function TooltipLayer() {
  const [tip, setTip] = useState<Tip | null>(null)

  useEffect(() => {
    let anchor: HTMLElement | null = null
    let shown = false
    let timer: number | undefined
    let closedAt = -Infinity

    const park = (el: HTMLElement) => {
      if (!el.title) return
      el.dataset.tooltip = el.title
      el.removeAttribute('title')
    }
    const read = (el: HTMLElement, x: number | undefined, instant: boolean): Tip | null => {
      const text = el.title || el.dataset.tooltip || ''
      const keys = (el.getAttribute('aria-keyshortcuts') ?? '').split(' ')[0].split('+').map((key) => KEYS[key] ?? key).join('')
      const adds = keys !== '' || text !== el.textContent?.trim() || truncated(el)
      return text && adds ? { anchor: el, text, keys, x, instant } : null
    }
    const observer = new MutationObserver(() => {
      if (!anchor) return
      if ('tooltip' in anchor.dataset) park(anchor)
      if (shown) setTip((current) => current && read(current.anchor, current.x, current.instant))
    })
    const hide = () => {
      window.clearTimeout(timer)
      if (shown) closedAt = performance.now()
      shown = false
      setTip(null)
    }
    const leave = () => {
      hide()
      observer.disconnect()
      if (anchor?.dataset.tooltip) {
        anchor.title = anchor.dataset.tooltip
        delete anchor.dataset.tooltip
      }
      anchor = null
    }
    const enter = (el: HTMLElement, x?: number) => {
      leave()
      anchor = el
      observer.observe(el, { attributeFilter: ['title'] })
      const instant = performance.now() - closedAt < WARM
      const open = () => {
        const next = read(el, x, instant)
        shown = next !== null
        setTip(next)
      }
      if (instant || x === undefined) open()
      else timer = window.setTimeout(open, SHOW_DELAY)
    }

    const listeners = new AbortController()
    const on = <K extends keyof WindowEventMap>(type: K, listener: (event: WindowEventMap[K]) => void, capture = true) =>
      window.addEventListener(type, listener, { capture, signal: listeners.signal })
    on('pointerover', (event) => {
      if (event.pointerType !== 'mouse') return
      const el = event.buttons ? null : (event.target as Element).closest<HTMLElement>(ANCHOR)
      if (el !== anchor) {
        if (el) enter(el, event.clientX)
        else leave()
      }
      if (el) park(el)
    })
    on('pointerout', (event) => {
      if (!event.relatedTarget) leave()
    })
    on('focusin', (event) => {
      const target = event.target as Element
      const el = hasKeyboardFocus(target) ? target.closest<HTMLElement>(ANCHOR) : null
      if (el && el !== anchor) enter(el)
    })
    on('focusout', (event) => {
      if (anchor?.contains(event.target as Node) && !anchor.matches(':hover')) leave()
    })
    on('scroll', (event) => {
      if (anchor && (event.target as Node).contains(anchor)) hide()
    })
    on('pointerdown', hide)
    on('keydown', hide)
    on('blur', leave, false)
    return () => {
      listeners.abort()
      leave()
    }
  }, [])

  return tip ? createPortal(<Tooltip tip={tip} />, document.body) : null
}

function Tooltip({ tip }: { tip: Tip }) {
  const ref = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = ref.current!
    const rect = layoutRect(tip.anchor)
    const { width, height } = layoutRect(el)
    const viewport = layoutViewport()
    const beside = tip.anchor.dataset.tooltipSide === 'right'
    const above = rect.top - GAP - height >= MARGIN
    const centre = tip.x !== undefined && rect.width > WIDE ? layoutPoint(tip.x, 0).x : rect.left + rect.width / 2
    const left = beside ? rect.right + GAP : centre - width / 2
    const top = beside ? rect.top + (rect.height - height) / 2 : above ? rect.top - GAP - height : rect.bottom + GAP
    el.style.left = `${Math.max(MARGIN, Math.min(left, viewport.width - width - MARGIN))}px`
    el.style.top = `${Math.max(MARGIN, Math.min(top, viewport.height - height - MARGIN))}px`
    el.style.setProperty('--tooltip-origin', beside ? 'left' : above ? 'bottom' : 'top')
  }, [tip])

  return (
    <div ref={ref} role="tooltip" className="pointer-events-none fixed top-0 left-0 z-tooltip w-max max-w-[min(320px,calc(100vw-16px))]">
      <div
        className={`flex origin-(--tooltip-origin) items-center gap-2 rounded-lg bg-ink px-2 py-1 text-[12px] leading-[1.35] font-medium whitespace-pre-line text-bg shadow-raised [overflow-wrap:anywhere] ${
          tip.instant ? '' : 'motion-safe:animate-[tooltip-in_125ms_ease-out]'
        }`}
      >
        {tip.text}
        {tip.keys ? <kbd className="shrink-0 font-sans text-bg/60">{tip.keys}</kbd> : null}
      </div>
    </div>
  )
}
