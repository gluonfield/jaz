import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { type AddPluginMode, AddPluginModal } from '@/components/settings/AddPluginModal'
import { MarketplaceRow } from '@/components/settings/MarketplacesModal'
import { PluginAvatar } from '@/components/settings/PluginVisuals'
import { ActionMenu } from '@/components/ui/ActionMenu'
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
  const added = marketplaces.data?.filter((source) => !source.built_in) ?? []
  const managed = plugins.data?.filter((plugin) => !plugin.installable) ?? []
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
        title="Plugins"
        action={
          <ActionMenu
            label="Plugin actions"
            actions={[
              { label: 'Install from Git', onSelect: () => setAddMode('install') },
              { label: 'Add marketplace', onSelect: () => setAddMode('marketplace') },
              { label: 'Check for updates', onSelect: () => refresh.mutate() },
            ]}
          />
        }
      >
        <AdminList items={plugins.data && [...managed, ...available]} error={plugins.error} empty="No plugins available.">
          {(plugin) => (
            <PluginRow key={plugin.id} plugin={plugin}>
              {plugin.installable ? (
                <Button
                  size="sm"
                  disabled={install.isPending}
                  onClick={() => install.mutate({ source_id: plugin.source_id, plugin_id: plugin.id })}
                >
                  Install
                </Button>
              ) : (
                <>
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
                </>
              )}
            </PluginRow>
          )}
        </AdminList>
      </AdminSection>

      {added.length > 0 ? (
        <AdminSection title="Marketplaces">
          <AdminList items={added} error={null}>
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
      ) : null}

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
