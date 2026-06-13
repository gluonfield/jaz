import { FileDiff, FileText, Globe, PanelRightOpen } from 'lucide-react'
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

export function useSidePanelState() {
  const [panelPref, setPanelPref] = useState<PanelPref>(storedPanelPref)
  const [view, setView] = useState<SidePanelView>(storedSidePanelView)
  const [previewUrl, setPreviewUrl] = useState('')
  const [fileRef, setFileRef] = useState<FileReference | null>(null)
  const [hasPanelSpace, setHasPanelSpace] = useState(false)
  const observerRef = useRef<ResizeObserver | null>(null)
  const width = SIDE_PANEL_WIDTHS[view]
  const open = panelPref === 'auto' ? hasPanelSpace : panelPref === 'open'

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
    setPanelPref(next === hasPanelSpace ? 'auto' : next ? 'open' : 'closed')
  }, [hasPanelSpace, open])

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

function SidePanelViewIcon({ view, size = 14 }: { view: SidePanelView; size?: number }) {
  switch (view) {
    case 'diff':
      return <FileDiff size={size} />
    case 'preview':
      return <Globe size={size} />
    case 'file':
      return <FileText size={size} />
    default:
      return <PanelRightOpen size={size} />
  }
}

const BASE_VIEW_OPTIONS: SidePanelView[] = ['overview', 'diff', 'preview']

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
  const [expanded, setExpanded] = useState(false)
  const collapseIfUnfocused = () => {
    if (!controlRef.current?.contains(document.activeElement)) setExpanded(false)
  }
  const toggleView = (next: SidePanelView) => {
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
      onPointerEnter={() => setExpanded(true)}
      onPointerLeave={collapseIfUnfocused}
      onFocus={() => setExpanded(true)}
      onBlur={(event) => {
        if (!controlRef.current?.contains(event.relatedTarget as Node | null)) setExpanded(false)
      }}
      className="group flex h-10 items-center rounded-full bg-surface/95 p-1 shadow-sm ring-1 ring-border/70"
    >
      <AnimatePresence initial={false} mode="popLayout">
        {expanded ? (
          <motion.div
            key="expanded"
            layout
            initial={{ opacity: 0, scale: 0.96, filter: 'blur(4px)' }}
            animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
            exit={{ opacity: 0, scale: 0.96, filter: 'blur(4px)' }}
            transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
            className="flex items-center gap-0.5"
          >
            {options.map((option) => {
              const active = open && view === option
              return (
                <motion.button
                  key={option}
                  type="button"
                  aria-pressed={active}
                  title={active ? `Hide ${SIDE_PANEL_VIEW_LABEL[option]} panel` : `Open ${SIDE_PANEL_VIEW_LABEL[option]}`}
                  onClick={() => toggleView(option)}
                  whileTap={{ scale: 0.96 }}
                  className={`relative h-8 cursor-pointer rounded-full px-2.5 text-[13px] font-medium transition-colors duration-150 ${
                    active ? 'text-ink' : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
                  }`}
                >
                  {active ? (
                    <motion.span
                      layoutId="side-panel-active-pill"
                      transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
                      className="absolute inset-0 rounded-full bg-bg shadow-sm ring-1 ring-border/60"
                    />
                  ) : null}
                  <span className="relative">{SIDE_PANEL_VIEW_LABEL[option]}</span>
                </motion.button>
              )
            })}
          </motion.div>
        ) : (
          <motion.div
            key="compact"
            layout
            initial={{ opacity: 0, scale: 0.96, filter: 'blur(4px)' }}
            animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
            exit={{ opacity: 0, scale: 0.96, filter: 'blur(4px)' }}
            transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
            className="flex"
          >
            <motion.button
              type="button"
              aria-label={open ? `Hide ${SIDE_PANEL_VIEW_LABEL[currentView]} panel` : 'Show side panel'}
              title={open ? `Hide ${SIDE_PANEL_VIEW_LABEL[currentView]} panel` : 'Show side panel'}
              onClick={() => toggleView(currentView)}
              whileTap={{ scale: 0.96 }}
              className="flex h-8 cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[13px] font-medium text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
            >
              {open ? <SidePanelViewIcon view={currentView} size={13} /> : <PanelRightOpen size={13} />}
              <span className="hidden max-w-24 truncate sm:inline">
                {open ? SIDE_PANEL_VIEW_LABEL[currentView] : 'Panel'}
              </span>
            </motion.button>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  )
}
