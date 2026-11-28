import { useNavigate } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import type { ReactNode } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { AppsTab } from '@/lib/appsLibrary'
import { clientRuntime } from '@/lib/clientRuntime'

// The bar over an opened app: the way back to the library tab that lists it,
// what the app is called, then what can be done with it. A window of its own
// has no way back.
export function AppPageHeader({ title, tab, children }: { title: ReactNode; tab?: AppsTab; children?: ReactNode }) {
  const navigate = useNavigate()
  return (
    <header className="flex h-11 shrink-0 items-center gap-1 border-b border-border/70 bg-bg px-2">
      {clientRuntime.windowKind === 'main' ? (
        <IconButton size="sm" aria-label="Back to Apps" title="Back to Apps" onClick={() => void navigate({ to: '/apps', search: { tab } })}>
          <ArrowLeft size={15} />
        </IconButton>
      ) : null}
      <div className="min-w-0 flex-1 px-1.5 text-[13.5px] font-medium text-ink">{title}</div>
      {children}
    </header>
  )
}
