import { PanelLeft } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { dismissOnEmptyTap } from '@/lib/dom/drawer'
import {
  consumeEscapeKey,
  createEscapeLayerID,
  isTopEscapeLayer,
  pushEscapeLayer,
} from '@/lib/dom/escapeLayers'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { ACPAgentsSettings } from './ACPAgentsSettings'
import { AgentProvidersSettings } from './AgentProvidersSettings'
import { AppearanceSettings } from './AppearanceSettings'
import { ArchivedThreadsSettings } from './ArchivedThreadsSettings'
import { ComputerSettings } from '@/components/settings/ComputerSettings'
import { NotificationsSettings } from '@/components/settings/NotificationsSettings'
import { BrowserSettings } from './BrowserSettings'
import { ConnectionsSettings } from './ConnectionsSettings'
import { DevicesSettings } from './DevicesSettings'
import { GeneralSettings } from './GeneralSettings'
import { KeyboardShortcutsSettings } from './KeyboardShortcutsSettings'
import { MCPSettings } from './MCPSettings'
import { MemorySettings } from './MemorySettings'
import { PersonalizationSettings } from './PersonalizationSettings'
import { SettingsNav } from './SettingsNav'
import {
  type SettingsSection,
  useExperimentalFeaturesEnabled,
  visibleSettingsSections,
} from './sections'
import { UsageSettings } from './UsageSettings'
import { VoiceSettings } from '@/components/settings/VoiceSettings'

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
  const escapeLayerID = useRef<string | null>(null)
  if (!escapeLayerID.current) escapeLayerID.current = createEscapeLayerID('settings')
  // Phone: the nav is a full-screen drawer over the content rather than a fixed
  // column. It opens first (so a section is picked), then dismisses to reveal it.
  const [navOpen, setNavOpen] = useState(true)

  // Esc closes; restore focus to whatever was focused before opening. An open
  // transient surface inside (the backend switcher popover) owns Escape first,
  // so it dismisses itself before Escape closes the whole panel.
  useEffect(() => {
    if (!open) return
    setNavOpen(true)
    const layerID = escapeLayerID.current
    if (!layerID) return
    const unregisterLayer = pushEscapeLayer(layerID)
    const previouslyFocused = document.activeElement as HTMLElement | null
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      if (!isTopEscapeLayer(layerID)) return
      if (document.querySelector('[data-escape-surface]')) return
      consumeEscapeKey(event)
      onClose()
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      unregisterLayer()
      previouslyFocused?.focus?.()
    }
  }, [open, onClose])

  const visibleSections = visibleSettingsSections(experimentalEnabled)
  const current = visibleSections.find((item) => item.id === section) ?? visibleSections[0]

  useEffect(() => {
    if (open && section && section !== current.id) onSectionChange(current.id, { replace: true })
  }, [current.id, onSectionChange, open, section])

  // Desktop: the page fills the content card while its menu takes the left
  // column (see the root layout), so the rail and titlebar stay in reach.
  // Phones have no rail, so it covers the screen with the menu as a drawer.
  return (
    <AnimatePresence>
      {open ? (
        <motion.div
          className="absolute inset-x-0 top-[var(--titlebar-height,0px)] bottom-0 z-modal flex bg-bg max-sm:fixed"
          initial={reduce ? { opacity: 0 } : { opacity: 0, scale: 0.99 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, scale: 0.99 }}
          transition={{ duration: 0.16, ease: 'easeOut' }}
          role="dialog"
          aria-modal="true"
          aria-label="Settings"
        >
          {isMobile ? (
            <aside
              onClick={dismissOnEmptyTap(() => setNavOpen(false))}
              className={`absolute inset-y-0 left-0 z-drawer flex w-full flex-col bg-panel transition-transform duration-300 ${
                navOpen ? '' : '-translate-x-full'
              }`}
            >
              <div className="h-[52px] shrink-0" />
              <SettingsNav
                section={current.id}
                onSectionChange={(next) => {
                  onSectionChange(next)
                  setNavOpen(false)
                }}
                onClose={onClose}
                onOpenConnect={onOpenConnect}
              />
            </aside>
          ) : null}

          <div className="flex min-w-0 flex-1 flex-col bg-bg">
            <div className="flex h-[52px] shrink-0 items-center px-3 sm:h-6">
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
    </AnimatePresence>
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
    case 'voice':
      return <VoiceSettings />
    case 'general':
      return <GeneralSettings />
    case 'notifications':
      return <NotificationsSettings />
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
    case 'computer':
      return <ComputerSettings />
    case 'browser':
      return <BrowserSettings />
    case 'usage':
      return <UsageSettings />
    case 'devices':
      return <DevicesSettings />
  }
}
