import { MonitorSmartphone, Server } from 'lucide-react'
import { useConnection } from '@/lib/connection'
import { connectionStatusDisplay, describeBackend } from '@/lib/connectionDisplay'

// The sidebar's connection indicator: which backend Jaz is talking to, with a
// health dot, and the entry point to switch machines after onboarding.
export function ConnectionFooterButton({ onClick }: { onClick: () => void }) {
  const { status, url } = useConnection()
  const backend = describeBackend(url)
  const { dot, label } = connectionStatusDisplay(status)
  const Icon = backend.local ? MonitorSmartphone : Server

  return (
    <button
      type="button"
      onClick={onClick}
      title={`${label} · ${backend.url}`}
      className="group flex w-full items-center gap-2 rounded-full px-2.5 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
    >
      <Icon size={15} className="shrink-0 text-ink-2" />
      <span className="min-w-0 flex-1 truncate text-left">{backend.title}</span>
      <span className={`size-1.5 shrink-0 rounded-full ${dot}`} />
    </button>
  )
}
