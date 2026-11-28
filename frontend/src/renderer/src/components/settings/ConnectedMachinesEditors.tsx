import { ChevronRight, Folder, FolderOpen, Plus, Terminal, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { Input } from '@/components/ui/Input'
import { Segmented } from '@/components/ui/Segmented'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import type { FolderAccessMode, MachineFolderGrant } from '@/lib/api/serverTypes'
import { clientRuntime } from '@/lib/clientRuntime'
import {
  buildConnectCloudCommand,
  defaultCommandShell,
  deriveGrantId,
  loadLocalMachineConfig,
  saveLocalMachineConfig,
  validateGrantPath,
  type CommandShellKind,
  type LocalMachineShareConfig,
} from '@/lib/cloudMachineConfig'
import { localDeviceLabel } from '@/lib/deviceLabel'
import { useCopyAction } from '@/lib/useCopyAction'

const ACCESS_OPTIONS: { value: FolderAccessMode; label: string }[] = [
  { value: 'ro', label: 'Read-only' },
  { value: 'rw', label: 'Read & write' },
  { value: 'rwd', label: 'Full' },
]

const SHELL_OPTIONS: { value: CommandShellKind; label: string }[] = [
  { value: 'posix', label: 'macOS / Linux' },
  { value: 'powershell', label: 'PowerShell' },
]

export function AllowedFoldersEditor({
  grants,
  disabled = false,
  layoutId,
  pathAriaLabel,
  description,
  emptyMessage,
  onAdd,
  onChangeMode,
  onRemove,
}: {
  grants: MachineFolderGrant[]
  disabled?: boolean
  layoutId: string
  pathAriaLabel: string
  description: string
  emptyMessage?: string
  onAdd: (grant: MachineFolderGrant, onAccepted?: () => void) => void
  onChangeMode: (id: string, mode: FolderAccessMode) => void
  onRemove: (id: string) => void
}) {
  const toast = useToast()
  const [newPath, setNewPath] = useState('')
  const [newMode, setNewMode] = useState<FolderAccessMode>('ro')

  const addGrant = (pathValue: string, clearOnAccept: boolean) => {
    const trimmed = pathValue.trim()
    const pathError = validateGrantPath(trimmed)
    if (pathError) {
      toast(pathError, 'danger')
      return
    }
    if (grants.some((grant) => grant.root_path === trimmed)) {
      toast('Folder is already in Allowed Folders', 'danger')
      return
    }
    const id = deriveGrantId(trimmed, grants)
    onAdd(
      { id, root_path: trimmed, mode: newMode },
      clearOnAccept ? () => setNewPath('') : undefined,
    )
  }

  const pickNativeFolder = async () => {
    const picked = await clientRuntime.pickDirectory?.()
    if (picked) {
      addGrant(picked, false)
    }
  }

  return (
    <div className="border-t border-border px-3 py-2.5">
      <div className="flex items-center justify-between gap-2">
        <div>
          <p className="text-[13px] font-medium text-ink">Allowed Folders</p>
          <p className="mt-0.5 text-[12px] text-ink-3">{description}</p>
        </div>
        {clientRuntime.pickDirectory ? (
          <Button
            size="sm"
            variant="secondary"
            disabled={disabled}
            onClick={() => void pickNativeFolder()}
          >
            <FolderOpen size={13} />
            Choose folder
          </Button>
        ) : null}
      </div>

      {grants.length > 0 ? (
        <div className="mt-2.5 flex flex-col divide-y divide-border/70 rounded-control bg-bg/60">
          {grants.map((grant) => (
            <div key={grant.id} className="flex flex-wrap items-center gap-2 px-2.5 py-2">
              <Folder size={13} className="shrink-0 text-ink-3" />
              <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink" title={grant.root_path}>
                {grant.root_path}
              </span>
              <select
                aria-label={`Access mode for ${grant.root_path}`}
                value={grant.mode}
                disabled={disabled}
                onChange={(event) => onChangeMode(grant.id, event.target.value as FolderAccessMode)}
                className="h-7 cursor-pointer rounded-full bg-surface px-2.5 text-[11.5px] font-medium text-ink outline-none ring-1 ring-border/70 focus-visible:ring-primary disabled:opacity-50"
              >
                <option value="ro">ro · Read-only</option>
                <option value="rw">rw · Read &amp; write</option>
                <option value="rwd">rwd · Read, write &amp; delete</option>
              </select>
              <button
                type="button"
                aria-label={`Remove ${grant.root_path}`}
                disabled={disabled}
                onClick={() => onRemove(grant.id)}
                className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:opacity-50"
              >
                <X size={13} />
              </button>
            </div>
          ))}
        </div>
      ) : emptyMessage ? (
        <p className="mt-2 text-[12px] text-ink-3">{emptyMessage}</p>
      ) : null}

      <form
        onSubmit={(event) => {
          event.preventDefault()
          addGrant(newPath, true)
        }}
        className="mt-2.5 flex flex-wrap items-center gap-2"
      >
        <Input
          value={newPath}
          disabled={disabled}
          onChange={(event) => setNewPath(event.target.value)}
          placeholder="/Users/you/Projects/repo"
          aria-label={pathAriaLabel}
          spellCheck={false}
          className="h-8 min-w-[200px] flex-1 font-mono text-[12px]"
        />
        <Segmented
          layoutId={layoutId}
          value={newMode}
          options={ACCESS_OPTIONS}
          onChange={setNewMode}
        />
        <Button type="submit" size="sm" variant="primary" disabled={disabled || !newPath.trim()}>
          <Plus size={13} />
          Add
        </Button>
      </form>
    </div>
  )
}

export function ConnectAnotherWorkstationDisclosure({ cloudUrl }: { cloudUrl: string }) {
  const [open, setOpen] = useState(false)

  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((prev) => !prev)}
        className="flex cursor-pointer items-center gap-1.5 text-[12px] font-medium text-ink-3 transition-colors duration-150 hover:text-ink"
      >
        <ChevronRight
          size={13}
          className={`transition-transform duration-150 ${open ? 'rotate-90' : ''}`}
        />
        <Terminal size={12} />
        Connect another workstation (CLI)
      </button>
      {open ? (
        <div className="mt-2">
          <StandaloneWorkstationCard cloudUrl={cloudUrl} />
        </div>
      ) : null}
    </div>
  )
}

export function StandaloneWorkstationSection({ cloudUrl }: { cloudUrl: string }) {
  return (
    <div>
      <p className="mb-2 text-[12px] font-medium text-ink-3">Pair a workstation (CLI)</p>
      <StandaloneWorkstationCard cloudUrl={cloudUrl} />
    </div>
  )
}

function StandaloneWorkstationCard({ cloudUrl }: { cloudUrl: string }) {
  const [localConfig, setLocalConfig] = useState<LocalMachineShareConfig>(() => loadLocalMachineConfig(cloudUrl))
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

  return (
    <SettingsCard className="overflow-hidden">
      <SettingsRow
        title="Machine name"
        description="Shown when selecting an execution target in cloud threads."
      >
        <Input
          value={localConfig.displayName}
          onChange={(event) => setLocalConfig((prev) => ({ ...prev, displayName: event.target.value }))}
          placeholder={localDeviceLabel()}
          aria-label="Standalone machine name"
          className="h-8 w-52 text-[13px]"
        />
      </SettingsRow>

      <AllowedFoldersEditor
        grants={localConfig.grants}
        layoutId="cli-new-grant-mode"
        pathAriaLabel="Standalone folder path"
        description="Only folders listed here can be read, staged to GCS, or accessed by local execution."
        onAdd={(grant, onAccepted) => {
          setLocalConfig((prev) => ({ ...prev, grants: [...prev.grants, grant] }))
          onAccepted?.()
        }}
        onChangeMode={(id, mode) =>
          setLocalConfig((prev) => ({
            ...prev,
            grants: prev.grants.map((grant) => (grant.id === id ? { ...grant, mode } : grant)),
          }))
        }
        onRemove={(id) =>
          setLocalConfig((prev) => ({
            ...prev,
            grants: prev.grants.filter((grant) => grant.id !== id),
          }))
        }
      />

      <SettingsRow
        title="Allow local execution (codex exec-server)"
        description="Opt-in. Lets cloud threads run sandboxed commands locally inside your Allowed Folders."
      >
        <Switch
          checked={localConfig.allowExec}
          onChange={(checked) => setLocalConfig((prev) => ({ ...prev, allowExec: checked }))}
          aria-label="Allow standalone local execution"
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
  )
}
