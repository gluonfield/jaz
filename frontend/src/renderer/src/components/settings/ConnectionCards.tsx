import { ChevronRight, Loader2, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { Switch } from '@/components/ui/Switch'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { SettingsCard } from './SettingsCard'
import { accountAddress, accountSyncLabel } from './connectionFormatting'
import { PluginIcon } from './ConnectionPluginVisuals'

export function ConnectionSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section>
      <p className="mb-2 text-[12px] font-medium text-ink-2">{title}</p>
      <div className="grid grid-cols-1 gap-2">{children}</div>
    </section>
  )
}

export function ExistingConnectionCard({
  plugin,
  account,
  disconnecting,
  updatingScopes,
  onScopesChange,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  account: IntegrationConnectionAccount
  disconnecting: boolean
  updatingScopes?: boolean
  onScopesChange?: (scopes: string[]) => void
  onDisconnect: () => void
}) {
  const address = accountAddress(account)
  const sync = accountSyncLabel(account)
  const chatAccount = plugin.id === 'whatsapp' || plugin.id === 'telegram'

  return (
    <SettingsCard className="grid h-full grid-cols-1 gap-3 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start">
      <ConnectionSummary
        plugin={plugin}
        title={plugin.name}
        detail={address || account.id}
        meta={sync}
      />
      <div className="grid gap-3 sm:justify-items-end">
        {chatAccount && onScopesChange ? (
          <ChatAccessControls
            account={account}
            disabled={updatingScopes || disconnecting}
            onChange={onScopesChange}
          />
        ) : null}
        <Button
          variant="danger"
          size="sm"
          disabled={disconnecting}
          onClick={onDisconnect}
          className="sm:justify-self-end"
        >
          {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
          {disconnecting ? 'Disconnecting' : 'Disconnect'}
        </Button>
      </div>
    </SettingsCard>
  )
}

function ChatAccessControls({
  account,
  disabled,
  onChange,
}: {
  account: IntegrationConnectionAccount
  disabled?: boolean
  onChange: (scopes: string[]) => void
}) {
  const scopes = normalizedScopes(account.scopes)
  const setScope = (scope: string, checked: boolean) => {
    const next = new Set(scopes)
    if (checked) next.add(scope)
    else next.delete(scope)
    if (scope === 'messages' && checked) next.add('contacts')
    onChange([...next])
  }

  return (
    <div className="flex flex-wrap items-center justify-start gap-x-4 gap-y-2 sm:justify-end">
      <AccessToggle
        label="Read"
        checked={scopes.has('messages')}
        disabled={disabled}
        onChange={(checked) => setScope('messages', checked)}
      />
      <AccessToggle
        label="Send"
        checked={scopes.has('send')}
        disabled={disabled}
        onChange={(checked) => setScope('send', checked)}
      />
    </div>
  )
}

function AccessToggle({
  label,
  checked,
  disabled,
  onChange,
}: {
  label: string
  checked: boolean
  disabled?: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label className="flex items-center gap-2 text-[12px] text-ink-2">
      <span>{label}</span>
      <Switch checked={checked} disabled={disabled} onChange={onChange} aria-label={label} />
    </label>
  )
}

function normalizedScopes(scopes?: string[]): Set<string> {
  return new Set(scopes ?? [])
}

export function ConnectionPluginCard({
  plugin,
  onOpen,
}: {
  plugin: IntegrationPlugin
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group grid h-full w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-card bg-surface px-3 py-3 text-left transition-colors duration-150 hover:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
    >
      <ConnectionSummary plugin={plugin} title={plugin.name} detail={plugin.description} />
      <ChevronRight
        size={14}
        className="shrink-0 text-ink-3 transition-transform duration-150 group-hover:translate-x-0.5 group-hover:text-ink-2"
      />
    </button>
  )
}

function ConnectionSummary({
  plugin,
  title,
  detail,
  meta,
}: {
  plugin: IntegrationPlugin
  title: string
  detail?: string
  meta?: string
}) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <PluginIcon plugin={plugin} compact />
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-ink" title={title}>
          {title}
        </p>
        {detail ? (
          <p className="mt-0.5 line-clamp-2 text-[12px] leading-5 text-ink-2" title={detail}>
            {detail}
          </p>
        ) : null}
        {meta ? (
          <p className="mt-0.5 truncate text-[11px] leading-4 text-ink-3" title={meta}>
            {meta}
          </p>
        ) : null}
      </div>
    </div>
  )
}
