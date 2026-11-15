import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { installPlugins, pluginsQuery, removePlugin, type Plugin } from '@/lib/api/plugins'
import { keys } from '@/lib/query/keys'

export function PluginsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const plugins = useQuery(pluginsQuery)
  const picker = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.plugins })
  const install = useMutation({
    mutationFn: installPlugins,
    onSuccess: (installed) => toast(`Installed ${installed.map((plugin) => plugin.display_name).join(', ')}`),
    onError: (error: Error) => toast(`Couldn't install: ${error.message}`, 'danger'),
    onSettled: refresh,
  })
  const remove = useMutation({
    mutationFn: removePlugin,
    onError: (error: Error) => toast(`Couldn't remove: ${error.message}`, 'danger'),
    onSettled: refresh,
  })
  const add = (file?: File) => {
    if (file) install.mutate(file)
  }

  return (
    <section
      className="pb-4"
      onDragOver={(event) => {
        event.preventDefault()
        setOver(true)
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(event) => {
        event.preventDefault()
        setOver(false)
        add(event.dataTransfer.files[0])
      }}
    >
      <div className="flex items-start justify-between gap-4 py-4">
        <div className="min-w-0">
          <p className="text-sm font-medium text-ink">Plugins</p>
          <p className="mt-0.5 text-[13px] text-ink-2">A plugin adds skills and the views they show. Add one from a .zip of its folder, or of a repository that holds several.</p>
        </div>
        <Button variant="primary" disabled={install.isPending} onClick={() => picker.current?.click()}>
          {install.isPending ? 'Installing…' : 'Add plugin'}
        </Button>
        <input
          ref={picker}
          type="file"
          accept=".zip,application/zip"
          hidden
          onChange={(event) => {
            add(event.target.files?.[0])
            event.target.value = ''
          }}
        />
      </div>
      {plugins.isPending ? <SkeletonRows count={2} /> : plugins.isError ? (
        <p className="py-2 text-[13px] text-danger" role="alert">{plugins.error.message}</p>
      ) : (
        <div className={`flex flex-col gap-2 rounded-card transition-colors ${over ? 'bg-surface-2' : ''}`}>
          {plugins.data.map((plugin) => (
            <PluginCard key={plugin.name} plugin={plugin} removing={remove.isPending} onRemove={() => remove.mutate(plugin.name)} />
          ))}
          {plugins.data.length === 0 ? (
            <p className="rounded-card bg-surface px-3 py-6 text-center text-[13px] text-ink-3" role="status">No plugins yet. Drop a plugin's .zip here.</p>
          ) : null}
        </div>
      )}
    </section>
  )
}

function PluginCard({ plugin, removing, onRemove }: { plugin: Plugin; removing: boolean; onRemove: () => void }) {
  const title = plugin.display_name
  return (
    <SettingsCard className="px-3 py-2.5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium text-ink">
            {title}
            {plugin.version ? <span className="ml-2 font-normal tabular-nums text-ink-3">{plugin.version}</span> : null}
          </p>
          {plugin.description ? <p className="mt-0.5 text-[12px] text-ink-2">{plugin.description}</p> : null}
          <Names label="Skills" names={plugin.skills} />
          <Names label="Views" names={plugin.views} />
          {plugin.warnings?.map((warning) => <p key={warning} className="mt-1.5 text-[12px] text-ink-3">{warning}</p>)}
        </div>
        <IconButton variant="ghost" size="xs" aria-label={`Remove ${title}`} title={`Remove ${title}`} disabled={removing} onClick={onRemove}>
          <Trash2 size={14} />
        </IconButton>
      </div>
    </SettingsCard>
  )
}

function Names({ label, names }: { label: string; names: string[] }) {
  if (names.length === 0) return null
  return (
    <p className="mt-1.5 flex flex-wrap items-center gap-1 text-[12px] text-ink-3">
      {label}
      {names.map((name) => <span key={name} className="rounded-full bg-surface-2 px-2 py-0.5 text-ink-2">{name}</span>)}
    </p>
  )
}
