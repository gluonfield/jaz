import { useQuery } from '@tanstack/react-query'
import { Folder, FolderOpen, Laptop, Plus, RefreshCw, Terminal, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { Input } from '@/components/ui/Input'
import { Segmented } from '@/components/ui/Segmented'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { machinesQuery } from '@/lib/api/cloudThreads'
import type { ConnectedMachine, FolderAccessMode } from '@/lib/api/serverTypes'
import { clientRuntime } from '@/lib/clientRuntime'
import {
  buildConnectCloudCommand,
  defaultCommandShell,
  deriveGrantId,
  FOLDER_ACCESS_LABELS,
  loadLocalMachineConfig,
  saveLocalMachineConfig,
  validateGrantPath,
  type CommandShellKind,
  type LocalMachineShareConfig,
} from '@/lib/cloudMachineConfig'
import { useConnection } from '@/lib/connection'
import { localDeviceLabel } from '@/lib/deviceLabel'
import { useCopyAction } from '@/lib/useCopyAction'

const ACCESS_OPTIONS: { value: FolderAccessMode; label: string }[] = [
  { value: 'ro', label: 'Read-only' },
  { value: 'rw', label: 'Read & write' },
  { value: 'rwd', label: 'Full' },
]

function platformBadge(platform: string): string {
  switch (platform.toLowerCase()) {
    case 'darwin':
    case 'macos':
      return 'macOS'
    case 'windows':
    case 'win32':
      return 'Windows'
    case 'linux':
      return 'Linux'
    default:
      return platform || 'Desktop'
  }
}

function formatLastSeen(iso: string, online: boolean): string {
  if (online) return 'Online now'
  if (!iso) return 'Never seen'
  const ms = Date.parse(iso)
  if (Number.isNaN(ms)) return iso
  const diffSec = Math.max(0, Math.floor((Date.now() - ms) / 1000))
  if (diffSec < 60) return 'Seen just now'
  if (diffSec < 3600) return `Seen ${Math.floor(diffSec / 60)}m ago`
  if (diffSec < 86400) return `Seen ${Math.floor(diffSec / 3600)}h ago`
  return `Seen ${new Date(ms).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`
}

function accessBadgeStyle(mode: FolderAccessMode): string {
  switch (mode) {
    case 'rwd':
      return 'bg-danger-soft text-danger'
    case 'rw':
      return 'bg-primary-soft text-primary'
    default:
      return 'bg-surface-2 text-ink-2'
  }
}

const SHELL_OPTIONS: { value: CommandShellKind; label: string }[] = [
  { value: 'posix', label: 'macOS / Linux' },
  { value: 'powershell', label: 'PowerShell' },
]

export function ConnectedMachinesSettings() {
  const { url: cloudUrl } = useConnection()
  const machines = useQuery(machinesQuery)

  return (
    <section className="flex flex-col gap-5 py-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Connected Machines</h1>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Pair Mac, Windows, or Linux workstations to stage approved folders into cloud runs or opt into local execution.
          </p>
        </div>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => void machines.refetch()}
          disabled={machines.isFetching}
        >
          <RefreshCw size={13} className={machines.isFetching ? 'animate-spin' : ''} />
          Refresh
        </Button>
      </div>

      <div>
        <p className="mb-2 text-[12px] font-medium text-ink-3">Paired workstations</p>
        <SettingsCard className="overflow-hidden">
          {machines.isPending ? (
            <div className="p-3">
              <SkeletonRows count={2} />
            </div>
          ) : machines.isError ? (
            <p className="px-3.5 py-3 text-[13px] text-danger">{machines.error.message}</p>
          ) : machines.data.length === 0 ? (
            <div className="px-3.5 py-4 text-[13px] text-ink-3">
              No machines paired yet. Configure allowed folders below and run the connect command on your workstation.
            </div>
          ) : (
            machines.data.map((machine) => <MachineRow key={machine.device_id} machine={machine} />)
          )}
        </SettingsCard>
      </div>

      <LocalMachineSharingSection key={cloudUrl} cloudUrl={cloudUrl} />
    </section>
  )
}

function LocalMachineSharingSection({ cloudUrl }: { cloudUrl: string }) {
  const toast = useToast()
  const [localConfig, setLocalConfig] = useState<LocalMachineShareConfig>(() => loadLocalMachineConfig(cloudUrl))
  const [newPath, setNewPath] = useState('')
  const [newMode, setNewMode] = useState<FolderAccessMode>('ro')
  const [shell, setShell] = useState<CommandShellKind>(defaultCommandShell)

  useEffect(() => {
    saveLocalMachineConfig(cloudUrl, localConfig)
  }, [cloudUrl, localConfig])

  let connectCommand = ''
  let commandError = ''
  try {
    connectCommand = buildConnectCloudCommand({
      cloudUrl,
      deviceId: localConfig.deviceId,
      displayName: localConfig.displayName || localDeviceLabel(),
      grants: localConfig.grants,
      allowExec: localConfig.allowExec,
      shell,
    })
  } catch (error) {
    commandError = error instanceof Error ? error.message : 'Invalid machine configuration'
  }
  const { copied, copy } = useCopyAction(connectCommand)

  const addFolderGrant = (pathValue: string) => {
    const trimmed = pathValue.trim()
    const pathError = validateGrantPath(trimmed)
    if (pathError) {
      toast(pathError, 'danger')
      return
    }
    if (localConfig.grants.some((grant) => grant.root_path === trimmed)) {
      toast('Folder is already in Allowed Folders', 'danger')
      return
    }
    const id = deriveGrantId(trimmed, localConfig.grants)
    setLocalConfig((prev) => ({
      ...prev,
      grants: [...prev.grants, { id, root_path: trimmed, mode: newMode }],
    }))
    setNewPath('')
  }

  const pickNativeFolder = async () => {
    const picked = await clientRuntime.pickDirectory?.()
    if (picked) addFolderGrant(picked)
  }

  const updateGrantMode = (id: string, mode: FolderAccessMode) => {
    setLocalConfig((prev) => ({
      ...prev,
      grants: prev.grants.map((grant) => (grant.id === id ? { ...grant, mode } : grant)),
    }))
  }

  const removeGrant = (id: string) => {
    setLocalConfig((prev) => ({
      ...prev,
      grants: prev.grants.filter((grant) => grant.id !== id),
    }))
  }

  return (
    <div>
      <p className="mb-2 text-[12px] font-medium text-ink-3">This machine sharing</p>
      <SettingsCard className="overflow-hidden">
        <SettingsRow
          title="Machine name"
          description="Shown when selecting an execution target in cloud threads."
        >
          <Input
            value={localConfig.displayName}
            onChange={(event) => setLocalConfig((prev) => ({ ...prev, displayName: event.target.value }))}
            placeholder={localDeviceLabel()}
            aria-label="Machine name"
            className="h-8 w-52 text-[13px]"
          />
        </SettingsRow>

        <div className="border-t border-border px-3 py-2.5">
          <div className="flex items-center justify-between gap-2">
            <div>
              <p className="text-[13px] font-medium text-ink">Allowed Folders</p>
              <p className="mt-0.5 text-[12px] text-ink-3">
                Only folders listed here can be read, staged to GCS, or accessed by local execution.
              </p>
            </div>
            {clientRuntime.pickDirectory ? (
              <Button size="sm" variant="secondary" onClick={() => void pickNativeFolder()}>
                <FolderOpen size={13} />
                Choose folder
              </Button>
            ) : null}
          </div>

          {localConfig.grants.length > 0 ? (
            <div className="mt-2.5 flex flex-col divide-y divide-border/70 rounded-control bg-bg/60">
              {localConfig.grants.map((grant) => (
                <div key={grant.id} className="flex flex-wrap items-center gap-2 px-2.5 py-2">
                  <Folder size={13} className="shrink-0 text-ink-3" />
                  <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink" title={grant.root_path}>
                    {grant.root_path}
                  </span>
                  <select
                    aria-label={`Access mode for ${grant.root_path}`}
                    value={grant.mode}
                    onChange={(event) => updateGrantMode(grant.id, event.target.value as FolderAccessMode)}
                    className="h-7 cursor-pointer rounded-full bg-surface px-2.5 text-[11.5px] font-medium text-ink outline-none ring-1 ring-border/70 focus-visible:ring-primary"
                  >
                    <option value="ro">ro · Read-only</option>
                    <option value="rw">rw · Read &amp; write</option>
                    <option value="rwd">rwd · Read, write &amp; delete</option>
                  </select>
                  <button
                    type="button"
                    aria-label={`Remove ${grant.root_path}`}
                    onClick={() => removeGrant(grant.id)}
                    className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                  >
                    <X size={13} />
                  </button>
                </div>
              ))}
            </div>
          ) : null}

          <form
            onSubmit={(event) => {
              event.preventDefault()
              addFolderGrant(newPath)
            }}
            className="mt-2.5 flex flex-wrap items-center gap-2"
          >
            <Input
              value={newPath}
              onChange={(event) => setNewPath(event.target.value)}
              placeholder="/Users/you/Projects/repo"
              aria-label="Folder path"
              spellCheck={false}
              className="h-8 min-w-[200px] flex-1 font-mono text-[12px]"
            />
            <Segmented
              layoutId="machine-new-grant-mode"
              value={newMode}
              options={ACCESS_OPTIONS}
              onChange={setNewMode}
            />
            <Button type="submit" size="sm" variant="primary" disabled={!newPath.trim()}>
              <Plus size={13} />
              Add
            </Button>
          </form>
        </div>

        <SettingsRow
          title="Allow local execution (codex exec-server)"
          description="Opt-in. Lets cloud threads run sandboxed commands locally inside your Allowed Folders."
        >
          <Switch
            checked={localConfig.allowExec}
            onChange={(checked) => setLocalConfig((prev) => ({ ...prev, allowExec: checked }))}
            aria-label="Allow local execution"
          />
        </SettingsRow>

        <div className="border-t border-border px-3 py-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-1.5 text-[12px] font-medium text-ink">
              <Terminal size={13} className="text-ink-3" />
              Connect standalone workstation
            </div>
            <div className="flex items-center gap-2">
              <Segmented
                layoutId="machine-connect-shell"
                value={shell}
                options={SHELL_OPTIONS}
                onChange={setShell}
              />
              <Button size="sm" variant="ghost" onClick={copy} disabled={!connectCommand}>
                <CopyToggleIcon copied={copied} />
                {copied ? 'Copied' : 'Copy command'}
              </Button>
            </div>
          </div>
          <p className="mt-1 text-[11.5px] text-ink-3">
            Reads <code className="font-mono text-ink-2">JAZ_CLOUD_TOKEN</code> from the environment so credentials stay out of process arguments.
          </p>
          {commandError ? (
            <p className="mt-2 text-[12px] text-danger">{commandError}</p>
          ) : (
            <pre className="mt-2 overflow-x-auto rounded-control bg-bg px-3 py-2.5 font-mono text-[11.5px] leading-relaxed text-ink-2 select-all">
              {connectCommand}
            </pre>
          )}
        </div>
      </SettingsCard>
    </div>
  )
}

function MachineRow({ machine }: { machine: ConnectedMachine }) {
  const caps = machine.capabilities
  return (
    <div className="border-t border-border px-3.5 py-3 first:border-t-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span
            aria-label={machine.online ? 'Online' : 'Offline'}
            className={`size-2 shrink-0 rounded-full ${machine.online ? 'bg-ok' : 'bg-ink-3/40'}`}
          />
          <Laptop size={14} className="shrink-0 text-ink-2" />
          <span className="truncate text-[13.5px] font-medium text-ink">{machine.display_name || machine.device_id}</span>
          <span className="shrink-0 rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-medium text-ink-2">
            {platformBadge(machine.platform)}
          </span>
        </div>
        <span className="text-[11.5px] text-ink-3 tabular-nums">
          {formatLastSeen(machine.last_seen_at, machine.online)}
        </span>
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        {caps.file_staging ? (
          <span className="rounded-full bg-bg px-2 py-0.5 text-[11px] text-ink-2">File staging</span>
        ) : null}
        {caps.local_exec ? (
          <span className="rounded-full bg-primary-soft px-2 py-0.5 text-[11px] font-medium text-primary">Local exec</span>
        ) : null}
        {caps.local_browser ? (
          <span className="rounded-full bg-bg px-2 py-0.5 text-[11px] text-ink-2">Local browser</span>
        ) : null}
        {caps.computer_use ? (
          <span className="rounded-full bg-bg px-2 py-0.5 text-[11px] text-ink-2">Computer use</span>
        ) : null}
      </div>

      {machine.folder_grants.length > 0 ? (
        <div className="mt-2.5 flex flex-col gap-1 rounded-control bg-bg/60 px-2.5 py-2">
          {machine.folder_grants.map((grant) => (
            <div key={grant.id} className="flex items-center gap-2 text-[12px]">
              <Folder size={12} className="shrink-0 text-ink-3" />
              <span className="min-w-0 flex-1 truncate font-mono text-ink" title={grant.root_path}>
                {grant.root_path}
              </span>
              {grant.label ? <span className="shrink-0 text-[11px] text-ink-3">{grant.label}</span> : null}
              <span
                className={`shrink-0 rounded-full px-2 py-px text-[10.5px] font-medium ${accessBadgeStyle(grant.mode)}`}
                title={FOLDER_ACCESS_LABELS[grant.mode]}
              >
                {grant.mode} · {FOLDER_ACCESS_LABELS[grant.mode]}
              </span>
            </div>
          ))}
        </div>
      ) : (
        <p className="mt-2 text-[12px] text-ink-3">No folders shared on this machine.</p>
      )}
    </div>
  )
}
