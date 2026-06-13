import { AnimatePresence, motion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { parseFileReference, type FileReference } from '../../../../shared/fileReader'
import { SIDE_PANEL_WIDTHS, type SidePanelView } from './SidePanel'

const PANEL_CHAT_COMFORT = 800
const PANEL_PREF_KEY = 'jaz.sessionPanel'
const PANEL_VIEW_PREF_KEY = 'jaz.sessionPanelView'

type PanelPref = 'auto' | 'open' | 'closed'

function storedPanelPref(): PanelPref {
  const value = localStorage.getItem(PANEL_PREF_KEY)
  return value === 'open' || value === 'closed' ? value : 'auto'
}

function storedSidePanelView(): SidePanelView {
  const value = localStorage.getItem(PANEL_VIEW_PREF_KEY)
  return value === 'diff' || value === 'preview' || value === 'file' ? value : 'overview'
}

export function useSidePanelState(gitAvailable: boolean) {
  const [panelPref, setPanelPref] = useState<PanelPref>(storedPanelPref)
  const [view, setView] = useState<SidePanelView>(storedSidePanelView)
  const [previewUrl, setPreviewUrl] = useState('')
  const [fileRef, setFileRef] = useState<FileReference | null>(null)
  const [hasPanelSpace, setHasPanelSpace] = useState(false)
  const observerRef = useRef<ResizeObserver | null>(null)
  const width = SIDE_PANEL_WIDTHS[view]
  // Auto-open only earns its keep on a git repo — Overview/Diff have little to
  // show otherwise. Explicit 'open' (a user pick) still opens anywhere.
  const autoOpen = hasPanelSpace && gitAvailable
  const open = panelPref === 'auto' ? autoOpen : panelPref === 'open'

  const measureRef = useCallback((el: HTMLDivElement | null) => {
    observerRef.current?.disconnect()
    observerRef.current = null
    if (!el) return
    const update = () => setHasPanelSpace(el.clientWidth >= PANEL_CHAT_COMFORT + width)
    const observer = new ResizeObserver(update)
    observer.observe(el)
    update()
    observerRef.current = observer
  }, [width])

  useEffect(() => {
    localStorage.setItem(PANEL_PREF_KEY, panelPref)
  }, [panelPref])
  useEffect(() => {
    localStorage.setItem(PANEL_VIEW_PREF_KEY, view)
  }, [view])

  const toggle = useCallback(() => {
    const next = !open
    setPanelPref(next === autoOpen ? 'auto' : next ? 'open' : 'closed')
  }, [autoOpen, open])

  const selectView = useCallback((next: SidePanelView) => {
    setView(next)
    setPanelPref('open')
  }, [])

  const openPreview = useCallback((url: string) => {
    setPreviewUrl(url)
    setView('preview')
    setPanelPref('open')
  }, [])

  const openFile = useCallback((file: string | FileReference) => {
    const ref = typeof file === 'string' ? parseFileReference(file) : file
    if (!ref) return false
    setFileRef(ref)
    setView('file')
    setPanelPref('open')
    return true
  }, [])

  useWindowEvent('keydown', (e) => {
    if (!(e.metaKey || e.ctrlKey) || !e.shiftKey || e.defaultPrevented) return
    if (e.key.toLowerCase() !== 's') return
    e.preventDefault()
    toggle()
  })

  return {
    measureRef,
    fileRef,
    open,
    previewUrl,
    selectView,
    setPreviewUrl,
    toggle,
    view,
    width,
    openFile,
    openPreview,
  }
}

const SIDE_PANEL_VIEW_LABEL: Record<SidePanelView, string> = {
  overview: 'Overview',
  diff: 'Code Diff',
  preview: 'Preview',
  file: 'File Reader',
}

const BASE_VIEW_OPTIONS: SidePanelView[] = ['overview', 'diff', 'preview']

// A quiet segmented control on a single surface track, sized like the home
// composer's pills. At rest with the panel closed it collapses to just the
// current view; hover (or focus) unfurls the full row, and an open panel keeps
// it unfurled. The current view is the same keyed button throughout, so the
// siblings simply fan in beside it rather than the whole control flickering.
export function SidePanelControl({
  open,
  view,
  fileAvailable,
  onToggle,
  onSelectView,
}: {
  open: boolean
  view: SidePanelView
  fileAvailable: boolean
  onToggle: () => void
  onSelectView: (view: SidePanelView) => void
}) {
  const options = fileAvailable || view === 'file' ? [...BASE_VIEW_OPTIONS, 'file' as const] : BASE_VIEW_OPTIONS
  const currentView = view === 'file' && !fileAvailable ? 'overview' : view
  const controlRef = useRef<HTMLDivElement>(null)
  const closeTimer = useRef<number | null>(null)
  const [hovered, setHovered] = useState(false)
  const expanded = open || hovered
  const visible = options.filter((option) => expanded || option === currentView)

  // Hover intent: collapse on a short delay, cancelled the moment the pointer
  // (or focus) comes back. Without it a transient pointerleave during the
  // expand reflow tears the row down mid-reach, so a tab you're moving toward
  // disappears before you can click it.
  const cancelClose = () => {
    if (closeTimer.current !== null) {
      window.clearTimeout(closeTimer.current)
      closeTimer.current = null
    }
  }
  const expand = () => {
    cancelClose()
    setHovered(true)
  }
  const collapseSoon = () => {
    cancelClose()
    closeTimer.current = window.setTimeout(() => {
      closeTimer.current = null
      if (!controlRef.current?.contains(document.activeElement)) setHovered(false)
    }, 160)
  }
  useEffect(() => () => {
    if (closeTimer.current !== null) window.clearTimeout(closeTimer.current)
  }, [])

  const toggleView = (next: SidePanelView) => {
    // Tapping the open view closes the panel; any other view opens to it.
    if (open && view === next) {
      onToggle()
      return
    }
    onSelectView(next)
  }

  return (
    <motion.div
      ref={controlRef}
      layout
      onPointerEnter={expand}
      onPointerLeave={collapseSoon}
      onFocus={expand}
      onBlur={(event) => {
        if (!controlRef.current?.contains(event.relatedTarget as Node | null)) collapseSoon()
      }}
      transition={{ type: 'spring', duration: 0.34, bounce: 0 }}
      className="flex h-8 items-center gap-0.5 rounded-full bg-surface p-0.5"
    >
      <AnimatePresence initial={false} mode="popLayout">
        {visible.map((option) => {
          const active = open && view === option
          return (
            <motion.button
              key={option}
              type="button"
              layout
              aria-pressed={active}
              title={active ? `Hide ${SIDE_PANEL_VIEW_LABEL[option]} panel` : `Open ${SIDE_PANEL_VIEW_LABEL[option]}`}
              onClick={() => toggleView(option)}
              whileTap={{ scale: 0.96 }}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{
                layout: { type: 'spring', duration: 0.34, bounce: 0 },
                opacity: { duration: 0.14, ease: 'easeOut' },
              }}
              className={`relative h-7 cursor-pointer rounded-full px-2.5 text-[13px] font-medium whitespace-nowrap transition-colors duration-150 ${
                active ? 'text-ink' : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
              }`}
            >
              {active ? (
                <motion.span
                  layoutId="side-panel-active-pill"
                  transition={{ type: 'spring', duration: 0.34, bounce: 0 }}
                  className="absolute inset-0 rounded-full bg-bg shadow-sm ring-1 ring-border/50"
                />
              ) : null}
              <span className="relative">{SIDE_PANEL_VIEW_LABEL[option]}</span>
            </motion.button>
          )
        })}
      </AnimatePresence>
    </motion.div>
  )
}
