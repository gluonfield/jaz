import { ArchiveRestore, ArrowLeft, Bot, Plug, Search, Sparkles, SlidersHorizontal } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { AgentSettings } from './AgentSettings'
import { ArchivedThreadsSettings } from './ArchivedThreadsSettings'
import { GeneralSettings } from './GeneralSettings'
import { MCPSettings } from './MCPSettings'
import { PersonalizationSettings } from './PersonalizationSettings'

type Section = 'general' | 'personalization' | 'mcp' | 'agents' | 'archived'

type NavItem = { id: Section; label: string; icon: typeof Bot; fullHeight?: boolean }

const GROUPS: Array<{ label: string; items: NavItem[] }> = [
  {
    label: 'Personal',
    items: [
      { id: 'general', label: 'General', icon: SlidersHorizontal },
      { id: 'personalization', label: 'Personalization', icon: Sparkles, fullHeight: true },
    ],
  },
  {
    label: 'Integrations',
    items: [
      { id: 'mcp', label: 'MCP servers', icon: Plug },
      { id: 'agents', label: 'Agents (ACP)', icon: Bot },
    ],
  },
  {
    label: 'Archived',
    items: [{ id: 'archived', label: 'Archived threads', icon: ArchiveRestore }],
  },
]

const ALL_ITEMS = GROUPS.flatMap((group) => group.items)

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
  const groups = GROUPS.map((group) => ({
    ...group,
    items: group.items.filter((item) => !q || item.label.toLowerCase().includes(q)),
  })).filter((group) => group.items.length > 0)

  const current = ALL_ITEMS.find((item) => item.id === section) ?? ALL_ITEMS[0]

  return createPortal(
    <AnimatePresence>
      {open ? (
        <motion.div
          className="fixed inset-0 z-[60] flex bg-bg"
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
                className="flex w-full items-center gap-2 rounded-control px-2 py-1.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
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
                  className="h-8 w-full rounded-control bg-ink/10 pl-8 pr-2.5 text-[13px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:bg-ink/15 focus:ring-1 focus:ring-ink/25"
                />
              </div>
            </div>

            <nav className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-3 pb-3">
              {groups.map((group) => (
                <div key={group.label}>
                  <p className="px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3">
                    {group.label}
                  </p>
                  <div className="flex flex-col gap-px">
                    {group.items.map((item) => {
                      const Icon = item.icon
                      const selected = item.id === section
                      return (
                        <button
                          key={item.id}
                          type="button"
                          onClick={() => setSection(item.id)}
                          className={`flex items-center gap-2 rounded-control px-2 py-1.5 text-left text-[13px] transition-colors duration-150 ${
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
                  </div>
                </div>
              ))}
              {groups.length === 0 ? (
                <p className="px-2 text-[13px] text-ink-3">No matching settings.</p>
              ) : null}
            </nav>
          </aside>

          <div className="flex min-w-0 flex-1 flex-col">
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
    case 'mcp':
      return <MCPSettings />
    case 'agents':
      return <AgentSettings />
    case 'archived':
      return <ArchivedThreadsSettings />
    case 'personalization':
      return <PersonalizationSettings />
  }
}
