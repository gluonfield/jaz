import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Folder, FolderOpen, Laptop, Plus, RefreshCw, Terminal, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { Input } from '@/components/ui/Input'
import { Segmented } from '@/components/ui/Segmented'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { machinesQuery } from '@/lib/api/cloudThreads'
import {
  machineBridgeQuery,
  machineBridgeQueryKey,
  updateMachineBridge,
  type MachineBridgeShareConfig,
  type MachineBridgeStatus,
} from '@/lib/api/machineBridge'
import { ApiError } from '@/lib/api/response'
import type { ConnectedMachine, FolderAccessMode, MachineFolderGrant } from '@/lib/api/serverTypes'
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
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { localDeviceLabel } from '@/lib/deviceLabel'
import { keys } from '@/lib/query/keys'
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

const RECONNECT_REFRESH_MS = 1_200

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
  if (online) {
    return 'Online now'
  }
  if (!iso) {
    return 'Never seen'
  }
  const ms = Date.parse(iso)
  if (Number.isNaN(ms)) {
    return iso
  }
  const diffSec = Math.max(0, Math.floor((Date.now() - ms) / 1000))
  if (diffSec < 60) {
    return 'Seen just now'
  }
  if (diffSec < 3600) {
    return `Seen ${Math.floor(diffSec / 60)}m ago`
  }
  if (diffSec < 86400) {
    return `Seen ${Math.floor(diffSec / 3600)}h ago`
  }
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

function bridgeStatusBadge(
  status: MachineBridgeStatus,
  cloudActive: boolean,
): { label: string; dotClass: string; pillClass: string } {
  if (!status.config.enabled) {
    return {
      label: 'Paused',
      dotClass: 'bg-ink-3/40',
      pillClass: 'bg-surface-2 text-ink-2',
    }
  }
  if (!cloudActive) {
    return {
      label: 'Waiting for Cloud',
      dotClass: 'bg-ink-3/40',
      pillClass: 'bg-surface-2 text-ink-2',
    }
  }
  if (status.connected) {
    return {
      label: 'Connected',
      dotClass: 'bg-ok',
      pillClass: 'bg-surface-2 text-ink',
    }
  }
  return {
    label: 'Connecting',
    dotClass: 'bg-primary animate-pulse',
    pillClass: 'bg-primary-soft text-primary',
  }
}

function isBridgeUnavailableError(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 404 || error.status === 501)
}

export function ConnectedMachinesSettings() {
  const { url: cloudUrl, active: cloudActive } = useCloudSession()
  const isCloudOnly = useIsCloudOnly()
  const canManageLocalBridge = clientRuntime.capabilities.localBackend && !isCloudOnly
  const machines = useQuery({ ...machinesQuery, enabled: cloudActive })
  const bridge = useQuery({ ...machineBridgeQuery, enabled: canManageLocalBridge })
  const bridgeUnavailable = bridge.isError && isBridgeUnavailableError(bridge.error)
  const showLiveBridge = canManageLocalBridge && !bridgeUnavailable
  const localDeviceId = showLiveBridge ? (bridge.data?.device_id ?? '') : ''

  const refreshAll = () => {
    if (cloudActive) {
      void machines.refetch()
    }
    if (showLiveBridge) {
      void bridge.refetch()
    }
  }

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
          onClick={refreshAll}
          disabled={machines.isFetching || (showLiveBridge && bridge.isFetching)}
        >
          <RefreshCw
            size={13}
            className={machines.isFetching || (showLiveBridge && bridge.isFetching) ? 'animate-spin' : ''}
          />
          Refresh
        </Button>
      </div>

      <div>
        <p className="mb-2 text-[12px] font-medium text-ink-3">Paired workstations</p>
        <SettingsCard className="overflow-hidden">
          {!cloudActive ? (
            <div className="px-3.5 py-4 text-[13px] text-ink-3">
              Connect to a Cloud workspace to view paired workstations.
            </div>
          ) : machines.isPending ? (
            <div className="p-3">
              <SkeletonRows count={2} />
            </div>
          ) : machines.isError ? (
            <p className="px-3.5 py-3 text-[13px] text-danger">{machines.error.message}</p>
          ) : machines.data.length === 0 ? (
            <div className="px-3.5 py-4 text-[13px] text-ink-3">
              {showLiveBridge
                ? 'No machines paired yet. Enable sharing for this machine below or connect another workstation via CLI.'
                : 'No machines paired yet. Configure allowed folders below and run the connect command on your workstation.'}
            </div>
          ) : (
            machines.data.map((machine) => (
              <MachineRow
                key={machine.device_id}
                machine={machine}
                isThisMachine={Boolean(localDeviceId && machine.device_id === localDeviceId)}
              />
            ))
          )}
        </SettingsCard>
      </div>

      {showLiveBridge ? (
        <>
          <LiveLocalBridgeSection
            status={bridge.data}
            isPending={bridge.isPending}
            error={bridge.isError ? bridge.error : null}
            cloudActive={cloudActive}
          />
          <ConnectAnotherWorkstationDisclosure key={cloudUrl} cloudUrl={cloudUrl} />
        </>
      ) : (
        <StandaloneWorkstationSection key={cloudUrl} cloudUrl={cloudUrl} />
      )}
    </section>
  )
}

function AllowedFoldersEditor({
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

function LiveLocalBridgeSection({
  status,
  isPending,
  error,
  cloudActive,
}: {
  status: MachineBridgeStatus | undefined
  isPending: boolean
  error: Error | null
  cloudActive: boolean
}) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const [nameDraft, setNameDraft] = useState<string | null>(null)
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const requestSeqRef = useRef(0)

  useEffect(() => () => {
    if (refreshTimerRef.current) {
      clearTimeout(refreshTimerRef.current)
    }
  }, [])

  const mutation = useMutation({
    mutationFn: (vars: {
      seq: number
      config: MachineBridgeShareConfig
      successMessage?: string
      onAccepted?: () => void
    }) => updateMachineBridge(vars.config),
    onSuccess: (updated, vars) => {
      if (vars.seq === requestSeqRef.current) {
        queryClient.setQueryData(machineBridgeQueryKey, updated)
        setNameDraft(null)
      }
      vars.onAccepted?.()
      if (vars.successMessage) {
        toast(vars.successMessage)
      }
      void queryClient.invalidateQueries({ queryKey: keys.cloudMachines })
      if (refreshTimerRef.current) {
        clearTimeout(refreshTimerRef.current)
      }
      refreshTimerRef.current = setTimeout(() => {
        void queryClient.invalidateQueries({ queryKey: keys.cloudMachines })
        void queryClient.invalidateQueries({ queryKey: machineBridgeQueryKey })
      }, RECONNECT_REFRESH_MS)
    },
    onError: (caught) => {
      toast(caught instanceof Error ? caught.message : 'Failed to update machine sharing', 'danger')
    },
  })

  if (isPending) {
    return (
      <div>
        <p className="mb-2 text-[12px] font-medium text-ink-3">This machine</p>
        <SettingsCard className="p-3">
          <SkeletonRows count={3} />
        </SettingsCard>
      </div>
    )
  }

  if (error || !status) {
    return (
      <div>
        <p className="mb-2 text-[12px] font-medium text-ink-3">This machine</p>
        <SettingsCard className="px-3.5 py-3">
          <p className="text-[13px] text-danger">
            {error?.message || 'Unable to load local machine bridge status.'}
          </p>
        </SettingsCard>
      </div>
    )
  }

  const config = status.config
  const badge = bridgeStatusBadge(status, cloudActive)
  const displayedName = nameDraft ?? config.display_name
  const trimmedName = displayedName.trim()
  const nameDirty = nameDraft !== null && trimmedName !== config.display_name
  const baseConfig: MachineBridgeShareConfig =
    nameDraft !== null ? { ...config, display_name: trimmedName } : config

  const saveConfig = (
    next: MachineBridgeShareConfig,
    successMessage?: string,
    onAccepted?: () => void,
  ) => {
    const seq = requestSeqRef.current + 1
    requestSeqRef.current = seq
    mutation.mutate({ seq, config: next, successMessage, onAccepted })
  }

  const commitName = () => {
    if (nameDraft === null) {
      return
    }
    if (trimmedName === config.display_name) {
      setNameDraft(null)
      return
    }
    saveConfig({ ...config, display_name: trimmedName }, 'Machine name updated')
  }

  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <p className="text-[12px] font-medium text-ink-3">This machine</p>
          <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium ${badge.pillClass}`}>
            <span className={`size-1.5 shrink-0 rounded-full ${badge.dotClass}`} />
            {badge.label}
          </span>
        </div>
        {status.device_id ? (
          <span className="font-mono text-[11px] text-ink-3 select-all" title="Device ID">
            {status.device_id}
          </span>
        ) : null}
      </div>

      <SettingsCard className="overflow-hidden">
        {status.last_error ? (
          <div className="border-b border-border px-3 py-2.5">
            <p role="alert" className="text-[12px] text-danger">
              {status.last_error}
            </p>
          </div>
        ) : null}

        <SettingsRow
          title="Enable sharing from this machine"
          description="Keep this workstation connected to your active Cloud workspace so cloud threads can access approved folders."
        >
          <Switch
            checked={config.enabled}
            disabled={mutation.isPending}
            onChange={(checked) =>
              saveConfig(
                { ...baseConfig, enabled: checked },
                checked ? 'Machine sharing enabled' : 'Machine sharing paused',
              )
            }
            aria-label="Enable sharing from this machine"
          />
        </SettingsRow>

        <SettingsRow
          title="Machine name"
          description="Shown when selecting an execution target in cloud threads."
        >
          <form
            onSubmit={(event) => {
              event.preventDefault()
              commitName()
            }}
            className="flex items-center gap-1.5"
          >
            <Input
              value={displayedName}
              disabled={mutation.isPending}
              onChange={(event) => setNameDraft(event.target.value)}
              onBlur={commitName}
              onKeyDown={(event) => {
                if (event.key === 'Escape') {
                  setNameDraft(null)
                }
              }}
              placeholder={localDeviceLabel()}
              aria-label="Machine name"
              className="h-8 w-52 text-[13px]"
            />
            {nameDirty ? (
              <Button
                type="submit"
                size="sm"
                variant="secondary"
                disabled={mutation.isPending}
                onMouseDown={(event) => event.preventDefault()}
              >
                Save
              </Button>
            ) : null}
          </form>
        </SettingsRow>

        <AllowedFoldersEditor
          grants={config.grants}
          disabled={mutation.isPending}
          layoutId="bridge-new-grant-mode"
          pathAriaLabel="Folder path"
          description="Only folders listed here can be read, staged to cloud runs, or accessed by local execution."
          emptyMessage="No folders shared yet. Add a folder below to grant cloud threads access."
          onAdd={(grant, onAccepted) =>
            saveConfig({ ...baseConfig, grants: [...baseConfig.grants, grant] }, 'Folder added', onAccepted)
          }
          onChangeMode={(id, mode) =>
            saveConfig(
              {
                ...baseConfig,
                grants: baseConfig.grants.map((grant) => (grant.id === id ? { ...grant, mode } : grant)),
              },
              'Folder access updated',
            )
          }
          onRemove={(id) =>
            saveConfig(
              { ...baseConfig, grants: baseConfig.grants.filter((grant) => grant.id !== id) },
              'Folder removed',
            )
          }
        />

        <SettingsRow
          title="Allow local execution (codex exec-server)"
          description="Opt-in. Lets cloud threads run sandboxed commands locally inside your Allowed Folders."
        >
          <Switch
            checked={config.allow_exec}
            disabled={mutation.isPending}
            onChange={(checked) =>
              saveConfig(
                { ...baseConfig, allow_exec: checked },
                checked ? 'Local execution enabled' : 'Local execution disabled',
              )
            }
            aria-label="Allow local execution"
          />
        </SettingsRow>
      </SettingsCard>
    </div>
  )
}

function ConnectAnotherWorkstationDisclosure({ cloudUrl }: { cloudUrl: string }) {
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

function StandaloneWorkstationSection({ cloudUrl }: { cloudUrl: string }) {
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

function MachineRow({ machine, isThisMachine }: { machine: ConnectedMachine; isThisMachine: boolean }) {
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
          {isThisMachine ? (
            <span className="shrink-0 rounded-full bg-primary-soft px-2 py-0.5 text-[11px] font-medium text-primary">
              This machine
            </span>
          ) : null}
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
