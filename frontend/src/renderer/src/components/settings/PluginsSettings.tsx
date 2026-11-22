import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDownToLine, Loader2, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { SearchField } from '@/components/ui/SearchField'
import { Segmented } from '@/components/ui/Segmented'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import {
  addPluginSource,
  installPlugin,
  refreshPluginSources,
  removePlugin,
  removePluginSource,
  setPluginEnabled,
  updatePlugin,
  type Plugin,
  type PluginInstallInput,
  type PluginInstallResult,
  type PluginSource,
} from '@/lib/api/plugins'
import { keys } from '@/lib/query/keys'
import { AddPluginModal } from './AddPluginModal'
import { ConnectionQRModal } from './ConnectionQRModal'
import { PluginDetailModal } from './PluginDetailModal'
import { pluginActionLabel, pluginCanConnect, pluginSummary } from './pluginFormatting'
import { PluginAvatar } from './PluginVisuals'
import { usePluginSignIn } from './usePluginSignIn'

type PluginsTab = 'installed' | 'browse' | 'sources'

export function PluginsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [tab, setTab] = useState<PluginsTab>('installed')
  const [query, setQuery] = useState('')
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)

  const signIn = usePluginSignIn({ onStartAccepted: () => setSelectedID(null) })
  const catalog = signIn.catalog
  const allPlugins = signIn.plugins
  const sources = catalog.data?.sources ?? []

  const invalidateAll = () =>
    Promise.all([keys.plugins, keys.mcpServers, keys.mcpApps].map((queryKey) => queryClient.invalidateQueries({ queryKey })))

  const reportInstall = (result: PluginInstallResult, action: 'Installed' | 'Updated') => {
    if (result.plugins.length) {
      toast(`${action} ${result.plugins.map((plugin) => plugin.display_name).join(', ')}`)
    }
    if (result.skipped?.length) {
      toast(`Skipped ${result.skipped.map((plugin) => `${plugin.name}: ${plugin.reason}`).join('; ')}`)
    }
  }

  const install = useMutation({
    mutationFn: (input: PluginInstallInput) => installPlugin(input),
    onSuccess: (result) => {
      reportInstall(result, 'Installed')
      setAdding(false)
    },
    onError: (error: Error) => toast(`Couldn't install: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const update = useMutation({
    mutationFn: (id: string) => updatePlugin(id),
    onSuccess: (result) => reportInstall(result, 'Updated'),
    onError: (error: Error) => toast(`Couldn't update: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const remove = useMutation({
    mutationFn: (id: string) => removePlugin(id),
    onSuccess: () => {
      setSelectedID(null)
      toast('Removed plugin')
    },
    onError: (error: Error) => toast(`Couldn't remove: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const toggleEnabled = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => setPluginEnabled(id, enabled),
    onError: (error: Error) => toast(`Couldn't update plugin: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const addSource = useMutation({
    mutationFn: (input: PluginInstallInput) => addPluginSource(input),
    onSuccess: (source) => {
      setAdding(false)
      setTab(source.is_marketplace ? 'browse' : 'sources')
      toast(`Added source ${source.repository}`)
    },
    onError: (error: Error) => toast(`Couldn't add source: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const removeSource = useMutation({
    mutationFn: (id: string) => removePluginSource(id),
    onSuccess: () => toast('Removed source'),
    onError: (error: Error) => toast(`Couldn't remove source: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const refresh = useMutation({
    mutationFn: refreshPluginSources,
    onSuccess: () => toast('Checked plugin sources for updates'),
    onError: (error: Error) => toast(`Couldn't refresh sources: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const installedPlugins = useMemo(
    () => filterPlugins(allPlugins.filter((plugin) => plugin.installed), query),
    [allPlugins, query],
  )
  const browsePlugins = useMemo(
    () => filterPlugins(allPlugins.filter((plugin) => !plugin.installed), query),
    [allPlugins, query],
  )
  const selected = useMemo(
    () => allPlugins.find((plugin) => plugin.id === selectedID) ?? null,
    [allPlugins, selectedID],
  )

  const installMarketplacePlugin = (plugin: Plugin) => {
    if (!plugin.source?.repository) return
    install.mutate({
      repository: plugin.source.repository,
      ref: plugin.source.ref,
      paths: plugin.source.path ? [plugin.source.path] : undefined,
      plugin_id: plugin.id,
      source_id: plugin.source.id,
    })
  }

  return (
    <section className="pb-6">
      <div className="flex flex-wrap items-start justify-between gap-4 py-4">
        <div className="min-w-0">
          <p className="text-sm font-medium text-ink">Plugins</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Connect built-in integrations or install skills, MCP servers, and views from Git.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {sources.length > 0 ? (
            <Button variant="ghost" size="sm" disabled={refresh.isPending} onClick={() => refresh.mutate()}>
              <RefreshCw size={13} className={refresh.isPending ? 'animate-spin' : undefined} />
              Refresh
            </Button>
          ) : null}
          <Button variant="primary" size="sm" onClick={() => setAdding(true)}>
            <Plus size={14} />
            Add from Git
          </Button>
        </div>
      </div>

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <Segmented
          layoutId="plugins-settings-tab"
          value={tab}
          onChange={setTab}
          options={[
            { value: 'installed', label: 'Installed' },
            { value: 'browse', label: browsePlugins.length ? `Browse (${browsePlugins.length})` : 'Browse' },
            { value: 'sources', label: sources.length ? `Sources (${sources.length})` : 'Sources' },
          ]}
        />
        {tab !== 'sources' ? (
          <SearchField
            value={query}
            onChange={setQuery}
            placeholder="Filter plugins…"
            className="h-8 w-52 max-sm:w-full"
          />
        ) : null}
      </div>

      {catalog.isPending ? (
        <SkeletonRows count={4} />
      ) : catalog.isError ? (
        <p className="py-2 text-[13px] text-danger" role="alert">
          {catalog.error.message}
        </p>
      ) : tab === 'installed' ? (
        installedPlugins.length === 0 ? (
          <p className="rounded-card bg-surface px-4 py-8 text-center text-[13px] text-ink-3" role="status">
            {query ? 'No installed plugins match your filter.' : 'No plugins installed yet.'}
          </p>
        ) : (
          <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
            {installedPlugins.map((plugin) => (
              <InstalledPluginCard
                key={plugin.id}
                plugin={plugin}
                connecting={signIn.isConnecting && signIn.connectingPluginID === plugin.id}
                toggling={toggleEnabled.isPending && toggleEnabled.variables?.id === plugin.id}
                onOpen={() => setSelectedID(plugin.id)}
                onConnect={() => signIn.start(plugin)}
                onToggle={(enabled) => toggleEnabled.mutate({ id: plugin.id, enabled })}
              />
            ))}
          </div>
        )
      ) : tab === 'browse' ? (
        browsePlugins.length === 0 ? (
          <div className="rounded-card bg-surface px-4 py-8 text-center text-[13px] text-ink-3">
            <p>{query ? 'No marketplace plugins match your filter.' : 'No marketplace catalogs added yet.'}</p>
            {!query ? (
              <Button variant="secondary" size="sm" className="mt-3" onClick={() => setAdding(true)}>
                <Plus size={13} />
                Add marketplace from Git
              </Button>
            ) : null}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
            {browsePlugins.map((plugin) => (
              <BrowsePluginCard
                key={plugin.id}
                plugin={plugin}
                installing={install.isPending && install.variables?.plugin_id === plugin.id}
                onOpen={() => setSelectedID(plugin.id)}
                onInstall={() => installMarketplacePlugin(plugin)}
              />
            ))}
          </div>
        )
      ) : sources.length === 0 ? (
        <div className="rounded-card bg-surface px-4 py-8 text-center text-[13px] text-ink-3">
          <p>No Git marketplace sources configured.</p>
          <Button variant="secondary" size="sm" className="mt-3" onClick={() => setAdding(true)}>
            <Plus size={13} />
            Add Git source
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {sources.map((source) => (
            <SourceRow
              key={source.id}
              source={source}
              removing={removeSource.isPending && removeSource.variables === source.id}
              onRemove={() => removeSource.mutate(source.id)}
            />
          ))}
        </div>
      )}

      <PluginDetailModal
        plugin={selected}
        connecting={signIn.isConnecting && signIn.connectingPluginID === selected?.id}
        installing={install.isPending && install.variables?.plugin_id === selected?.id}
        updating={update.isPending && update.variables === selected?.id}
        removing={remove.isPending && remove.variables === selected?.id}
        toggling={toggleEnabled.isPending && toggleEnabled.variables?.id === selected?.id}
        onClose={() => setSelectedID(null)}
        onConnect={(plugin) => signIn.start(plugin)}
        onInstall={installMarketplacePlugin}
        onUpdate={(plugin) => update.mutate(plugin.id)}
        onRemove={(plugin) => remove.mutate(plugin.id)}
        onToggleEnabled={(plugin, enabled) => toggleEnabled.mutate({ id: plugin.id, enabled })}
      />

      {adding ? (
        <AddPluginModal
          open
          busy={install.isPending || addSource.isPending}
          onClose={() => setAdding(false)}
          onInstall={(input) => install.mutate(input)}
          onAddSource={(input) => addSource.mutate(input)}
        />
      ) : null}

      <ConnectionQRModal {...signIn.qrModal} />
    </section>
  )
}

function InstalledPluginCard({
  plugin,
  connecting,
  toggling,
  onOpen,
  onConnect,
  onToggle,
}: {
  plugin: Plugin
  connecting: boolean
  toggling: boolean
  onOpen: () => void
  onConnect: () => void
  onToggle: (enabled: boolean) => void
}) {
  const connected = Boolean(plugin.auth?.connected)
  const needsConnect = Boolean(plugin.auth && !connected && pluginCanConnect(plugin))
  const summary = pluginSummary(plugin)

  return (
    <SettingsCard className="flex flex-col justify-between gap-3 px-3.5 py-3 transition-colors hover:bg-surface-2/60">
      <div className="flex items-start gap-3">
        <button type="button" onClick={onOpen} className="flex min-w-0 flex-1 cursor-pointer items-start gap-3 text-left">
          <PluginAvatar plugin={plugin} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5">
              <p className="truncate text-[13.5px] font-medium text-ink">{plugin.display_name}</p>
              {connected ? (
                <span className="size-1.5 shrink-0 rounded-full bg-ok" title="Connected" />
              ) : null}
              {plugin.source?.update_available ? (
                <span className="rounded-full bg-primary/15 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                  Update
                </span>
              ) : null}
            </div>
            {summary ? (
              <p className="mt-0.5 line-clamp-2 text-[12px] leading-4 text-ink-2">{summary}</p>
            ) : null}
          </div>
        </button>
        <Switch
          checked={plugin.enabled}
          disabled={toggling}
          aria-label={`Enable ${plugin.display_name}`}
          onChange={onToggle}
        />
      </div>

      {needsConnect ? (
        <div className="flex items-center justify-end border-t border-border/50 pt-2">
          <Button size="sm" variant="secondary" disabled={connecting} onClick={onConnect}>
            {connecting ? <Loader2 size={12} className="animate-spin" /> : null}
            {pluginActionLabel(plugin, connecting)}
          </Button>
        </div>
      ) : null}
    </SettingsCard>
  )
}

function BrowsePluginCard({
  plugin,
  installing,
  onOpen,
  onInstall,
}: {
  plugin: Plugin
  installing: boolean
  onOpen: () => void
  onInstall: () => void
}) {
  return (
    <SettingsCard className="flex items-start gap-3 px-3.5 py-3 transition-colors hover:bg-surface-2/60">
      <button type="button" onClick={onOpen} className="flex min-w-0 flex-1 cursor-pointer items-start gap-3 text-left">
        <PluginAvatar plugin={plugin} />
        <div className="min-w-0 flex-1">
          <p className="truncate text-[13.5px] font-medium text-ink">{plugin.display_name}</p>
          {plugin.description ? (
            <p className="mt-0.5 line-clamp-2 text-[12px] leading-4 text-ink-2">{plugin.description}</p>
          ) : null}
        </div>
      </button>
      <Button size="sm" variant="secondary" disabled={installing} onClick={onInstall} className="shrink-0">
        {installing ? <Loader2 size={12} className="animate-spin" /> : <ArrowDownToLine size={12} />}
        {installing ? 'Installing…' : 'Install'}
      </Button>
    </SettingsCard>
  )
}

function SourceRow({
  source,
  removing,
  onRemove,
}: {
  source: PluginSource
  removing: boolean
  onRemove: () => void
}) {
  return (
    <SettingsCard className="flex items-center justify-between gap-3 px-3.5 py-2.5">
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-ink">
          {source.repository}
          {source.ref ? <span className="ml-1.5 font-normal text-ink-3">@{source.ref}</span> : null}
        </p>
        <p className="mt-0.5 truncate text-[12px] text-ink-3">
          {source.is_marketplace ? 'Marketplace' : 'Repository'}
          {source.commit_sha ? ` · ${source.commit_sha.slice(0, 8)}` : ''}
          {source.auto_update ? ' · Auto-update' : ''}
        </p>
      </div>
      <IconButton
        variant="ghost"
        size="xs"
        aria-label={`Remove ${source.repository}`}
        title={`Remove ${source.repository}`}
        disabled={removing}
        onClick={onRemove}
      >
        <Trash2 size={14} />
      </IconButton>
    </SettingsCard>
  )
}

function filterPlugins(plugins: Plugin[], query: string): Plugin[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return plugins
  return plugins.filter((plugin) => {
    const haystack = [
      plugin.id,
      plugin.display_name,
      plugin.description,
      plugin.category,
      ...(plugin.keywords ?? []),
      ...(plugin.skills?.map((s) => s.name) ?? []),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return haystack.includes(needle)
  })
}
