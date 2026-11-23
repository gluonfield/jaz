import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDownToLine, Check, Copy, ExternalLink, Loader2, Plug, Plus, QrCode, RefreshCw, Trash2, Unplug } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import {
  savePluginConfig,
  type IntegrationConnectionAccount,
  type Plugin,
  type PluginConfigVariable,
} from '@/lib/api/plugins'
import { openExternal } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import {
  accountDetail,
  accountLabel,
  categoryLabel,
  pluginActionLabel,
  pluginCanConnect,
} from './pluginFormatting'
import { PluginAvatar } from './PluginVisuals'

export function PluginDetailModal({
  plugin,
  canManage = true,
  canToggleEnabled = false,
  togglingEnabled = false,
  connecting,
  disconnectingAccountID,
  installing,
  updating,
  removing,
  onClose,
  onConnect,
  onDisconnect,
  onToggleEnabled,
  onInstall,
  onUpdate,
  onRemove,
}: {
  plugin: Plugin | null
  canManage?: boolean
  canToggleEnabled?: boolean
  togglingEnabled?: boolean
  connecting: boolean
  disconnectingAccountID?: string
  installing: boolean
  updating: boolean
  removing: boolean
  onClose: () => void
  onConnect: (plugin: Plugin) => void
  onDisconnect: (accountID: string) => void
  onToggleEnabled?: (plugin: Plugin, enabled: boolean) => void
  onInstall: (plugin: Plugin) => void
  onUpdate: (plugin: Plugin) => void
  onRemove: (plugin: Plugin) => void
}) {
  if (!plugin) return null

  const accounts = plugin.auth?.accounts ?? []
  const hasAccounts = accounts.length > 0
  const hasConfig = (plugin.config_variables?.length ?? 0) > 0
  const hasPrompts = (plugin.suggested_prompts?.length ?? 0) > 0
  const hasSkills = (plugin.skills?.length ?? 0) > 0
  const hasTools = (plugin.tools?.length ?? 0) > 0
  const hasMCP = (plugin.mcp_servers?.length ?? 0) > 0
  const singleConnectedAccount =
    plugin.auth?.connected && !plugin.auth.multi_account && accounts.length === 1 ? accounts[0] : null
  const isRemoteMCPOnly = Boolean(plugin.auth && !hasSkills && !(plugin.views?.length ?? 0))
  const canRemove = canManage && plugin.installed && plugin.origin !== 'builtin' && !isRemoteMCPOnly

  return (
    <Modal open onClose={onClose} title={plugin.display_name} size="wide" chromeless>
      <div className="px-8 pb-7 pt-8">
        <div className="flex items-start justify-between gap-4">
          <PluginAvatar plugin={plugin} size="lg" />
          <div className="flex items-center gap-2">
            {canManage && !canToggleEnabled && plugin.installed && plugin.source ? (
              <Button variant="secondary" size="sm" disabled={updating || removing} onClick={() => onUpdate(plugin)}>
                <RefreshCw size={13} className={updating ? 'animate-spin' : undefined} />
                {updating ? 'Updating…' : plugin.source.update_available ? 'Update' : 'Check update'}
              </Button>
            ) : null}
            {canRemove ? (
              <Button variant="ghost" size="sm" disabled={removing || updating} onClick={() => onRemove(plugin)}>
                <Trash2 size={13} />
                Remove
              </Button>
            ) : null}
          </div>
        </div>

        <div className="mt-5 flex items-start justify-between gap-5">
          <div className="min-w-0">
            <div className="flex flex-wrap items-baseline gap-2">
              <h2 className="truncate text-2xl font-semibold leading-tight text-ink">{plugin.display_name}</h2>
              {plugin.version ? (
                <span className="text-[12px] tabular-nums text-ink-3">v{plugin.version}</span>
              ) : null}
            </div>
            {plugin.description ? (
              <p className="mt-1.5 max-w-[58ch] text-[14px] leading-6 text-ink-2">{plugin.description}</p>
            ) : null}
            {plugin.auth?.reason ? (
              <p className="mt-1.5 max-w-[58ch] text-[13px] leading-5 text-ink-3">{plugin.auth.reason}</p>
            ) : null}
          </div>

          {singleConnectedAccount ? (
            <Button
              variant="secondary"
              size="lg"
              disabled={disconnectingAccountID === singleConnectedAccount.id}
              onClick={() => onDisconnect(singleConnectedAccount.id)}
              className="shrink-0"
            >
              {disconnectingAccountID === singleConnectedAccount.id ? <Loader2 size={14} className="animate-spin" /> : <Unplug size={14} />}
              {disconnectingAccountID === singleConnectedAccount.id ? 'Disconnecting…' : 'Disconnect'}
            </Button>
          ) : plugin.auth ? (
            <ConnectButton plugin={plugin} connecting={connecting} onConnect={() => onConnect(plugin)} />
          ) : canToggleEnabled && plugin.installed && onToggleEnabled ? (
            <div className="flex shrink-0 items-center gap-2 pt-1 text-[13px] text-ink-2">
              <Switch
                checked={plugin.enabled}
                disabled={togglingEnabled}
                aria-label={`Enable ${plugin.display_name}`}
                onChange={(next) => onToggleEnabled(plugin, next)}
              />
              <span>{plugin.enabled ? 'Enabled' : 'Disabled'}</span>
            </div>
          ) : canManage && !plugin.installed ? (
            <Button variant="primary" size="lg" disabled={installing} onClick={() => onInstall(plugin)} className="shrink-0">
              {installing ? <Loader2 size={14} className="animate-spin" /> : <ArrowDownToLine size={14} />}
              {installing ? 'Installing…' : 'Install'}
            </Button>
          ) : null}
        </div>
      </div>

      <div className="space-y-8 px-8 pb-9">
        {hasAccounts ? (
          <AccountsSection
            accounts={accounts}
            disconnectingAccountID={disconnectingAccountID}
            onDisconnect={onDisconnect}
          />
        ) : null}
        {hasConfig ? <ConfigSection key={plugin.id} plugin={plugin} variables={plugin.config_variables!} /> : null}
        {hasPrompts ? <SuggestedPromptsSection prompts={plugin.suggested_prompts!} /> : null}
        {hasSkills ? (
          <section className="space-y-2.5">
            <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">Skills</h3>
            <div className="divide-y divide-border/60 rounded-card bg-surface">
              {plugin.skills!.map((skill) => (
                <div key={skill.name} className="px-3.5 py-2.5">
                  <p className="font-mono text-[12.5px] font-medium text-ink">{skill.name}</p>
                  {skill.description ? <p className="mt-0.5 text-[12px] text-ink-2">{skill.description}</p> : null}
                </div>
              ))}
            </div>
          </section>
        ) : null}
        {hasMCP ? (
          <section className="space-y-2.5">
            <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">MCP servers</h3>
            <div className="flex flex-wrap gap-2">
              {plugin.mcp_servers!.map((srv) => (
                <span key={srv.name} className="inline-flex items-center gap-1.5 rounded-full bg-surface px-3 py-1 text-[12px] text-ink">
                  <Plug size={12} className="text-ink-3" />
                  {srv.name}
                  <span className="text-ink-3">({srv.transport})</span>
                </span>
              ))}
            </div>
          </section>
        ) : null}
        {hasTools ? (
          <section className="space-y-2.5">
            <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">Tools</h3>
            <div className="divide-y divide-border/60 rounded-card bg-surface">
              {plugin.tools!.map((tool) => (
                <div key={tool.name} className="px-3.5 py-2.5">
                  <p className="font-mono text-[12.5px] font-medium text-ink">{tool.name}</p>
                  {tool.description ? <p className="mt-0.5 text-[12px] text-ink-2">{tool.description}</p> : null}
                </div>
              ))}
            </div>
          </section>
        ) : null}
        {plugin.warnings?.length ? (
          <section className="space-y-1.5 rounded-card bg-surface px-3.5 py-3">
            <h3 className="text-[12px] font-medium text-ink-2">Warnings</h3>
            {plugin.warnings.map((warning) => (
              <p key={warning} className="text-[12px] text-ink-3">{warning}</p>
            ))}
          </section>
        ) : null}
        <InformationSection plugin={plugin} />
      </div>
    </Modal>
  )
}

function ConnectButton({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: Plugin
  connecting: boolean
  onConnect: () => void
}) {
  const sessionAuth = plugin.auth?.kind === 'session'
  const available = pluginCanConnect(plugin)
  const connected = Boolean(plugin.auth?.connected)
  let Icon = sessionAuth ? QrCode : Plug
  if (available && connected && plugin.auth?.multi_account) Icon = Plus
  if (connecting) Icon = Loader2

  return (
    <Button
      variant="primary"
      size="lg"
      disabled={!available || connecting}
      onClick={onConnect}
      className="shrink-0"
    >
      <Icon size={14} className={connecting ? 'animate-spin' : undefined} />
      {pluginActionLabel(plugin, connecting)}
    </Button>
  )
}

function AccountsSection({
  accounts,
  disconnectingAccountID,
  onDisconnect,
}: {
  accounts: IntegrationConnectionAccount[]
  disconnectingAccountID?: string
  onDisconnect: (accountID: string) => void
}) {
  return (
    <section className="space-y-2.5">
      <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">Connected accounts</h3>
      <div className="divide-y divide-border/60 rounded-card bg-surface">
        {accounts.map((account) => {
          const detail = accountDetail(account)
          return (
            <div key={account.id} className="flex items-center justify-between gap-3 px-3.5 py-2.5">
              <div className="min-w-0">
                <p className="truncate text-[13px] font-medium text-ink">{accountLabel(account)}</p>
                {detail ? <p className="truncate text-[12px] text-ink-3">{detail}</p> : null}
              </div>
              <Button
                variant="ghost"
                size="sm"
                disabled={disconnectingAccountID === account.id}
                onClick={() => onDisconnect(account.id)}
              >
                {disconnectingAccountID === account.id ? 'Disconnecting…' : 'Disconnect'}
              </Button>
            </div>
          )
        })}
      </div>
    </section>
  )
}

function ConfigSection({ plugin, variables }: { plugin: Plugin; variables: PluginConfigVariable[] }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [values, setValues] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: () => savePluginConfig(plugin.id, values),
    onSuccess: () => {
      setValues({})
      toast(`Saved ${plugin.display_name} settings`)
      void queryClient.invalidateQueries({ queryKey: keys.plugins })
      void queryClient.invalidateQueries({ queryKey: keys.mcpServers })
    },
    onError: (error: Error) => toast(`Couldn't save settings: ${error.message}`, 'danger'),
  })

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    save.mutate()
  }

  return (
    <section className="space-y-2.5">
      <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">Configuration</h3>
      <form className="space-y-3 rounded-card bg-surface p-3.5" onSubmit={onSubmit}>
        {variables.map((variable) => {
          const configured = Boolean(plugin.config_set?.[variable.name])
          return (
            <label key={variable.name} className="flex flex-col gap-1 text-[12px] text-ink-2">
              <span className="flex items-center gap-2 font-medium text-ink">
                <span className="font-mono">{variable.name}</span>
                {variable.required ? <span className="text-ink-3">Required</span> : null}
                {configured ? <span className="text-ok">Configured</span> : null}
              </span>
              {variable.description ? <span className="text-ink-3">{variable.description}</span> : null}
              <Input
                type={variable.secret ? 'password' : 'text'}
                value={values[variable.name] ?? ''}
                placeholder={configured ? 'Leave blank to keep saved value' : `Enter ${variable.name}`}
                autoComplete="off"
                onChange={(event) => setValues((prev) => ({ ...prev, [variable.name]: event.target.value }))}
              />
            </label>
          )
        })}
        <Button type="submit" variant="primary" size="sm" disabled={save.isPending || Object.keys(values).length === 0}>
          {save.isPending ? 'Saving…' : 'Save configuration'}
        </Button>
      </form>
    </section>
  )
}

function SuggestedPromptsSection({ prompts }: { prompts: string[] }) {
  const [copied, setCopied] = useState<string | null>(null)
  const copy = (prompt: string) => {
    void navigator.clipboard?.writeText(prompt)
    setCopied(prompt)
    window.setTimeout(() => setCopied((current) => (current === prompt ? null : current)), 1500)
  }

  return (
    <section className="space-y-2.5">
      <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">Try asking</h3>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {prompts.map((prompt) => (
          <button
            key={prompt}
            type="button"
            onClick={() => copy(prompt)}
            className="flex items-start justify-between gap-2 rounded-card bg-surface px-3.5 py-2.5 text-left text-[13px] text-ink-2 transition-colors hover:bg-surface-2 hover:text-ink"
          >
            <span className="line-clamp-2">{prompt}</span>
            {copied === prompt ? <Check size={13} className="mt-0.5 shrink-0 text-ok" /> : <Copy size={13} className="mt-0.5 shrink-0 text-ink-3" />}
          </button>
        ))}
      </div>
    </section>
  )
}

function InformationSection({ plugin }: { plugin: Plugin }) {
  const rows: { label: string; value: string; href?: string }[] = []
  if (plugin.author) rows.push({ label: 'Author', value: plugin.author, href: plugin.author_url })
  if (plugin.category) rows.push({ label: 'Category', value: categoryLabel(plugin.category) })
  rows.push({
    label: 'Source',
    value: plugin.origin === 'builtin'
      ? 'Built-in'
      : plugin.source
        ? `${plugin.source.repository}${plugin.source.ref ? `@${plugin.source.ref}` : ''}${plugin.source.commit ? ` (${plugin.source.commit.slice(0, 8)})` : ''}`
        : plugin.repository || plugin.origin,
  })
  if (plugin.homepage) rows.push({ label: 'Homepage', value: plugin.homepage, href: plugin.homepage })

  return (
    <section className="space-y-2.5">
      <h3 className="text-[12px] font-medium uppercase tracking-wide text-ink-3">Details</h3>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-2.5 rounded-card bg-surface px-4 py-3 text-[12.5px]">
        {rows.map((row) => (
          <div key={row.label} className="min-w-0">
            <dt className="text-ink-3">{row.label}</dt>
            <dd className="mt-0.5 truncate text-ink">
              {row.href ? (
                <button
                  type="button"
                  onClick={() => openExternal(row.href!)}
                  className="inline-flex max-w-full items-center gap-1 truncate text-ink hover:underline"
                >
                  <span className="truncate">{row.value}</span>
                  <ExternalLink size={11} className="shrink-0 text-ink-3" />
                </button>
              ) : (
                row.value
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
