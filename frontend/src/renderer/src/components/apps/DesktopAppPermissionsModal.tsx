import { AlertTriangle, FolderOpen, Shield, ShieldAlert, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { JazApp, OrgAppPolicy } from '@/lib/api/apps'
import { clientRuntime } from '@/lib/clientRuntime'
import type { DesktopAppPermission, InstalledDesktopApp } from '@shared/desktopApps'

const TIER2_OPTIONS: { id: DesktopAppPermission; label: string; desc: string }[] = [
  { id: 'notifications', label: 'Native notifications', desc: 'Send desktop notifications from this app.' },
  { id: 'always_on_top', label: 'Always on top', desc: 'Float this app window above other windows.' },
  { id: 'shortcuts', label: 'Global shortcut', desc: 'Bind a system-wide keyboard shortcut to summon this app.' },
  { id: 'tray', label: 'Menu bar / tray item', desc: 'Pin a quick-access menu bar icon for this app.' },
]

export function DesktopAppPermissionsModal({
  open,
  onClose,
  app,
  installed,
  orgPolicy,
  onSaved,
}: {
  open: boolean
  onClose: () => void
  app: JazApp
  installed: InstalledDesktopApp | null
  orgPolicy?: OrgAppPolicy
  onSaved?: (updated: InstalledDesktopApp) => void
}) {
  const desktopApps = clientRuntime.desktopApps
  const [selected, setSelected] = useState<DesktopAppPermission[]>(installed?.permissions ?? [])
  const [folders, setFolders] = useState<string[]>(installed?.scopedFolders ?? [])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setSelected(installed?.permissions ?? [])
    setFolders(installed?.scopedFolders ?? [])
    setError('')
  }, [open, installed])

  const currentSha = app.bundle_sha256 || installed?.latestBundleSha256 || ''
  const isExternal = Boolean(installed?.isExternalOrg)
  const fullAccessPolicyAllowed = orgPolicy?.allow_desktop_full_access ?? false
  const tier3BlockedExternal = isExternal
  const fullAccessDisabled = tier3BlockedExternal || !fullAccessPolicyAllowed

  const toggle = (perm: DesktopAppPermission) => {
    setSelected((cur) => (cur.includes(perm) ? cur.filter((p) => p !== perm) : [...cur, perm]))
  }

  const addFolder = async () => {
    if (!desktopApps) return
    const picked = await desktopApps.pickScopedFolder(app.id)
    if (picked && !folders.includes(picked)) {
      setFolders((cur) => [...cur, picked])
      if (!selected.includes('fs')) setSelected((cur) => [...cur, 'fs'])
    }
  }

  const handleSave = async () => {
    if (!desktopApps) return
    setSaving(true)
    setError('')
    try {
      const next = installed
        ? await desktopApps.setPermissions(
            app.id,
            currentSha,
            selected,
            folders,
            fullAccessPolicyAllowed,
          )
        : await desktopApps.install({
            appId: app.id,
            workspaceId: app.org_id || 'local',
            title: app.title,
            description: app.description,
            kind: app.kind,
            icon: app.icon,
            version: app.current_version,
            bundleSha256: currentSha,
            declaredCapabilities: app.capabilities ?? [],
            permissions: selected,
            scopedFolders: folders,
            isExternalOrg: false,
            allowDesktopFullAccess: fullAccessPolicyAllowed,
          })
      onSaved?.(next)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save Desktop App permissions')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="md"
      title={installed ? `Permissions · ${app.title}` : `Install "${app.title}" as Desktop App`}
      description="Elevated permissions are locked to this exact bundle digest. Any future app update pauses Tier 2 and Tier 3 grants until you re-approve."
      footer={
        <>
          <div className="min-w-0 truncate text-[12px] text-danger">{error}</div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button variant="ghost" size="md" disabled={saving} onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" size="md" disabled={saving || !desktopApps} onClick={() => void handleSave()}>
              {saving ? 'Saving…' : installed ? 'Approve for this version' : 'Install Desktop App'}
            </Button>
          </div>
        </>
      }
    >
      <div className="space-y-4 text-[13px]">
        {installed?.permissionsPaused ? (
          <div className="flex items-start gap-2.5 rounded-control border border-amber-500/40 bg-amber-500/10 p-3 text-[12.5px] text-ink">
            <AlertTriangle size={15} className="mt-0.5 shrink-0 text-amber-500" aria-hidden />
            <div>
              <p className="font-medium">Permissions paused after update</p>
              <p className="mt-0.5 text-ink-2">
                This app updated to v{app.current_version} ({currentSha.slice(0, 12)}…). Review and re-approve permissions to re-enable them.
              </p>
            </div>
          </div>
        ) : null}

        <div className="rounded-control border border-border bg-surface px-3 py-2 text-[12px] text-ink-2">
          <div className="flex items-center justify-between gap-2">
            <span className="font-medium text-ink">Version lock</span>
            <span className="font-mono text-[11px] text-ink-3">
              v{app.current_version} · {currentSha ? currentSha.slice(0, 16) : 'unhashed'}
            </span>
          </div>
        </div>

        <section className="space-y-2">
          <h3 className="flex items-center gap-1.5 text-[12px] font-semibold uppercase tracking-wide text-ink-3">
            <Shield size={13} aria-hidden />
            Tier 2 · Desktop App Permissions
          </h3>
          <div className="space-y-1.5">
            {TIER2_OPTIONS.map((opt) => {
              const checked = selected.includes(opt.id)
              return (
                <label
                  key={opt.id}
                  className="flex cursor-pointer items-start gap-2.5 rounded-control border border-border bg-bg px-3 py-2 hover:bg-surface"
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggle(opt.id)}
                    className="mt-1 accent-primary"
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium text-ink">{opt.label}</span>
                    <span className="block text-[11.5px] text-ink-3">{opt.desc}</span>
                  </span>
                </label>
              )
            })}
          </div>
        </section>

        <section className="space-y-2">
          <h3 className="flex items-center gap-1.5 text-[12px] font-semibold uppercase tracking-wide text-ink-3">
            <ShieldAlert size={13} aria-hidden />
            Tier 3 · Local Machine Access
          </h3>
          {tier3BlockedExternal ? (
            <p className="rounded-control border border-border bg-surface px-3 py-2 text-[12px] text-ink-3">
              Tier 3 local filesystem and machine access is hard-blocked for external apps.
            </p>
          ) : (
            <div className="space-y-2">
              <div className="rounded-control border border-border bg-bg p-3">
                <div className="flex items-center justify-between gap-2">
                  <label className="flex cursor-pointer items-center gap-2">
                    <input
                      type="checkbox"
                      checked={selected.includes('fs')}
                      onChange={() => toggle('fs')}
                      className="accent-primary"
                    />
                    <span className="font-medium text-ink">Scoped folder access</span>
                  </label>
                  <Button variant="secondary" size="sm" onClick={() => void addFolder()}>
                    <FolderOpen size={12} />
                    Pick folder
                  </Button>
                </div>
                {folders.length > 0 ? (
                  <ul className="mt-2 space-y-1">
                    {folders.map((folder) => (
                      <li key={folder} className="flex items-center justify-between gap-2 rounded bg-surface px-2 py-1 font-mono text-[11px] text-ink-2">
                        <span className="truncate">{folder}</span>
                        <button
                          type="button"
                          onClick={() => setFolders((cur) => cur.filter((f) => f !== folder))}
                          className="text-ink-3 hover:text-danger"
                          aria-label={`Remove ${folder}`}
                        >
                          <Trash2 size={12} />
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="mt-1.5 text-[11.5px] text-ink-3">
                    Only explicitly picked folders can be read or written by this app.
                  </p>
                )}
              </div>

              <label
                className={`flex items-start gap-2.5 rounded-control border border-border bg-bg px-3 py-2 ${
                  fullAccessDisabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer hover:bg-surface'
                }`}
              >
                <input
                  type="checkbox"
                  disabled={fullAccessDisabled}
                  checked={!fullAccessDisabled && selected.includes('full_access')}
                  onChange={() => toggle('full_access')}
                  className="mt-1 accent-primary"
                />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium text-ink">Local machine access (org-policy gated)</span>
                  <span className="block text-[11.5px] text-ink-3">
                    {fullAccessPolicyAllowed
                      ? 'Allow this self/org-authored app to request local machine bridge operations.'
                      : 'Disabled by your organization app policy. Ask an org admin to enable allow_desktop_full_access.'}
                  </span>
                </span>
              </label>
            </div>
          )}
        </section>
      </div>
    </Modal>
  )
}
