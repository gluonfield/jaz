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

// Always-visible segmented picker, in the home composer's flat-pill language:
// the views sit in a row of quiet buttons (no container chrome, no hover-to-
// reveal). The open view wears a lifted chip that springs between segments via
// a shared layoutId; closing leaves no chip, so an idle panel reads at a glance.
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
  const toggleView = (next: SidePanelView) => {
    // Tapping the open view closes the panel; any other view opens to it.
    if (open && view === next) {
      onToggle()
      return
    }
    onSelectView(next)
  }
  return (
    <motion.div layout className="flex items-center gap-0.5">
      <AnimatePresence initial={false} mode="popLayout">
        {options.map((option) => {
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
              initial={{ opacity: 0, scale: 0.9, filter: 'blur(4px)' }}
              animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
              exit={{ opacity: 0, scale: 0.9, filter: 'blur(4px)' }}
              transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
              className={`relative flex h-8 cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[13px] font-medium transition-colors duration-150 ${
                active ? 'text-ink' : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
              }`}
            >
              {active ? (
                <motion.span
                  layoutId="side-panel-active-pill"
                  transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
                  className="absolute inset-0 rounded-full bg-surface shadow-sm ring-1 ring-border/60"
                />
              ) : null}
              <span className="relative flex items-center gap-1.5">
                <SidePanelViewIcon view={option} size={13} />
                {SIDE_PANEL_VIEW_LABEL[option]}
              </span>
            </motion.button>
          )
        })}
      </AnimatePresence>
    </motion.div>
  )
}
