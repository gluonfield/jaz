import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, RefreshCw, Trash2 } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { type AddPluginMode, AddPluginModal } from '@/components/settings/AddPluginModal'
import { MarketplaceRow } from '@/components/settings/MarketplacesModal'
import { PluginAvatar } from '@/components/settings/PluginVisuals'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Select } from '@/components/ui/Select'
import { useToast } from '@/components/ui/toast'
import {
  addOrgMarketplace,
  adminMarketplacesQuery,
  adminPluginsQuery,
  type AdminPlugin,
  type InstallationPolicy,
  installOrgPlugin,
  removeOrgMarketplace,
  setOrgPluginPolicy,
  uninstallOrgPlugin,
} from '@/lib/api/cloudOrg'
import { type PluginSource, refreshPluginSources } from '@/lib/api/plugins'
import { keys } from '@/lib/query/keys'
import { AdminList, AdminSection } from './AdminSection'

const POLICY_OPTIONS: { value: InstallationPolicy; label: string; description: string }[] = [
  { value: 'INSTALLED_BY_DEFAULT', label: 'On for everyone', description: 'Every member has it; each can turn it off.' },
  { value: 'AVAILABLE', label: 'Members opt in', description: 'Members turn it on for themselves.' },
  { value: 'DISABLED', label: 'Off for everyone', description: 'No member can use it.' },
]

const INTERRUPTS = 'Turns that are running now are interrupted.'

function interrupted(turns: number): string {
  if (turns === 0) return ''
  return ` · interrupted ${turns} running ${turns === 1 ? 'turn' : 'turns'}`
}

function PluginRow({ plugin, children }: { plugin: AdminPlugin; children: ReactNode }) {
  return (
    <div className="flex items-center gap-3 px-3 py-2.5">
      <PluginAvatar plugin={plugin} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13px] font-medium text-ink">{plugin.display_name}</p>
        {plugin.description ? <p className="truncate text-[12px] text-ink-3">{plugin.description}</p> : null}
      </div>
      {children}
    </div>
  )
}

export function OrgPluginsSection() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const marketplaces = useQuery(adminMarketplacesQuery)
  const plugins = useQuery(adminPluginsQuery)
  const [addMode, setAddMode] = useState<AddPluginMode | null>(null)

  const officialID = marketplaces.data?.find((source) => source.built_in)?.id
  const managed = plugins.data?.filter((plugin) => !plugin.installable)
  const available = plugins.data?.filter((plugin) => plugin.installable) ?? []

  const mutation = {
    onError: (error: Error) => toast(error.message, 'danger'),
    onSettled: () =>
      Promise.all(
        [keys.cloudAdminMarketplaces, keys.cloudAdminPlugins, keys.plugins].map((queryKey) =>
          queryClient.invalidateQueries({ queryKey }),
        ),
      ),
  }

  const refresh = useMutation({
    ...mutation,
    mutationFn: refreshPluginSources,
    onSuccess: () => toast('Marketplaces are up to date'),
  })

  const addMarketplace = useMutation({
    ...mutation,
    mutationFn: addOrgMarketplace,
    onSuccess: ({ marketplace }) => {
      setAddMode(null)
      toast(`Added ${marketplace.repository}`)
    },
  })

  const removeMarketplace = useMutation({
    ...mutation,
    mutationFn: (source: PluginSource) => removeOrgMarketplace(source.id),
    onSuccess: (result, source) => toast(`Removed ${source.repository}${interrupted(result.fenced_turns)}`),
  })

  const install = useMutation({
    ...mutation,
    mutationFn: installOrgPlugin,
    onSuccess: (result) => {
      setAddMode(null)
      toast(`Installed ${result.plugins.map((plugin) => plugin.display_name).join(', ')}`)
    },
  })

  const setPolicy = useMutation({
    ...mutation,
    mutationFn: ({ plugin, policy }: { plugin: AdminPlugin; policy: InstallationPolicy }) =>
      setOrgPluginPolicy(plugin.id, policy),
    onSuccess: (result, { plugin }) => toast(`Updated ${plugin.display_name}${interrupted(result.fenced_turns)}`),
  })

  const uninstall = useMutation({
    ...mutation,
    mutationFn: (plugin: AdminPlugin) => uninstallOrgPlugin(plugin.id),
    onSuccess: (result, plugin) => toast(`Uninstalled ${plugin.display_name}${interrupted(result.fenced_turns)}`),
  })

  const changePolicy = (plugin: AdminPlugin, policy: InstallationPolicy) => {
    if (policy === plugin.installation_policy) return
    if (
      policy === 'DISABLED' &&
      !window.confirm(`Turn off ${plugin.display_name} for every member? ${INTERRUPTS}`)
    ) {
      return
    }
    setPolicy.mutate({ plugin, policy })
  }

  return (
    <>
      <AdminSection
        title="Plugin marketplaces"
        description="Git repositories whose plugin catalogs are offered to this organization."
        action={
          <>
            <IconButton
              variant="ghost"
              size="sm"
              aria-label="Refresh marketplaces"
              title="Refresh marketplaces"
              disabled={refresh.isPending}
              onClick={() => refresh.mutate()}
            >
              <RefreshCw size={13} className={refresh.isPending ? 'animate-spin' : ''} />
            </IconButton>
            <Button size="sm" onClick={() => setAddMode('marketplace')}>
              <Plus size={13} />
              Add marketplace
            </Button>
          </>
        }
      >
        <AdminList items={marketplaces.data} error={marketplaces.error}>
          {(source) => (
            <MarketplaceRow
              key={source.id}
              source={source}
              removing={removeMarketplace.isPending}
              onRemove={() => {
                if (
                  window.confirm(
                    `Remove ${source.repository}? Its plugins are uninstalled for every member. ${INTERRUPTS}`,
                  )
                ) {
                  removeMarketplace.mutate(source)
                }
              }}
            />
          )}
        </AdminList>
      </AdminSection>

      <AdminSection
        title="Plugins"
        description="Plugins installed for the organization, and who gets them."
        action={
          <Button size="sm" onClick={() => setAddMode('install')}>
            <Plus size={13} />
            Install from Git
          </Button>
        }
      >
        <AdminList items={managed} error={plugins.error} empty="No plugins installed yet.">
          {(plugin) => (
            <PluginRow key={plugin.id} plugin={plugin}>
              <Select
                aria-label={`Who gets ${plugin.display_name}`}
                value={plugin.installation_policy}
                options={POLICY_OPTIONS}
                disabled={setPolicy.isPending}
                onChange={(policy) => changePolicy(plugin, policy as InstallationPolicy)}
              />
              {/* A plugin from a Git marketplace is removed with its marketplace. */}
              {!plugin.marketplace_id || plugin.marketplace_id === officialID ? (
                <IconButton
                  variant="danger"
                  size="sm"
                  aria-label={`Uninstall ${plugin.display_name}`}
                  title="Uninstall"
                  disabled={uninstall.isPending}
                  onClick={() => {
                    if (window.confirm(`Uninstall ${plugin.display_name} for every member? ${INTERRUPTS}`)) {
                      uninstall.mutate(plugin)
                    }
                  }}
                >
                  <Trash2 size={13} />
                </IconButton>
              ) : null}
            </PluginRow>
          )}
        </AdminList>
        {available.length > 0 ? (
          <>
            <p className="pt-1.5 text-[12px] font-medium text-ink-2">Available from the official marketplace</p>
            <AdminList items={available} error={null} className="max-h-[320px] overflow-y-auto">
              {(plugin) => (
                <PluginRow key={plugin.id} plugin={plugin}>
                  <Button
                    size="sm"
                    disabled={install.isPending}
                    onClick={() => install.mutate({ source_id: plugin.source_id, plugin_id: plugin.id })}
                  >
                    Install
                  </Button>
                </PluginRow>
              )}
            </AdminList>
          </>
        ) : null}
      </AdminSection>

      {addMode ? (
        <AddPluginModal
          open
          initialMode={addMode}
          busy={install.isPending || addMarketplace.isPending}
          onClose={() => setAddMode(null)}
          onInstall={install.mutate}
          onAddSource={addMarketplace.mutate}
        />
      ) : null}
    </>
  )
}
