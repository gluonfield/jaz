import { ArrowLeft } from 'lucide-react'
import { Fragment, useState } from 'react'
import { BackendSwitcher } from '@/components/connection/BackendSwitcher'
import { SearchField } from '@/components/ui/SearchField'
import { type SettingsSection, useExperimentalFeaturesEnabled, visibleSettingsSections } from './sections'

// The Settings menu. On desktop it takes the left column on the window colour,
// like the thread list; phones show it as a drawer inside full-screen Settings.
export function SettingsNav({
  section,
  onSectionChange,
  onClose,
  onOpenConnect,
}: {
  section?: SettingsSection
  onSectionChange: (section: SettingsSection) => void
  onClose: () => void
  onOpenConnect: () => void
}) {
  const [experimentalEnabled] = useExperimentalFeaturesEnabled()
  const [query, setQuery] = useState('')
  const q = query.trim().toLowerCase()
  const items = visibleSettingsSections(experimentalEnabled).filter((item) => !q || item.label.toLowerCase().includes(q))

  return (
    <div className="flex min-h-0 flex-1 flex-col pt-1.5 pr-3 pl-1.5 max-sm:px-3 max-sm:pt-0">
      <button
        type="button"
        onClick={onClose}
        className="mb-1.5 flex items-center gap-2 rounded-lg px-2.5 py-1 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-list-hover hover:text-ink max-sm:gap-2.5 max-sm:px-3 max-sm:py-2.5 max-sm:text-[15px]"
      >
        <ArrowLeft size={15} className="text-ink-3 max-sm:size-[18px]" />
        <span className="flex-1">Back to jaz</span>
      </button>

      {/* The backend these settings apply to — switch above everything. */}
      <div className="pb-2.5">
        <BackendSwitcher
          onConnectServer={() => {
            onClose()
            onOpenConnect()
          }}
        />
      </div>

      <div className="pb-2.5">
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder="Search settings…"
          className="h-8 max-sm:h-11 max-sm:pl-9 max-sm:text-[15px]"
        />
      </div>

      <nav aria-label="Settings sections" className="flex min-h-0 flex-1 flex-col gap-px overflow-y-auto pb-2.5">
        {items.map((item, index) => {
          const Icon = item.icon
          const selected = item.id === section
          const heading = item.group !== items[index - 1]?.group ? item.group : undefined
          return (
            <Fragment key={item.id}>
              {heading ? (
                <p className="px-2.5 pb-1 pt-3 text-[11.5px] font-medium text-ink-3 max-sm:px-3 max-sm:text-[13px]">
                  {heading}
                </p>
              ) : null}
              <button
                type="button"
                aria-current={selected ? 'page' : undefined}
                onClick={() => onSectionChange(item.id)}
                className={`flex items-center gap-2 rounded-lg px-2.5 py-1 text-left text-[13px] transition-colors duration-150 max-sm:gap-2.5 max-sm:px-3 max-sm:py-2.5 max-sm:text-[15px] ${
                  selected ? 'bg-list-active font-medium text-ink' : 'text-ink-2 hover:bg-list-hover hover:text-ink'
                }`}
              >
                <Icon size={15} className={`max-sm:size-[18px] ${selected ? 'text-ink' : 'text-ink-3'}`} />
                <span className="flex-1">{item.label}</span>
              </button>
            </Fragment>
          )
        })}
        {items.length === 0 ? <p className="px-2 text-[13px] text-ink-3">No matching settings.</p> : null}
      </nav>
    </div>
  )
}
