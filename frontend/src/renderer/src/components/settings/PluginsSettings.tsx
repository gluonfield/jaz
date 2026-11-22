import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, MoreHorizontal, Plus, RefreshCw, Settings2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { SearchField } from '@/components/ui/SearchField'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  addPluginSource,
  disconnectPluginAccount,
  installPlugin,
  refreshPluginSources,
  removePlugin,
  removePluginSource,
  updatePlugin,
  type Plugin,
  type PluginInstallInput,
  type PluginInstallResult,
} from '@/lib/api/plugins'
import { keys } from '@/lib/query/keys'
import { AddPluginModal, type AddPluginMode } from './AddPluginModal'
import { ConnectionQRModal } from './ConnectionQRModal'
import { MarketplacesModal } from './MarketplacesModal'
import { PluginDetailModal } from './PluginDetailModal'
import { categoryLabel, pluginCanConnect, pluginSummary } from './pluginFormatting'
import { PluginAvatar } from './PluginVisuals'
import { usePluginSignIn } from './usePluginSignIn'

export function PluginsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [query, setQuery] = useState('')
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [addMode, setAddMode] = useState<AddPluginMode | null>(null)
  const [managingSources, setManagingSources] = useState(false)

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
      setAddMode(null)
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

  const disconnect = useMutation({
    mutationFn: (accountID: string) => disconnectPluginAccount(accountID),
    onSuccess: () => toast('Disconnected'),
    onError: (error: Error) => toast(`Couldn't disconnect: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const addSource = useMutation({
    mutationFn: (input: PluginInstallInput) => addPluginSource(input),
    onSuccess: (source) => {
      setAddMode(null)
      toast(`Added marketplace ${source.repository}`)
    },
    onError: (error: Error) => toast(`Couldn't add marketplace: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const removeSource = useMutation({
    mutationFn: (id: string) => removePluginSource(id),
    onSuccess: () => toast('Removed marketplace'),
    onError: (error: Error) => toast(`Couldn't remove marketplace: ${error.message}`, 'danger'),
    onSettled: invalidateAll,
  })

  const refresh = useMutation({
    mutationFn: refreshPluginSources,
    onSuccess: (updated) => {
      queryClient.setQueryData(keys.plugins, updated)
      void queryClient.invalidateQueries({ queryKey: keys.mcpServers })
      void queryClient.invalidateQueries({ queryKey: keys.mcpApps })
      toast('Checked plugin catalogs for updates')
    },
    onError: (error: Error) => toast(`Couldn't refresh catalogs: ${error.message}`, 'danger'),
  })

  const autoRefreshed = useRef(false)
  useEffect(() => {
    if (autoRefreshed.current || !catalog.isSuccess) return
    const staleBefore = Date.now() - 60_000
    const isStale =
      sources.length === 0 ||
      sources.some((source) => !source.checked_at || !(Date.parse(source.checked_at) >= staleBefore))
    if (!isStale) return
    autoRefreshed.current = true
    void refreshPluginSources()
      .then((updated) => {
        queryClient.setQueryData(keys.plugins, updated)
      })
      .catch(() => undefined)
  }, [catalog.isSuccess, queryClient, sources])

  const filtered = useMemo(() => filterPlugins(allPlugins, query), [allPlugins, query])

  const connectedPlugins = useMemo(
    () => filtered.filter(isActivePlugin),
    [filtered],
  )

  const catalogGroups = useMemo(() => {
    const available = filtered.filter((plugin) => !isActivePlugin(plugin))
    return groupCatalogPlugins(available)
  }, [filtered])

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

  const quickAdd = (plugin: Plugin) => {
    if (plugin.auth && pluginCanConnect(plugin)) {
      signIn.start(plugin)
      return
    }
    if (!plugin.installed) {
      installMarketplacePlugin(plugin)
      return
    }
    setSelectedID(plugin.id)
  }

  const disconnectingAccountID = disconnect.isPending ? disconnect.variables : undefined

  return (
    <section className="pb-10">
      <div className="flex flex-wrap items-center justify-between gap-4 pb-6 pt-3">
        <div className="min-w-0">
          <h2 className="text-[20px] font-semibold tracking-tight text-ink">Plugins</h2>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Connect plugins to let Jaz work across your tools
          </p>
        </div>

        <div className="flex items-center gap-2">
          <SearchField
            value={query}
            onChange={setQuery}
            placeholder="Search plugins"
            className="h-8 w-48 max-sm:w-full"
          />
          <IconButton
            variant="ghost"
            size="sm"
            aria-label="Refresh plugin catalogs"
            title="Refresh plugin catalogs"
            disabled={refresh.isPending}
            onClick={() => refresh.mutate()}
          >
            <RefreshCw size={14} className={refresh.isPending ? 'animate-spin' : undefined} />
          </IconButton>
          <IconButton
            variant="ghost"
            size="sm"
            aria-label="Manage Git marketplaces"
            title="Manage Git marketplaces"
            onClick={() => setManagingSources(true)}
          >
            <Settings2 size={14} />
          </IconButton>
          <Button variant="primary" size="sm" onClick={() => setAddMode('install')}>
            <Plus size={14} />
            Add
          </Button>
        </div>
      </div>

      {catalog.isPending ? (
        <SkeletonRows count={5} />
      ) : catalog.isError ? (
        <p className="py-2 text-[13px] text-danger" role="alert">
          {catalog.error.message}
        </p>
      ) : filtered.length === 0 ? (
        <p className="rounded-card bg-surface px-4 py-10 text-center text-[13px] text-ink-3" role="status">
          {query ? 'No plugins match your search.' : 'No plugins available.'}
        </p>
      ) : (
        <div className="space-y-8">
          {connectedPlugins.length > 0 ? (
            <section className="space-y-1">
              <div className="flex items-center justify-between pb-1">
                <h3 className="text-[13px] font-semibold text-ink">Connected</h3>
                <span className="text-[12px] tabular-nums text-ink-3">{connectedPlugins.length}</span>
              </div>
              <div className="divide-y divide-border/50">
                {connectedPlugins.map((plugin) => {
                  const accounts = plugin.auth?.accounts ?? []
                  const singleAccount = accounts.length === 1 ? accounts[0] : null
                  const hasSkillsOrViews = (plugin.skills?.length ?? 0) > 0 || (plugin.views?.length ?? 0) > 0
                  const canRemove = plugin.installed && plugin.origin !== 'builtin' && (!plugin.auth || hasSkillsOrViews)
                  return (
                    <ConnectedPluginRow
                      key={plugin.id}
                      plugin={plugin}
                      busy={
                        Boolean(singleAccount && disconnectingAccountID === singleAccount.id) ||
                        (remove.isPending && remove.variables === plugin.id) ||
                        (update.isPending && update.variables === plugin.id)
                      }
                      onOpen={() => setSelectedID(plugin.id)}
                      onAddAccount={plugin.auth?.multi_account && pluginCanConnect(plugin) ? () => signIn.start(plugin) : undefined}
                      onDisconnect={singleAccount ? () => disconnect.mutate(singleAccount.id) : undefined}
                      onRemove={canRemove ? () => remove.mutate(plugin.id) : undefined}
                      onUpdate={plugin.installed && plugin.source ? () => update.mutate(plugin.id) : undefined}
                    />
                  )
                })}
              </div>
            </section>
          ) : null}

          {catalogGroups.map((group) => (
            <section key={group.label} className="space-y-2">
              <h3 className="text-[13px] font-semibold text-ink">{group.label}</h3>
              <div className="grid grid-cols-1 gap-x-8 gap-y-0.5 sm:grid-cols-2">
                {group.plugins.map((plugin) => {
                  const busy =
                    (signIn.isConnecting && signIn.connectingPluginID === plugin.id) ||
                    (install.isPending && install.variables?.plugin_id === plugin.id)
                  return (
                    <CatalogPluginRow
                      key={plugin.id}
                      plugin={plugin}
                      busy={busy}
                      onOpen={() => setSelectedID(plugin.id)}
                      onAdd={() => quickAdd(plugin)}
                    />
                  )
                })}
              </div>
            </section>
          ))}
        </div>
      )}

      <PluginDetailModal
        plugin={selected}
        connecting={signIn.isConnecting && signIn.connectingPluginID === selected?.id}
        disconnectingAccountID={disconnectingAccountID}
        installing={install.isPending && install.variables?.plugin_id === selected?.id}
        updating={update.isPending && update.variables === selected?.id}
        removing={remove.isPending && remove.variables === selected?.id}
        onClose={() => setSelectedID(null)}
        onConnect={(plugin) => signIn.start(plugin)}
        onDisconnect={(accountID) => disconnect.mutate(accountID)}
        onInstall={installMarketplacePlugin}
        onUpdate={(plugin) => update.mutate(plugin.id)}
        onRemove={(plugin) => remove.mutate(plugin.id)}
      />

      {addMode ? (
        <AddPluginModal
          key={addMode}
          open
          initialMode={addMode}
          busy={install.isPending || addSource.isPending}
          onClose={() => setAddMode(null)}
          onInstall={(input) => install.mutate(input)}
          onAddSource={(input) => addSource.mutate(input)}
        />
      ) : null}

      {managingSources ? (
        <MarketplacesModal
          sources={sources}
          removingID={removeSource.isPending ? removeSource.variables : undefined}
          onClose={() => setManagingSources(false)}
          onAdd={() => {
            setManagingSources(false)
            setAddMode('marketplace')
          }}
          onRemove={(id) => removeSource.mutate(id)}
        />
      ) : null}

      <ConnectionQRModal {...signIn.qrModal} />
    </section>
  )
}

function ConnectedPluginRow({
  plugin,
  busy,
  onOpen,
  onAddAccount,
  onDisconnect,
  onRemove,
  onUpdate,
}: {
  plugin: Plugin
  busy: boolean
  onOpen: () => void
  onAddAccount?: () => void
  onDisconnect?: () => void
  onRemove?: () => void
  onUpdate?: () => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const summary = pluginSummary(plugin)
  const multipleAccounts = (plugin.auth?.accounts?.length ?? 0) > 1
  const hasTopActions = Boolean(onUpdate || onAddAccount)
  const hasDestructive = Boolean(onDisconnect || onRemove || multipleAccounts)

  const run = (action: () => void) => {
    setMenuOpen(false)
    action()
  }

  return (
    <div className="flex items-center gap-3 py-3.5">
      <button type="button" onClick={onOpen} className="flex min-w-0 flex-1 cursor-pointer items-center gap-3.5 text-left">
        <PluginAvatar plugin={plugin} size="md" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="truncate text-[14px] font-medium text-ink">{plugin.display_name}</p>
            {plugin.source?.update_available ? (
              <span className="rounded-full bg-primary/15 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                Update
              </span>
            ) : null}
          </div>
          {summary ? <p className="mt-0.5 truncate text-[12.5px] text-ink-2">{summary}</p> : null}
        </div>
      </button>

      <Popover
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        placement="below"
        align="end"
        trigger={
          <IconButton
            variant="ghost"
            size="sm"
            aria-label={`${plugin.display_name} options`}
            aria-expanded={menuOpen}
            disabled={busy}
            onClick={() => setMenuOpen((open) => !open)}
          >
            {busy ? <Loader2 size={15} className="animate-spin" /> : <MoreHorizontal size={16} />}
          </IconButton>
        }
      >
        <div role="menu" className="flex flex-col gap-0.5">
          {onUpdate ? (
            <MenuRow role="menuitem" onClick={() => run(onUpdate)}>
              {plugin.source?.update_available ? 'Update plugin' : 'Check for updates'}
            </MenuRow>
          ) : null}
          {onAddAccount ? (
            <MenuRow role="menuitem" onClick={() => run(onAddAccount)}>
              Add account
            </MenuRow>
          ) : null}
          {hasTopActions && hasDestructive ? <div className="my-1 h-px bg-border/60" /> : null}
          {multipleAccounts ? (
            <MenuRow role="menuitem" onClick={() => run(onOpen)}>
              Manage accounts
            </MenuRow>
          ) : null}
          {onDisconnect ? (
            <MenuRow role="menuitem" onClick={() => run(onDisconnect)}>
              Disconnect
            </MenuRow>
          ) : null}
          {onRemove ? (
            <MenuRow role="menuitem" onClick={() => run(onRemove)}>
              Remove
            </MenuRow>
          ) : null}
        </div>
      </Popover>
    </div>
  )
}

function CatalogPluginRow({
  plugin,
  busy,
  onOpen,
  onAdd,
}: {
  plugin: Plugin
  busy: boolean
  onOpen: () => void
  onAdd: () => void
}) {
  return (
    <div className="flex items-center gap-3 rounded-[10px] px-2 py-2.5 transition-colors hover:bg-surface">
      <button type="button" onClick={onOpen} className="flex min-w-0 flex-1 cursor-pointer items-center gap-3.5 text-left">
        <PluginAvatar plugin={plugin} size="md" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-[13.5px] font-medium text-ink">{plugin.display_name}</p>
          {plugin.description ? (
            <p className="mt-0.5 truncate text-[12.5px] text-ink-2">{plugin.description}</p>
          ) : null}
        </div>
      </button>
      <IconButton
        variant="ghost"
        size="sm"
        aria-label={plugin.auth ? `Connect ${plugin.display_name}` : `Install ${plugin.display_name}`}
        title={plugin.auth ? `Connect ${plugin.display_name}` : `Install ${plugin.display_name}`}
        disabled={busy}
        onClick={onAdd}
      >
        {busy ? <Loader2 size={15} className="animate-spin" /> : <Plus size={16} />}
      </IconButton>
    </div>
  )
}

function isActivePlugin(plugin: Plugin): boolean {
  if (plugin.auth) return plugin.auth.connected
  return plugin.installed && plugin.origin !== 'builtin'
}

function groupCatalogPlugins(plugins: Plugin[]): { label: string; plugins: Plugin[] }[] {
  const popular: Plugin[] = []
  const byCategory = new Map<string, Plugin[]>()

  for (const plugin of plugins) {
    if (plugin.origin === 'builtin') {
      popular.push(plugin)
      continue
    }
    const label = categoryLabel(plugin.category)
    const bucket = byCategory.get(label)
    if (bucket) bucket.push(plugin)
    else byCategory.set(label, [plugin])
  }

  const groups: { label: string; plugins: Plugin[] }[] = []
  if (popular.length > 0) {
    groups.push({ label: 'Popular', plugins: popular })
  }
  const preferredOrder = ['Meetings', 'Productivity']
  for (const label of preferredOrder) {
    const items = byCategory.get(label)
    if (items?.length) {
      groups.push({ label, plugins: items })
      byCategory.delete(label)
    }
  }
  const rest = Array.from(byCategory.entries()).sort(([a], [b]) => a.localeCompare(b))
  for (const [label, items] of rest) {
    groups.push({ label, plugins: items })
  }
  return groups
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
