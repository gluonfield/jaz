import { ChevronDown, FileDiff, Globe, PanelRightClose, PanelRightOpen } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
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
  return value === 'diff' || value === 'preview' ? value : 'overview'
}

export function useSidePanelState() {
  const [panelPref, setPanelPref] = useState<PanelPref>(storedPanelPref)
  const [view, setView] = useState<SidePanelView>(storedSidePanelView)
  const [previewUrl, setPreviewUrl] = useState('')
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

  useWindowEvent('keydown', (e) => {
    if (!(e.metaKey || e.ctrlKey) || !e.shiftKey || e.defaultPrevented) return
    if (e.key.toLowerCase() !== 's') return
    e.preventDefault()
    toggle()
  })

  return {
    measureRef,
    open,
    previewUrl,
    selectView,
    setPreviewUrl,
    toggle,
    view,
    width,
    openPreview,
  }
}

const SIDE_PANEL_VIEW_LABEL: Record<SidePanelView, string> = {
  overview: 'Overview',
  diff: 'Code Diff',
  preview: 'Preview',
}

function SidePanelViewIcon({ view, size = 14 }: { view: SidePanelView; size?: number }) {
  switch (view) {
    case 'diff':
      return <FileDiff size={size} />
    case 'preview':
      return <Globe size={size} />
    default:
      return <PanelRightOpen size={size} />
  }
}

export function SidePanelControl({
  open,
  view,
  onToggle,
  onSelectView,
}: {
  open: boolean
  view: SidePanelView
  onToggle: () => void
  onSelectView: (view: SidePanelView) => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const select = (next: SidePanelView) => {
    onSelectView(next)
    setMenuOpen(false)
  }
  return (
    <div className="flex items-center rounded-full bg-surface p-0.5">
      <button
        type="button"
        aria-label={open ? 'Hide side panel' : 'Show side panel'}
        title={`${open ? 'Hide' : 'Show'} side panel (Shift+⌘S)`}
        onClick={onToggle}
        className="grid size-8 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      >
        {open ? <PanelRightClose size={16} /> : <PanelRightOpen size={16} />}
      </button>
      <Popover
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        placement="below"
        align="end"
        trigger={
          <button
            type="button"
            aria-label="Choose side panel view"
            title="Choose side panel view"
            onClick={() => setMenuOpen((value) => !value)}
            className="flex h-8 cursor-pointer items-center gap-1 rounded-full px-2 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
          >
            <SidePanelViewIcon view={view} />
            <span className="hidden max-w-20 truncate sm:inline">{SIDE_PANEL_VIEW_LABEL[view]}</span>
            <ChevronDown size={13} className="shrink-0 text-ink-3" aria-hidden />
          </button>
        }
      >
        <MenuRow selected={view === 'overview'} onClick={() => select('overview')}>
          Overview
        </MenuRow>
        <MenuRow selected={view === 'diff'} onClick={() => select('diff')}>
          Code Diff
        </MenuRow>
        <MenuRow selected={view === 'preview'} onClick={() => select('preview')}>
          Preview
        </MenuRow>
      </Popover>
    </div>
  )
}
