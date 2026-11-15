import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Input } from '@/components/ui/Input'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { installPlugins, installRepository, updatePlugin, pluginsQuery, removePlugin, type Installed, type Plugin } from '@/lib/api/plugins'
import { keys } from '@/lib/query/keys'

export function PluginsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const plugins = useQuery(pluginsQuery)
  const picker = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const [adding, setAdding] = useState(false)
  const [repository, setRepository] = useState('')
  const [ref, setRef] = useState('')
  const report = (result: Installed, action: 'Installed' | 'Updated') => {
    if (result.plugins.length) toast(`${action} ${result.plugins.map((plugin) => plugin.display_name).join(', ')}`)
    if (result.skipped?.length) toast(`Skipped ${result.skipped.map((plugin) => `${plugin.name}: ${plugin.reason}`).join('; ')}`)
  }
  const refresh = () => Promise.all([keys.plugins, keys.mcpServers, keys.mcpApps].map((queryKey) => queryClient.invalidateQueries({ queryKey })))
  const install = useMutation({
    mutationFn: installPlugins,
    onSuccess: (installed) => toast(`Installed ${installed.map((plugin) => plugin.display_name).join(', ')}`),
    onError: (error: Error) => toast(`Couldn't install: ${error.message}`, 'danger'),
    onSettled: refresh,
  })
  const repositoryInstall = useMutation({
    mutationFn: installRepository,
    onSuccess: (result) => {
      report(result, 'Installed')
      setAdding(false)
    },
    onError: (error: Error) => toast(`Couldn't install: ${error.message}`, 'danger'),
    onSettled: refresh,
  })
  const update = useMutation({
    mutationFn: updatePlugin,
    onSuccess: (result) => report(result, 'Updated'),
    onError: (error: Error) => toast(`Couldn't update: ${error.message}`, 'danger'),
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
          <p className="mt-0.5 text-[13px] text-ink-2">A plugin adds skills, MCP servers and views. Install from a Git repository or a ZIP.</p>
        </div>
        <Button variant="primary" disabled={install.isPending} onClick={() => setAdding(!adding)}>
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
      {adding ? (
        <form className="mb-4 flex flex-col gap-3 rounded-card bg-surface p-3" onSubmit={(event) => {
          event.preventDefault()
          repositoryInstall.mutate({ repository, ref: ref || undefined })
        }}>
          <label className="flex flex-col gap-1 text-[12px] text-ink-2">
            Repository
            <Input value={repository} onChange={(event) => setRepository(event.target.value)} placeholder="owner/repo or Git URL" autoComplete="off" disabled={repositoryInstall.isPending} />
          </label>
          <label className="flex flex-col gap-1 text-[12px] text-ink-2">
            Ref (optional)
            <Input value={ref} onChange={(event) => setRef(event.target.value)} placeholder="Branch, tag or commit" autoComplete="off" disabled={repositoryInstall.isPending} />
          </label>
          <p className="text-[12px] text-ink-3">Private repositories need Git access on the Jaz backend.</p>
          <p className="text-[12px] text-ink-3">Plugins can run commands on the backend machine, so install only from sources you trust.</p>
          <div className="flex gap-2">
            <Button type="submit" variant="primary" disabled={repositoryInstall.isPending || !repository.trim()}>{repositoryInstall.isPending ? 'Installing…' : 'Install repository'}</Button>
            <Button type="button" variant="ghost" disabled={install.isPending} onClick={() => picker.current?.click()}>Choose ZIP</Button>
          </div>
        </form>
      ) : null}
      {plugins.isPending ? <SkeletonRows count={2} /> : plugins.isError ? (
        <p className="py-2 text-[13px] text-danger" role="alert">{plugins.error.message}</p>
      ) : (
        <div className={`flex flex-col gap-2 rounded-card transition-colors ${over ? 'bg-surface-2' : ''}`}>
          {plugins.data.map((plugin) => (
            <PluginCard key={plugin.name} plugin={plugin} removing={remove.isPending} updating={update.isPending && update.variables === plugin.name} onUpdate={() => update.mutate(plugin.name)} onRemove={() => remove.mutate(plugin.name)} />
          ))}
          {plugins.data.length === 0 ? (
            <p className="rounded-card bg-surface px-3 py-6 text-center text-[13px] text-ink-3" role="status">No plugins yet. Drop a plugin's .zip here.</p>
          ) : null}
        </div>
      )}
    </section>
  )
}

function PluginCard({ plugin, removing, updating, onUpdate, onRemove }: { plugin: Plugin; removing: boolean; updating: boolean; onUpdate: () => void; onRemove: () => void }) {
  const title = plugin.display_name
  return (
    <SettingsCard className="px-3 py-2.5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium text-ink">
            {title}
            {plugin.version ? <span className="ml-2 font-normal tabular-nums text-ink-3">{plugin.version}</span> : null}
            {plugin.source ? <span title={plugin.source.repository} className="ml-2 break-all font-normal text-ink-3">{plugin.source.repository}{plugin.source.ref ? `@${plugin.source.ref}` : ''} · {plugin.source.commit.slice(0, 8)}</span> : null}
          </p>
          {plugin.description ? <p className="mt-0.5 text-[12px] text-ink-2">{plugin.description}</p> : null}
          <Names label="Skills" names={plugin.skills} />
          <Names label="Views" names={plugin.views} />
          <Names label="MCP servers" names={plugin.mcp_servers} />
          {plugin.warnings?.map((warning) => <p key={warning} className="mt-1.5 text-[12px] text-ink-3">{warning}</p>)}
        </div>
        {plugin.source ? <Button variant="ghost" size="sm" disabled={removing || updating} onClick={onUpdate}>{updating ? 'Updating…' : 'Update'}</Button> : null}
        <IconButton variant="ghost" size="xs" aria-label={`Remove ${title}`} title={`Remove ${title}`} disabled={removing || updating} onClick={onRemove}>
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
