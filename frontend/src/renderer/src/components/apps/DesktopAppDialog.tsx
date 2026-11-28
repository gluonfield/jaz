import { useMutation, useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { orgAppPolicyQuery, type BundledApp } from '@/lib/api/apps'
import { desktopApps, installDesktopApp } from '@/lib/desktopApps'
import { declaredDesktopPermissions, isMachineAccess, type DesktopAppPermission, type InstalledDesktopApp } from '@shared/desktopApps'

const ASKS: Record<DesktopAppPermission, { label: string; detail: string }> = {
  notifications: { label: 'Show notifications', detail: 'Even while you use other apps.' },
  always_on_top: { label: 'Float above other windows', detail: 'Its window can stay on top of everything else.' },
  shortcuts: { label: 'Use a keyboard shortcut from any app', detail: 'A key combination brings it up wherever you are.' },
  tray: { label: 'Add an icon to the menu bar', detail: 'So you can reach it while its window is closed.' },
  fs: { label: 'Read and change files in folders you choose', detail: 'Only the folders you pick here.' },
  full_access: { label: 'Full access to this computer', detail: 'It can run programs and read, change or delete any of your files. Only allow this if you trust whoever made it.' },
}

// What an app that declares desktop capabilities asks for, in plain words: shown
// before it installs, and later from its Permissions menu item.
export function DesktopAppDialog({
  app,
  installed,
  onClose,
  onSaved,
}: {
  app: BundledApp
  installed: InstalledDesktopApp | null
  onClose: () => void
  onSaved: () => void
}) {
  const declared = declaredDesktopPermissions(app.capabilities ?? installed?.declaredCapabilities)
  const policy = useQuery({ ...orgAppPolicyQuery, enabled: declared.includes('full_access') })
  const fullAccess = policy.data?.allow_desktop_full_access ?? false
  const [allowed, setAllowed] = useState(installed?.permissions ?? declared.filter((permission) => !isMachineAccess(permission)))
  const [folders, setFolders] = useState(installed?.scopedFolders ?? [])
  const save = useMutation({
    mutationFn: () => installed
      ? desktopApps!.setPermissions(app.id, app.bundle_sha256 || installed.latestBundleSha256, allowed, folders, fullAccess)
      : installDesktopApp(app, allowed, folders, fullAccess),
    onSuccess: () => {
      onSaved()
      onClose()
    },
  })
  const toggle = (permission: DesktopAppPermission) =>
    setAllowed((current) => current.includes(permission) ? current.filter((item) => item !== permission) : [...current, permission])
  const pickFolder = async () => {
    const folder = await desktopApps!.pickScopedFolder(app.id)
    if (!folder) return
    setFolders((current) => current.includes(folder) ? current : [...current, folder])
    setAllowed((current) => current.includes('fs') ? current : [...current, 'fs'])
  }

  return (
    <Modal
      open
      onClose={onClose}
      size="md"
      title={installed ? `${app.title} permissions` : `Install ${app.title}?`}
      description={installed?.permissionsPaused
        ? 'It changed since you allowed these, so they stay off until you allow them again.'
        : installed ? 'Choose what it may do on this computer.' : 'It opens in its own window with its own icon. It asks to:'}
      footer={
        <>
          <div className="min-w-0 truncate text-[12px] text-danger">{save.error?.message}</div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button variant="ghost" size="md" disabled={save.isPending} onClick={onClose}>Cancel</Button>
            <Button variant="primary" size="md" disabled={save.isPending} onClick={() => save.mutate()}>
              {save.isPending ? 'Saving…' : installed ? 'Allow' : 'Install'}
            </Button>
          </div>
        </>
      }
    >
      <ul className="space-y-1.5 text-[13px]">
        {declared.map((permission) => {
          const blocked = permission === 'full_access' && !fullAccess
          return (
            <li key={permission} className="rounded-control border border-border bg-bg px-3 py-2.5">
              <div className="flex items-start gap-2">
                <label className={`flex min-w-0 flex-1 items-start gap-2.5 ${blocked ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'}`}>
                  <input
                    type="checkbox"
                    disabled={blocked}
                    checked={!blocked && allowed.includes(permission)}
                    onChange={() => toggle(permission)}
                    className="mt-0.5 accent-primary"
                  />
                  <span className="min-w-0">
                    <span className="block font-medium text-ink">{ASKS[permission].label}</span>
                    <span className="block text-[12px] text-ink-3">{blocked ? 'Your organization doesn’t allow this.' : ASKS[permission].detail}</span>
                  </span>
                </label>
                {permission === 'fs' ? (
                  <Button size="sm" onClick={() => void pickFolder()}>Choose folder…</Button>
                ) : null}
              </div>
              {permission === 'fs' && folders.length ? (
                <ul className="mt-2 space-y-1 pl-6">
                  {folders.map((folder) => (
                    <li key={folder} className="flex items-center gap-2 font-mono text-[11.5px] text-ink-2">
                      <span className="min-w-0 flex-1 truncate" title={folder}>{folder}</span>
                      <button
                        type="button"
                        aria-label={`Remove ${folder}`}
                        onClick={() => setFolders((current) => current.filter((item) => item !== folder))}
                        className="cursor-pointer text-ink-3 hover:text-ink"
                      >
                        <X size={12} />
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </li>
          )
        })}
      </ul>
    </Modal>
  )
}
