import { ArrowLeft } from 'lucide-react'
import { Fragment } from 'react'
import { SidebarWorkspaceSwitcher } from '@/components/sidebar/SidebarWorkspaceSwitcher'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { type SettingsSection, useExperimentalFeaturesEnabled, visibleSettingsSections } from './sections'

// The Settings menu. On desktop it takes the left column on the window colour,
// like the thread list; phones show it as a drawer inside full-screen Settings.
export function SettingsNav({
  section,
  onSectionChange,
  onClose,
}: {
  section?: SettingsSection
  onSectionChange: (section: SettingsSection) => void
  onClose: () => void
}) {
  const [experimentalEnabled] = useExperimentalFeaturesEnabled()
  const cloud = useCloudSession()
  const cloudOnly = useIsCloudOnly()
  const items = visibleSettingsSections(experimentalEnabled, cloud.active, !cloudOnly)

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

      <SidebarWorkspaceSwitcher className="shrink-0 pt-1.5 pb-0.5" />

      <nav aria-label="Settings sections" className="flex min-h-0 flex-1 flex-col gap-px overflow-y-auto pb-2.5">
        {items.map((item, index) => {
          const Icon = item.icon
          const selected = item.id === section
          return (
            <Fragment key={item.id}>
              {item.group === 'app' && items[index - 1]?.group !== 'app' ? (
                <p className="px-2.5 pt-3.5 pb-1 text-[11.5px] font-medium text-ink-3 max-sm:px-3 max-sm:text-[13px]">
                  App
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
      </nav>
    </div>
  )
}
