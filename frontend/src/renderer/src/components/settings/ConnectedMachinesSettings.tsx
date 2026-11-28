import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Folder } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import {
  AddMachineDisclosure,
  FoldersEditor,
  RunCommandsRow,
} from '@/components/settings/ConnectedMachinesEditors'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { useCloudMachinesQuery } from '@/lib/hooks/useCloudCatalogQueries'
import {
  machineBridgeQuery,
  machineBridgeQueryKey,
  isMachineBridgeUnavailable,
  updateMachineBridge,
  type MachineBridgeShareConfig,
  type MachineBridgeStatus,
} from '@/lib/api/machineBridge'
import type { ConnectedMachine } from '@/lib/api/serverTypes'
import { clientRuntime } from '@/lib/clientRuntime'
import { FOLDER_ACCESS_LABELS } from '@/lib/cloudMachineConfig'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { relativeTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

const RECONNECT_REFRESH_MS = 1_200

function platformLabel(platform: string): string {
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
      return platform
  }
}

function bridgeState(status: MachineBridgeStatus, cloudActive: boolean): { label: string; dotClass: string } {
  if (!status.config.enabled) {
    return { label: 'Not shared', dotClass: 'bg-ink-3/40' }
  }
  if (!cloudActive) {
    return { label: 'Waiting for Cloud', dotClass: 'bg-ink-3/40' }
  }
  if (status.connected) {
    return { label: 'Connected', dotClass: 'bg-ok' }
  }
  return { label: 'Connecting', dotClass: 'bg-ink-3 animate-pulse' }
}

export function ConnectedMachinesSettings() {
  const { url: cloudUrl, active: cloudActive } = useCloudSession()
  const isCloudOnly = useIsCloudOnly()
  const canManageLocalBridge = clientRuntime.capabilities.localBackend && !isCloudOnly
  const machines = useQuery({ ...useCloudMachinesQuery(), enabled: cloudActive })
  const bridge = useQuery({ ...machineBridgeQuery, enabled: canManageLocalBridge })
  const hasThisMachine = canManageLocalBridge && !isMachineBridgeUnavailable(bridge.error)
  const otherMachines =
    hasThisMachine && bridge.isPending
      ? []
      : (machines.data ?? []).filter((machine) => machine.device_id !== bridge.data?.device_id)

  return (
    <section className="flex flex-col gap-5 py-4">
      <div>
        <h1 className="text-lg font-semibold text-ink">Connected Machines</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">Let Cloud threads work in folders on your computers.</p>
      </div>

      {hasThisMachine ? (
        <ThisMachineCard
          status={bridge.data}
          isPending={bridge.isPending}
          error={bridge.error}
          cloudActive={cloudActive}
        />
      ) : null}

      {machines.isError ? (
        <p className="text-[13px] text-danger">{machines.error.message}</p>
      ) : otherMachines.length > 0 ? (
        <div>
          {hasThisMachine ? <p className="mb-2 text-[12px] font-medium text-ink-3">Other machines</p> : null}
          <SettingsCard className="overflow-hidden">
            {otherMachines.map((machine) => (
              <MachineRow key={machine.device_id} machine={machine} />
            ))}
          </SettingsCard>
        </div>
      ) : null}

      <AddMachineDisclosure
        key={cloudUrl}
        cloudUrl={cloudUrl}
        label={hasThisMachine ? 'Add another machine' : 'Add a machine'}
        defaultOpen={!hasThisMachine && machines.data?.length === 0}
      />
    </section>
  )
}

function ThisMachineCard({
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
    mutationFn: (vars: { seq: number; config: MachineBridgeShareConfig; successMessage: string }) =>
      updateMachineBridge(vars.config),
    onSuccess: (updated, vars) => {
      if (vars.seq === requestSeqRef.current) {
        queryClient.setQueryData(machineBridgeQueryKey, updated)
        setNameDraft(null)
      }
      toast(vars.successMessage)
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
      <SettingsCard className="p-3">
        <SkeletonRows count={3} />
      </SettingsCard>
    )
  }

  if (error || !status) {
    return (
      <SettingsCard className="px-3 py-2.5">
        <p className="text-[13px] text-danger">{error?.message || 'Unable to load this machine.'}</p>
      </SettingsCard>
    )
  }

  const config = status.config
  const state = bridgeState(status, cloudActive)
  const displayedName = nameDraft ?? config.display_name
  const trimmedName = displayedName.trim()
  const nameDirty = nameDraft !== null && trimmedName !== config.display_name
  const baseConfig: MachineBridgeShareConfig =
    nameDraft !== null ? { ...config, display_name: trimmedName } : config

  const saveConfig = (next: MachineBridgeShareConfig, successMessage: string) => {
    const seq = requestSeqRef.current + 1
    requestSeqRef.current = seq
    mutation.mutate({ seq, config: next, successMessage })
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
    <SettingsCard className="overflow-hidden">
      {status.last_error ? (
        <p role="alert" className="border-b border-border px-3 py-2.5 text-[12px] text-danger">
          {status.last_error}
        </p>
      ) : null}

      <SettingsRow title="Share this machine" description="Keeps it available to your Cloud workspace.">
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-1.5 text-[12px] text-ink-3">
            <span className={`size-1.5 rounded-full ${state.dotClass}`} />
            {state.label}
          </span>
          <Switch
            checked={config.enabled}
            disabled={mutation.isPending}
            onChange={(checked) =>
              saveConfig(
                { ...baseConfig, enabled: checked },
                checked ? 'Machine sharing enabled' : 'Machine sharing paused',
              )
            }
            aria-label="Share this machine"
          />
        </div>
      </SettingsRow>

      <SettingsRow title="Name">
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
            aria-label="Machine name"
            className="h-8 min-w-72 text-[13px]"
          />
          {nameDirty ? (
            <Button
              type="submit"
              size="sm"
              disabled={mutation.isPending}
              onMouseDown={(event) => event.preventDefault()}
            >
              Save
            </Button>
          ) : null}
        </form>
      </SettingsRow>

      <FoldersEditor
        grants={config.grants}
        disabled={mutation.isPending}
        pickDirectory={clientRuntime.pickDirectory}
        onChange={(grants, change) => saveConfig({ ...baseConfig, grants }, change)}
      />

      <RunCommandsRow
        machine="this machine"
        checked={config.allow_exec}
        disabled={mutation.isPending}
        onChange={(allow_exec) =>
          saveConfig({ ...baseConfig, allow_exec }, allow_exec ? 'Running commands enabled' : 'Running commands disabled')
        }
      />
    </SettingsCard>
  )
}

function MachineRow({ machine }: { machine: ConnectedMachine }) {
  const caps = machine.capabilities
  const details = [
    platformLabel(machine.platform),
    caps.local_exec && 'Runs commands',
    caps.local_browser && 'Browser',
    caps.computer_use && 'Computer use',
  ].filter(Boolean)
  const seen = relativeTime(machine.last_seen_at)
  return (
    <div className="border-t border-border px-3 py-2.5 first:border-t-0">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`size-1.5 shrink-0 rounded-full ${machine.online ? 'bg-ok' : 'bg-ink-3/40'}`} />
          <span className="truncate text-[13px] font-medium text-ink">
            {machine.display_name || machine.device_id}
          </span>
        </div>
        <span className="shrink-0 text-[12px] text-ink-3 tabular-nums">
          {machine.online ? 'Online' : seen ? `Seen ${seen}` : 'Offline'}
        </span>
      </div>
      <p className="mt-0.5 pl-3.5 text-[12px] text-ink-3">{details.join(' · ')}</p>
      {machine.folder_grants.map((grant) => (
        <div key={grant.id} className="mt-1.5 flex items-center gap-2 pl-3.5 text-[12px]">
          <Folder size={13} className="shrink-0 text-ink-3" />
          <span className="min-w-0 flex-1 truncate text-ink-2" title={grant.root_path}>
            {grant.root_path}
          </span>
          <span className="shrink-0 text-ink-3">{FOLDER_ACCESS_LABELS[grant.mode]}</span>
        </div>
      ))}
    </div>
  )
}
