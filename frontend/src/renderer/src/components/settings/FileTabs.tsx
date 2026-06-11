import { motion } from 'motion/react'

export interface FileTab {
  name: string
  dirty?: boolean
  badge?: string
}

// Shared tablist for file-editor settings sections (Personalization, Memory):
// mono tab labels, animated underline, unsaved-changes dot, optional badge.
export function FileTabs({
  tabs,
  active,
  onSelect,
  underlineId,
  className,
}: {
  tabs: FileTab[]
  active: string
  onSelect: (name: string) => void
  underlineId: string
  className?: string
}) {
  return (
    <div role="tablist" className={`flex gap-1 ${className ?? ''}`}>
      {tabs.map((tab) => {
        const isActive = tab.name === active
        return (
          <button
            key={tab.name}
            role="tab"
            type="button"
            aria-selected={isActive}
            onClick={() => onSelect(tab.name)}
            className={`relative -mb-px flex items-center gap-1.5 rounded-t-control px-3 py-2 font-mono text-[12px] transition-colors duration-150 ${
              isActive ? 'font-medium text-ink' : 'text-ink-2 hover:bg-surface hover:text-ink'
            }`}
          >
            {isActive ? (
              <motion.span
                layoutId={underlineId}
                className="absolute inset-x-0 -bottom-px h-0.5 rounded-full bg-primary"
                transition={{ type: 'spring', stiffness: 480, damping: 38 }}
              />
            ) : null}
            {tab.name}
            {tab.dirty ? (
              <span aria-label="unsaved changes" className="size-1.5 rounded-full bg-accent" />
            ) : tab.badge ? (
              <span className="rounded bg-surface-2 px-1 text-[10px] text-ink-3">{tab.badge}</span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}
