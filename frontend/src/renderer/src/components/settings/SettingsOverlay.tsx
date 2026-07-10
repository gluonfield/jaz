import { ArrowLeft, PanelLeft } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { BackendSwitcher } from '@/components/connection/BackendSwitcher'
import { SearchField } from '@/components/ui/SearchField'
import { dismissOnEmptyTap } from '@/lib/dom/drawer'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { ACPAgentsSettings } from './ACPAgentsSettings'
import { AgentProvidersSettings } from './AgentProvidersSettings'
import { AppearanceSettings } from './AppearanceSettings'
import { ArchivedThreadsSettings } from './ArchivedThreadsSettings'
import { BrowserSettings } from './BrowserSettings'
import { ConnectionsSettings } from './ConnectionsSettings'
import { DevicesSettings } from './DevicesSettings'
import { GeneralSettings } from './GeneralSettings'
import { KeyboardShortcutsSettings } from './KeyboardShortcutsSettings'
import { MCPSettings } from './MCPSettings'
import { MemorySettings } from './MemorySettings'
import { PersonalizationSettings } from './PersonalizationSettings'
import {
  type SettingsSection,
  useExperimentalFeaturesEnabled,
  visibleSettingsSections,
} from './sections'
import { UsageSettings } from './UsageSettings'

type SectionNavigationOptions = { replace?: boolean }

export function SettingsOverlay({
  open,
  section,
  onSectionChange,
  onClose,
  onOpenConnect,
}: {
  open: boolean
  section?: SettingsSection
  onSectionChange: (section: SettingsSection, options?: SectionNavigationOptions) => void
  onClose: () => void
  onOpenConnect: () => void
}) {
  const reduce = useReducedMotion()
  const isMobile = useIsMobile()
  const [experimentalEnabled] = useExperimentalFeaturesEnabled()
  const [query, setQuery] = useState('')
  // Phone: the nav is a full-screen drawer over the content rather than a fixed
  // column. It opens first (so a section is picked), then dismisses to reveal it.
  const [navOpen, setNavOpen] = useState(true)

  // Esc closes; restore focus to whatever was focused before opening. An open
  // transient surface inside (the backend switcher popover) owns Escape first,
  // so it dismisses itself before Escape closes the whole panel.
  useEffect(() => {
    if (!open) return
    setQuery('')
    setNavOpen(true)
    const previouslyFocused = document.activeElement as HTMLElement | null
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      if (document.querySelector('[data-escape-surface]')) return
      event.stopPropagation()
      onClose()
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      previouslyFocused?.focus?.()
    }
  }, [open, onClose])

  const visibleSections = visibleSettingsSections(experimentalEnabled)
  const q = query.trim().toLowerCase()
  const items = visibleSections.filter((item) => !q || item.label.toLowerCase().includes(q))
  const current = visibleSections.find((item) => item.id === section) ?? visibleSections[0]

  useEffect(() => {
    if (open && section && section !== current.id) onSectionChange(current.id, { replace: true })
  }, [current.id, onSectionChange, open, section])

  return createPortal(
    <AnimatePresence>
      {open ? (
        <motion.div
          className="fixed inset-0 z-modal flex bg-bg"
          initial={reduce ? { opacity: 0 } : { opacity: 0, scale: 0.99 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, scale: 0.99 }}
          transition={{ duration: 0.16, ease: 'easeOut' }}
          role="dialog"
          aria-modal="true"
          aria-label="Settings"
        >
          <aside
            onClick={isMobile ? dismissOnEmptyTap(() => setNavOpen(false)) : undefined}
            className={`flex w-[272px] shrink-0 flex-col bg-surface max-sm:absolute max-sm:inset-y-0 max-sm:left-0 max-sm:z-drawer max-sm:w-full max-sm:transition-transform max-sm:duration-300 ${
              navOpen ? '' : 'max-sm:-translate-x-full'
            }`}
          >
            <div className={`h-[52px] shrink-0 ${isMobile ? '' : 'titlebar-drag'}`} />

            <div className="px-3 pb-1.5">
              <button
                type="button"
                onClick={onClose}
                className="flex w-full items-center gap-2 rounded-full px-2.5 py-1 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink max-sm:gap-2.5 max-sm:px-3 max-sm:py-2.5 max-sm:text-[15px]"
              >
                <ArrowLeft size={15} className="text-ink-3 max-sm:size-[18px]" />
                <span className="flex-1">Back to jaz</span>
              </button>
            </div>

            {/* The backend these settings apply to — switch above everything. */}
            <div className="px-3 pb-2.5">
              <BackendSwitcher
                onConnectServer={() => {
                  onClose()
                  onOpenConnect()
                }}
              />
            </div>

            <div className="px-3 pb-2.5">
              <SearchField
                value={query}
                onChange={setQuery}
                placeholder="Search settings…"
                className="h-8 max-sm:h-11 max-sm:pl-9 max-sm:text-[15px]"
              />
            </div>

            <nav className="flex min-h-0 flex-1 flex-col gap-px overflow-y-auto px-3 pb-2.5">
              {items.map((item) => {
                const Icon = item.icon
                const selected = item.id === current.id
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => {
                      onSectionChange(item.id)
                      if (isMobile) setNavOpen(false)
                    }}
                    className={`flex items-center gap-2 rounded-full px-2.5 py-1 text-left text-[13px] transition-colors duration-150 max-sm:gap-2.5 max-sm:px-3 max-sm:py-2.5 max-sm:text-[15px] ${
                      selected
                        ? 'bg-primary-soft font-medium text-ink'
                        : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
                    }`}
                  >
                    <Icon size={15} className={`max-sm:size-[18px] ${selected ? 'text-ink' : 'text-ink-3'}`} />
                    <span className="flex-1">{item.label}</span>
                  </button>
                )
              })}
              {items.length === 0 ? (
                <p className="px-2 text-[13px] text-ink-3">No matching settings.</p>
              ) : null}
            </nav>
          </aside>

          <div className="flex min-w-0 flex-1 flex-col bg-bg">
            <div className={`flex h-[52px] shrink-0 items-center px-3 ${isMobile ? '' : 'titlebar-drag'}`}>
              {isMobile ? (
                <button
                  type="button"
                  aria-label="Settings menu"
                  title="Settings menu"
                  onClick={() => setNavOpen(true)}
                  className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                >
                  <PanelLeft size={20} />
                </button>
              ) : null}
            </div>
            <div className="min-h-0 flex-1 overflow-hidden">
              <AnimatePresence initial={false} mode="wait">
                <motion.div
                  key={current.id}
                  className="h-full"
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -4 }}
                  transition={{ duration: 0.14, ease: 'easeOut' }}
                >
                  {current.fullHeight ? (
                    <SectionContent section={current.id} onNavigate={onSectionChange} />
                  ) : (
                    <div className="h-full overflow-y-auto">
                      <div className="mx-auto max-w-[760px] px-10 pb-8 max-sm:px-4">
                        <SectionContent section={current.id} onNavigate={onSectionChange} />
                      </div>
                    </div>
                  )}
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}

function SectionContent({
  section,
  onNavigate,
}: {
  section: SettingsSection
  onNavigate: (section: SettingsSection, options?: SectionNavigationOptions) => void
}) {
  switch (section) {
    case 'general':
      return <GeneralSettings />
    case 'appearance':
      return <AppearanceSettings />
    case 'keyboard':
      return <KeyboardShortcutsSettings />
    case 'mcp':
      return <MCPSettings />
    case 'providers':
      return <AgentProvidersSettings />
    case 'agents':
      return <ACPAgentsSettings onOpenProviders={() => onNavigate('providers')} />
    case 'archived':
      return <ArchivedThreadsSettings />
    case 'personalization':
      return <PersonalizationSettings />
    case 'memory':
      return <MemorySettings />
    case 'connections':
      return <ConnectionsSettings />
    case 'browser':
      return <BrowserSettings />
    case 'usage':
      return <UsageSettings />
    case 'devices':
      return <DevicesSettings />
  }
}
