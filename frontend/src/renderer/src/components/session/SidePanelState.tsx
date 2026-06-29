import { ChevronDown } from 'lucide-react'
import { motion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { clientRuntime } from '@/lib/clientRuntime'
import { modalDialogOpen } from '@/lib/dom/modal'
import { isMobileViewport, useIsMobile } from '@/lib/hooks/useIsMobile'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { parseFileReference, type FileReference } from '../../../../shared/fileReader'
import { SIDE_PANEL_LAYOUT, type SidePanelView } from './SidePanel'

const PANEL_OPEN_KEY = 'jaz.sessionPanel'
const PANEL_MAX_WIDTH = 1180
const PANEL_MIN_THREAD_WIDTH = 360

export function useSidePanelState(sideChatAvailable = false) {
  const [open, setOpen] = useState(() => {
    const stored = localStorage.getItem(PANEL_OPEN_KEY)
    return stored === 'open' ? true : stored === 'closed' ? false : !isMobileViewport()
  })
  const [view, setView] = useState<SidePanelView>('overview')
  const [widthOverride, setWidthOverride] = useState<number | null>(null)
  const [resizing, setResizing] = useState(false)
  const [previewUrl, setPreviewUrl] = useState('')
  const [fileRef, setFileRef] = useState<FileReference | null>(null)
  const activeView = view === 'side-chat' && !sideChatAvailable ? 'overview' : view
  const layout = SIDE_PANEL_LAYOUT[activeView]
  const defaultWidth = layout.width
  const maxWidth = sidePanelMaxWidth(defaultWidth)
  const width = layout.resizable ? clampSidePanelWidth(widthOverride ?? defaultWidth, defaultWidth) : defaultWidth

  useEffect(() => {
    localStorage.setItem(PANEL_OPEN_KEY, open ? 'open' : 'closed')
  }, [open])

  const toggle = useCallback(() => setOpen((value) => !value), [])
  const resize = useCallback((next: number) => setWidthOverride(clampSidePanelWidth(next, defaultWidth)), [defaultWidth])

  const selectView = useCallback((next: SidePanelView) => {
    setView(next)
    setOpen(true)
  }, [])

  const openPreview = useCallback((url: string) => {
    setPreviewUrl(url)
    setView('preview')
    setOpen(true)
  }, [])

  useEffect(() => clientRuntime.onOpenPreviewURL?.(openPreview), [openPreview])

  useWindowEvent('resize', () => {
    setWidthOverride((current) => (current === null ? null : clampSidePanelWidth(current, defaultWidth)))
  })

  const openFile = useCallback((file: string | FileReference) => {
    const ref = typeof file === 'string' ? parseFileReference(file) : file
    if (!ref) return false
    setFileRef(ref)
    setView('file')
    setOpen(true)
    return true
  }, [])

  useWindowEvent('keydown', (e) => {
    if (!(e.metaKey || e.ctrlKey) || !e.shiftKey || e.defaultPrevented) return
    if (e.key.toLowerCase() !== 's') return
    e.preventDefault()
    toggle()
  })

  useWindowEvent('keydown', (e) => {
    if (!e.metaKey || e.shiftKey || e.altKey || e.ctrlKey || e.defaultPrevented) return
    if (modalDialogOpen()) return
    const key = e.key.toLowerCase()
    const target = (Object.keys(SIDE_PANEL_SHORTCUT) as SidePanelView[]).find(
      (option) => SIDE_PANEL_SHORTCUT[option]?.toLowerCase() === key,
    )
    if (!target || (target === 'side-chat' && !sideChatAvailable)) return
    e.preventDefault()
    if (open && activeView === target) toggle()
    else selectView(target)
  })

  return {
    fileRef,
    open,
    previewUrl,
    resize,
    resizing,
    selectView,
    setPreviewUrl,
    setResizing,
    toggle,
    view: activeView,
    width,
    defaultWidth,
    maxWidth,
    resizable: layout.resizable,
    openFile,
    openPreview,
  }
}

function clampSidePanelWidth(width: number, minWidth: number): number {
  return Math.round(Math.min(Math.max(width, minWidth), sidePanelMaxWidth(minWidth)))
}

function sidePanelMaxWidth(minWidth: number): number {
  if (typeof window === 'undefined') return PANEL_MAX_WIDTH
  return Math.max(minWidth, Math.min(PANEL_MAX_WIDTH, window.innerWidth - PANEL_MIN_THREAD_WIDTH))
}

const SIDE_PANEL_VIEW_LABEL: Record<SidePanelView, string> = {
  overview: 'Overview',
  diff: 'Code Diff',
  preview: 'Preview',
  terminal: 'Terminal',
  file: 'File Reader',
  'side-chat': 'Side chat',
}

const SIDE_PANEL_SHORTCUT: Partial<Record<SidePanelView, string>> = {
  'side-chat': 'J',
  diff: 'D',
  preview: 'P',
  terminal: 'T',
  overview: 'O',
}

// Overview sits last so it lands on the right edge of the row. It's the default
// view, so when the panel is closed the collapsed pill is Overview pinned to
// the right — the others fan in to its left and it never moves on hover.
const BASE_VIEW_OPTIONS: SidePanelView[] = ['side-chat', 'diff', 'preview', 'terminal', 'overview']

export function SidePanelControl({
  open,
  view,
  sideChatAvailable,
  fileAvailable,
  onToggle,
  onSelectView,
}: {
  open: boolean
  view: SidePanelView
  sideChatAvailable: boolean
  fileAvailable: boolean
  onToggle: () => void
  onSelectView: (view: SidePanelView) => void
}) {
  const baseOptions = sideChatAvailable ? BASE_VIEW_OPTIONS : BASE_VIEW_OPTIONS.filter((option) => option !== 'side-chat')
  const options = fileAvailable || view === 'file' ? [...baseOptions, 'file' as const] : baseOptions
  const currentView = (view === 'file' && !fileAvailable) || (view === 'side-chat' && !sideChatAvailable) ? 'overview' : view
  const isMobile = useIsMobile()
  const metaHeld = useMetaHeld(!isMobile)
  const [menuOpen, setMenuOpen] = useState(false)
  const controlRef = useRef<HTMLDivElement>(null)
  const closeTimer = useRef<number | null>(null)
  const [hovered, setHovered] = useState(false)
  const expanded = open || hovered || metaHeld

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
    if (open && view === next) {
      onToggle()
      return
    }
    onSelectView(next)
  }

  const renderButton = (option: SidePanelView) => {
    const active = open && view === option
    const shortcut = SIDE_PANEL_SHORTCUT[option]
    return (
      <motion.button
        key={option}
        type="button"
        aria-pressed={active}
        title={`${active ? `Hide ${SIDE_PANEL_VIEW_LABEL[option]} panel` : `Open ${SIDE_PANEL_VIEW_LABEL[option]}`}${shortcut ? ` (⌘${shortcut})` : ''}`}
        onClick={() => toggleView(option)}
        whileTap={{ scale: 0.96 }}
        className={`relative flex h-7 cursor-pointer items-center rounded-full px-2.5 text-[13px] font-medium whitespace-nowrap transition-colors duration-150 ${
          active ? 'text-ink' : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
        }`}
      >
        {active ? (
          <motion.span
            layoutId="side-panel-active-pill"
            transition={{ type: 'spring', duration: 0.32, bounce: 0 }}
            className="absolute inset-0 rounded-full bg-bg shadow-sm ring-1 ring-border/50"
          />
        ) : null}
        <span className="relative flex items-center">
          {SIDE_PANEL_VIEW_LABEL[option]}
          {shortcut ? (
            <span
              aria-hidden={!metaHeld}
              className="grid transition-[grid-template-columns,opacity] duration-200 ease-out"
              style={{ gridTemplateColumns: metaHeld ? '1fr' : '0fr', opacity: metaHeld ? 1 : 0 }}
            >
              <span className="overflow-hidden">
                <span className="block pl-1.5">
                  <KeyboardShortcut value={shortcut} />
                </span>
              </span>
            </span>
          ) : null}
        </span>
      </motion.button>
    )
  }

  // Phone: the segmented row clips in the cramped title bar, so collapse it to a
  // single dropdown — current view as the trigger, all views (plus Hide) inside.
  if (isMobile) {
    return (
      <Popover
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        placement="below"
        align="end"
        trigger={
          <button
            type="button"
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((value) => !value)}
            className={`flex h-8 items-center gap-1 rounded-full px-3 text-[13px] font-medium transition-colors duration-150 ${
              open ? 'bg-bg text-ink shadow-sm ring-1 ring-border/50' : 'bg-surface text-ink-2'
            }`}
          >
            <span>{SIDE_PANEL_VIEW_LABEL[currentView]}</span>
            <ChevronDown
              size={13}
              className={`text-ink-3 transition-transform duration-150 ${menuOpen ? 'rotate-180' : ''}`}
            />
          </button>
        }
      >
        {options.map((option) => (
          <MenuRow
            key={option}
            selected={open && currentView === option}
            onClick={() => {
              onSelectView(option)
              setMenuOpen(false)
            }}
          >
            {SIDE_PANEL_VIEW_LABEL[option]}
          </MenuRow>
        ))}
        {open ? (
          <>
            <div className="my-1 border-t border-border" />
            <MenuRow
              onClick={() => {
                onToggle()
                setMenuOpen(false)
              }}
            >
              Hide panel
            </MenuRow>
          </>
        ) : null}
      </Popover>
    )
  }

  return (
    <div
      ref={controlRef}
      onPointerEnter={expand}
      onPointerLeave={collapseSoon}
      onFocus={expand}
      onBlur={(event) => {
        if (!controlRef.current?.contains(event.relatedTarget as Node | null)) collapseSoon()
      }}
      className="flex h-8 items-center rounded-full bg-surface p-0.5"
    >
      {options.map((option) => {
        // The current view is a plain flex child (the rigid anchor); the others
        // sit in a grid track that animates 1fr↔0fr. Collapsing the fr unit
        // shrinks real width with no dead zone, so the pill and its label fade
        // out together in one motion instead of text-then-whitespace.
        if (option === currentView) return renderButton(option)
        return (
          <div
            key={option}
            aria-hidden={!expanded}
            className="grid min-w-0 transition-[grid-template-columns,opacity] duration-200 ease-out"
            style={{
              gridTemplateColumns: expanded ? '1fr' : '0fr',
              opacity: expanded ? 1 : 0,
              pointerEvents: expanded ? undefined : 'none',
            }}
          >
            <div className="overflow-hidden">{renderButton(option)}</div>
          </div>
        )
      })}
    </div>
  )
}
