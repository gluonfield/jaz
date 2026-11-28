import { Folder, Plus, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { IconButton } from '@/components/ui/IconButton'
import { Input } from '@/components/ui/Input'
import { Segmented } from '@/components/ui/Segmented'
import { Select } from '@/components/ui/Select'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import type { FolderAccessMode, MachineFolderGrant } from '@/lib/api/serverTypes'
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
import { useCopyAction } from '@/lib/useCopyAction'

const ACCESS_OPTIONS = Object.entries(FOLDER_ACCESS_LABELS).map(([value, label]) => ({ value, label }))

const SHELL_OPTIONS: { value: CommandShellKind; label: string }[] = [
  { value: 'posix', label: 'macOS / Linux' },
  { value: 'powershell', label: 'PowerShell' },
]

export function FoldersEditor({
  grants,
  disabled = false,
  pickDirectory,
  onChange,
}: {
  grants: MachineFolderGrant[]
  disabled?: boolean
  pickDirectory?: () => Promise<string | null>
  onChange: (grants: MachineFolderGrant[], change: string) => void
}) {
  const toast = useToast()
  const [path, setPath] = useState('')

  const add = (value: string) => {
    const rootPath = value.trim()
    const pathError = validateGrantPath(rootPath)
    if (pathError) {
      toast(pathError, 'danger')
      return false
    }
    if (grants.some((grant) => grant.root_path === rootPath)) {
      toast('That folder is already added', 'danger')
      return false
    }
    onChange([...grants, { id: deriveGrantId(rootPath, grants), root_path: rootPath, mode: 'ro' }], 'Folder added')
    return true
  }

  return (
    <div className="border-t border-border px-3 py-2.5">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[13px] font-medium text-ink">Folders</p>
          <p className="mt-0.5 text-[12px] text-ink-3">Cloud threads can only open the folders you add here.</p>
        </div>
        {pickDirectory ? (
          <Button
            size="sm"
            disabled={disabled}
            onClick={() =>
              void pickDirectory().then((picked) => {
                if (picked) {
                  add(picked)
                }
              })
            }
          >
            <Plus size={13} />
            Add folder
          </Button>
        ) : null}
      </div>

      {grants.length > 0 ? (
        <ul className="mt-1.5">
          {grants.map((grant) => (
            <li key={grant.id} className="grid h-9 grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-2">
              <Folder size={14} className="text-ink-3" />
              <span className="truncate text-[13px] text-ink" title={grant.root_path}>
                {grant.root_path}
              </span>
              <Select
                variant="plain"
                value={grant.mode}
                options={ACCESS_OPTIONS}
                disabled={disabled}
                onChange={(mode) =>
                  onChange(
                    grants.map((item) => (item.id === grant.id ? { ...item, mode: mode as FolderAccessMode } : item)),
                    'Folder access updated',
                  )
                }
                aria-label={`Access to ${grant.root_path}`}
              />
              <IconButton
                size="xs"
                aria-label={`Remove ${grant.root_path}`}
                disabled={disabled}
                onClick={() =>
                  onChange(
                    grants.filter((item) => item.id !== grant.id),
                    'Folder removed',
                  )
                }
              >
                <X size={13} />
              </IconButton>
            </li>
          ))}
        </ul>
      ) : null}

      {pickDirectory ? null : (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (add(path)) {
              setPath('')
            }
          }}
          className="mt-2 flex items-center gap-2"
        >
          <Input
            value={path}
            disabled={disabled}
            onChange={(event) => setPath(event.target.value)}
            placeholder="Folder path"
            aria-label="Folder path"
            spellCheck={false}
            className="h-8 min-w-0 flex-1 text-[13px]"
          />
          <Button type="submit" size="md" disabled={disabled || !path.trim()}>
            Add
          </Button>
        </form>
      )}
    </div>
  )
}

export function RunCommandsRow({
  machine,
  checked,
  disabled,
  onChange,
}: {
  machine: 'this machine' | 'that machine'
  checked: boolean
  disabled?: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <SettingsRow
      title="Run commands"
      description={`Cloud threads can run sandboxed commands on ${machine}, inside the folders above.`}
    >
      <Switch checked={checked} disabled={disabled} onChange={onChange} aria-label="Run commands" />
    </SettingsRow>
  )
}

export function AddMachineDisclosure({
  cloudUrl,
  label,
  defaultOpen,
}: {
  cloudUrl: string
  label: string
  defaultOpen: boolean
}) {
  const [toggled, setToggled] = useState<boolean | null>(null)
  const open = toggled ?? defaultOpen

  return (
    <div>
      <DisclosureTrigger label={label} open={open} onClick={() => setToggled(!open)} />
      {open ? <AddMachineCard cloudUrl={cloudUrl} /> : null}
    </div>
  )
}

function AddMachineCard({ cloudUrl }: { cloudUrl: string }) {
  const [config, setConfig] = useState<LocalMachineShareConfig>(() => loadLocalMachineConfig(cloudUrl))
  const [shell, setShell] = useState<CommandShellKind>(defaultCommandShell)

  useEffect(() => {
    saveLocalMachineConfig(cloudUrl, config)
  }, [cloudUrl, config])

  let command = ''
  let commandError = ''
  try {
    command = buildConnectCloudCommand({
      cloudUrl,
      deviceId: config.deviceId,
      displayName: config.displayName,
      grants: config.grants,
      allowExec: config.allowExec,
      shell,
    })
  } catch (error) {
    commandError = error instanceof Error ? error.message : 'Invalid machine configuration'
  }
  const { copied, copy } = useCopyAction(command)

  return (
    <SettingsCard className="mt-1 overflow-hidden">
      <SettingsRow title="Name">
        <Input
          value={config.displayName}
          onChange={(event) => setConfig((prev) => ({ ...prev, displayName: event.target.value }))}
          placeholder="Uses its hostname"
          aria-label="Machine name"
          className="h-8 min-w-72 text-[13px]"
        />
      </SettingsRow>

      <FoldersEditor
        grants={config.grants}
        onChange={(grants) => setConfig((prev) => ({ ...prev, grants }))}
      />

      <RunCommandsRow
        machine="that machine"
        checked={config.allowExec}
        onChange={(allowExec) => setConfig((prev) => ({ ...prev, allowExec }))}
      />

      <div className="border-t border-border px-3 py-2.5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-ink">Connect</p>
            <p className="mt-0.5 text-[12px] text-ink-3">
              Run this on that machine, with <code className="font-mono text-ink-2">JAZ_CLOUD_TOKEN</code> set.
            </p>
          </div>
          <div className="flex items-center gap-1">
            <Segmented layoutId="machine-connect-shell" value={shell} options={SHELL_OPTIONS} onChange={setShell} />
            <Button size="sm" onClick={copy} disabled={!command}>
              <CopyToggleIcon copied={copied} />
              {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
        </div>
        {commandError ? (
          <p className="mt-2 text-[12px] text-danger">{commandError}</p>
        ) : (
          <pre className="mt-2 overflow-x-auto rounded-control bg-bg px-3 py-2.5 font-mono text-[11.5px] leading-relaxed text-ink-2 select-all">
            {command}
          </pre>
        )}
      </div>
    </SettingsCard>
  )
}
