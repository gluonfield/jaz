import {
  ArchiveRestore,
  ArrowLeft,
  Bot,
  Boxes,
  Brain,
  ChartNoAxesColumn,
  Keyboard,
  MonitorSmartphone,
  Plug,
  Search,
  Sparkles,
  SlidersHorizontal,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { ACPAgentsSettings } from './ACPAgentsSettings'
import { AgentProvidersSettings } from './AgentProvidersSettings'
import { ArchivedThreadsSettings } from './ArchivedThreadsSettings'
import { DevicesSettings } from './DevicesSettings'
import { GeneralSettings } from './GeneralSettings'
import { KeyboardShortcutsSettings } from './KeyboardShortcutsSettings'
import { MCPSettings } from './MCPSettings'
import { MemorySettings } from './MemorySettings'
import { PersonalizationSettings } from './PersonalizationSettings'
import { UsageSettings } from './UsageSettings'

type Section =
  | 'general'
  | 'personalization'
  | 'memory'
  | 'usage'
  | 'devices'
  | 'keyboard'
  | 'mcp'
  | 'providers'
  | 'agents'
  | 'archived'

type NavItem = { id: Section; label: string; icon: typeof Bot; fullHeight?: boolean }

const NAV: NavItem[] = [
  { id: 'general', label: 'General', icon: SlidersHorizontal },
  { id: 'personalization', label: 'Personalization', icon: Sparkles, fullHeight: true },
  { id: 'memory', label: 'Memory', icon: Brain },
  { id: 'usage', label: 'Usage', icon: ChartNoAxesColumn },
  { id: 'devices', label: 'Devices', icon: MonitorSmartphone },
  { id: 'keyboard', label: 'Keyboard shortcuts', icon: Keyboard },
  { id: 'mcp', label: 'MCP servers', icon: Plug },
  { id: 'providers', label: 'Model Providers', icon: Boxes },
  { id: 'agents', label: 'Agents (ACP)', icon: Bot },
  { id: 'archived', label: 'Archived threads', icon: ArchiveRestore },
]

export function SettingsOverlay({ open, onClose }: { open: boolean; onClose: () => void }) {
  const reduce = useReducedMotion()
  const [section, setSection] = useState<Section>('general')
  const [query, setQuery] = useState('')

  // Esc closes; restore focus to whatever was focused before opening.
  useEffect(() => {
    if (!open) return
    setQuery('')
    const previouslyFocused = document.activeElement as HTMLElement | null
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      previouslyFocused?.focus?.()
    }
  }, [open, onClose])

  const q = query.trim().toLowerCase()
  const items = NAV.filter((item) => !q || item.label.toLowerCase().includes(q))

  const current = NAV.find((item) => item.id === section) ?? NAV[0]

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
          <aside className="flex w-[272px] shrink-0 flex-col border-r border-border bg-surface">
            <div className="titlebar-drag h-[52px] shrink-0" />

            <div className="px-3 pb-2">
              <button
                type="button"
                onClick={onClose}
                className="flex w-full items-center gap-2 rounded-full px-2.5 py-1.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
              >
                <ArrowLeft size={15} className="text-ink-3" />
                <span className="flex-1">Back to jaz</span>
              </button>
            </div>

            <div className="px-3 pb-3">
              <div className="relative">
                <Search
                  size={14}
                  className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-3"
                />
                <input
                  type="text"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search settings…"
                  aria-label="Search settings"
                  className="h-8 w-full rounded-full bg-ink/10 pl-8 pr-3 text-[13px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25"
                />
              </div>
            </div>

            <nav className="flex min-h-0 flex-1 flex-col gap-px overflow-y-auto px-3 pb-3">
              {items.map((item) => {
                const Icon = item.icon
                const selected = item.id === section
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setSection(item.id)}
                    className={`flex items-center gap-2 rounded-full px-2.5 py-1.5 text-left text-[13px] transition-colors duration-150 ${
                      selected
                        ? 'bg-primary-soft font-medium text-ink'
                        : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
                    }`}
                  >
                    <Icon size={15} className={selected ? 'text-ink' : 'text-ink-3'} />
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
            <div className="titlebar-drag h-[52px] shrink-0" />
            <div className="min-h-0 flex-1 overflow-hidden">
              <AnimatePresence initial={false} mode="wait">
                <motion.div
                  key={section}
                  className="h-full"
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -4 }}
                  transition={{ duration: 0.14, ease: 'easeOut' }}
                >
                  {current.fullHeight ? (
                    <SectionContent section={section} />
                  ) : (
                    <div className="h-full overflow-y-auto">
                      <div className="mx-auto max-w-[760px] px-10 pb-12">
                        <SectionContent section={section} />
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

function SectionContent({ section }: { section: Section }) {
  switch (section) {
    case 'general':
      return <GeneralSettings />
    case 'keyboard':
      return <KeyboardShortcutsSettings />
    case 'mcp':
      return <MCPSettings />
    case 'providers':
      return <AgentProvidersSettings />
    case 'agents':
      return <ACPAgentsSettings />
    case 'archived':
      return <ArchivedThreadsSettings />
    case 'personalization':
      return <PersonalizationSettings />
    case 'memory':
      return <MemorySettings />
    case 'usage':
      return <UsageSettings />
    case 'devices':
      return <DevicesSettings />
  }
}
