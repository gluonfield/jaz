import { LayoutList, PanelRight } from 'lucide-react'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import type { SidePanelMode } from '@/lib/sidePanelTabs'

export function SidePanelControl({ open, mode, modes = ['tabs', 'overview'], overviewLabel = 'Overview', onToggle }: {
  open: boolean
  mode: SidePanelMode
  modes?: SidePanelMode[]
  overviewLabel?: string
  onToggle: (mode: SidePanelMode) => void
}) {
  const isMobile = useIsMobile()
  const metaHeld = useMetaHeld(!isMobile)
  return (
    <div className="flex shrink-0 items-center gap-0.5">
      {modes.map((option) => {
        const active = open && mode === option
        const label = option === 'tabs' ? 'Side Panel' : overviewLabel
        const Icon = option === 'tabs' ? PanelRight : LayoutList
        return (
          <button
            key={option}
            type="button"
            aria-label={label}
            aria-pressed={active}
            aria-keyshortcuts={option === 'tabs' ? 'Meta+Shift+S' : 'Meta+O'}
            title={`${active ? 'Hide' : 'Open'} ${label}`}
            onClick={() => onToggle(option)}
            className={`flex h-7 cursor-pointer items-center justify-center rounded-lg px-[7px] text-xs pointer-coarse:h-10 pointer-coarse:min-w-10 font-medium whitespace-nowrap transition-[background-color,color,transform] duration-150 active:scale-[0.96] ${active ? 'bg-surface text-ink' : 'text-ink-2 hover:bg-surface hover:text-ink'}`}
          >
            <Icon size={14} aria-hidden className="shrink-0" />
            <span aria-hidden={!metaHeld} className={`inline-flex max-w-0 overflow-hidden transition-[max-width,margin-left,opacity] duration-200 ease-out motion-reduce:transition-none ${metaHeld ? 'ml-1 max-w-12 opacity-100' : 'opacity-0'}`}>
              <KeyboardShortcut value={option === 'tabs' ? '⇧S' : 'O'} />
            </span>
          </button>
        )
      })}
    </div>
  )
}
