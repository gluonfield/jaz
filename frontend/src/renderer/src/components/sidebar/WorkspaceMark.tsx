import { Laptop } from 'lucide-react'
import { OrgIconMark } from '@/components/cloud/OrgIconMark'
import type { CloudWorkspaceSummary } from '@/lib/cloudAccount'
import { workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'

// A Cloud workspace shows its org icon, or its initial; Local shows the device.
// `className` sets the size, corner radius and initial's font size.
export function WorkspaceMark({
  workspace,
  className = 'size-6 rounded-[7px] text-[12px]',
}: {
  workspace: CloudWorkspaceSummary | null
  className?: string
}) {
  if (workspace) {
    return <OrgIconMark name={workspaceDisplayName(workspace)} icon={workspace.org_icon} className={className} />
  }
  return (
    <span className={`grid shrink-0 place-items-center bg-ink text-bg ${className}`}>
      <Laptop className="size-[55%]" />
    </span>
  )
}
