import { FileDiff, FileText, Globe, PanelRightClose, PanelRightOpen, X } from 'lucide-react'
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
  const compactClick = () => {
    if (open) return
    onSelectView(view === 'file' && !fileAvailable ? 'overview' : view)
  }
  return (
    <div className="group flex h-10 items-center rounded-[12px] bg-surface/95 p-1 shadow-[0_10px_28px_rgba(0,0,0,0.14)] ring-1 ring-border/80">
      <div className="flex items-center group-hover:hidden group-focus-within:hidden">
        {open ? (
          <>
            <button
              type="button"
              aria-label={`Side panel: ${SIDE_PANEL_VIEW_LABEL[view]}`}
              className="flex h-8 cursor-default items-center gap-1.5 rounded-[8px] px-2.5 text-[13px] text-ink-2"
            >
              <SidePanelViewIcon view={view} />
              <span className="max-w-24 truncate">{SIDE_PANEL_VIEW_LABEL[view]}</span>
            </button>
            <button
              type="button"
              aria-label="Hide side panel"
              title="Hide side panel"
              onClick={onToggle}
              className="grid size-8 cursor-pointer place-items-center rounded-[8px] text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
            >
              <X size={15} />
            </button>
          </>
        ) : (
          <button
            type="button"
            aria-label="Show side panel"
            title="Show side panel"
            onClick={compactClick}
            className="grid size-8 cursor-pointer place-items-center rounded-[8px] text-ink-2 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
          >
            <PanelRightOpen size={16} />
          </button>
        )}
      </div>
      <div className="hidden items-center gap-0.5 group-hover:flex group-focus-within:flex">
        {options.map((option) => (
          <button
            key={option}
            type="button"
            aria-pressed={open && view === option}
            onClick={() => onSelectView(option)}
            className={`h-8 cursor-pointer rounded-[8px] px-2.5 text-[13px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] ${
              open && view === option
                ? 'bg-bg text-ink shadow-sm ring-1 ring-border/70'
                : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
            }`}
          >
            {SIDE_PANEL_VIEW_LABEL[option]}
          </button>
        ))}
        {open ? (
          <button
            type="button"
            aria-label="Hide side panel"
            title="Hide side panel"
            onClick={onToggle}
            className="grid size-8 cursor-pointer place-items-center rounded-[8px] text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
          >
            <PanelRightClose size={15} />
          </button>
        ) : null}
      </div>
    </div>
  )
}
