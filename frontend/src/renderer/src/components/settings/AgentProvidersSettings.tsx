import { CheckCircle2, ChevronDown, ExternalLink } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { ProviderLogo } from '@/components/settings/ProviderLogo'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { SettingsSection, useAgentSettingsDraft } from '@/components/settings/agentSettingsShell'
import { Input } from '@/components/ui/Input'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { providerKeyUrl } from '@/lib/agentRuntimes'
import type { AgentSettings as AgentSettingsData } from '@/lib/api/types'

const EASE = [0.22, 1, 0.36, 1] as const

type ProviderOption = AgentSettingsData['providers'][number]
type ProviderConnection = 'connected' | 'disconnected' | 'no-key'

function prettyEndpoint(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

export function AgentProvidersSettings() {
  const { settings, draft, providerKeys, setProviderKeys, save, providerKeyDirty } =
    useAgentSettingsDraft('providers')

  const providers = draft?.providers ?? []
  const canSave = draft != null && providerKeyDirty && !save.isPending

  return (
    <SettingsSection
      title="Providers"
      description="Configure model providers once. Provider-backed ACP agents can reuse them."
      canSave={canSave}
      saving={save.isPending}
      onSave={() => draft && save.mutate(draft)}
    >
      {settings.isError ? (
        <p className="py-2 text-[13px] text-danger">{settings.error.message}</p>
      ) : settings.isPending || !draft ? (
        <SkeletonRows count={3} />
      ) : (
        <div className="flex flex-col gap-1.5">
          {providers.map((provider) => (
            <ProviderRow
              key={provider.id}
              provider={provider}
              keyDraft={providerKeys[provider.id] ?? ''}
              disabled={save.isPending}
              onKeyChange={(value) => setProviderKeys({ ...providerKeys, [provider.id]: value })}
            />
          ))}
        </div>
      )}
    </SettingsSection>
  )
}

function ProviderRow({
  provider,
  keyDraft,
  disabled,
  onKeyChange,
}: {
  provider: ProviderOption
  keyDraft: string
  disabled: boolean
  onKeyChange: (value: string) => void
}) {
  const needsKey = Boolean(provider.api_key_env) && provider.requires_api_key !== false
  const connected = needsKey ? Boolean(provider.configured || keyDraft.trim()) : true
  const state: ProviderConnection = needsKey ? (connected ? 'connected' : 'disconnected') : 'no-key'
  const keyUrl = providerKeyUrl(provider.id)
  const [expanded, setExpanded] = useState(false)

  return (
    <SettingsCard className="overflow-hidden">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
          <ProviderLogo provider={provider.id} />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-[13.5px] font-medium text-ink">{provider.label}</span>
            <ProviderPill state={state} />
          </span>
          {provider.base_url ? (
            <span className="truncate font-mono text-[11px] text-ink-3">
              {prettyEndpoint(provider.base_url)}
            </span>
          ) : null}
        </span>
        {state === 'connected' ? <CheckCircle2 size={17} className="shrink-0 text-primary" /> : null}
        <ChevronDown
          size={15}
          className={`shrink-0 text-ink-3 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}
        />
      </button>

      <AnimatePresence initial={false}>
        {expanded ? (
          <motion.div
            key="body"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2, ease: EASE }}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-3 px-3 pb-3 pt-0.5">
              {needsKey ? (
                <div className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-[12px] font-medium text-ink-2">API key</span>
                    {provider.api_key_env ? (
                      <span className="font-mono text-[11px] text-ink-3">{provider.api_key_env}</span>
                    ) : null}
                  </div>
                  <Input
                    type="password"
                    value={keyDraft}
                    disabled={disabled}
                    onChange={(event) => onKeyChange(event.target.value)}
                    placeholder={
                      provider.configured
                        ? 'Configured - paste a new key to replace it'
                        : 'Paste an API key'
                    }
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono text-[12px]"
                    aria-label={`${provider.label} API key`}
                  />
                  {keyUrl ? (
                    <button
                      type="button"
                      onClick={() => window.open(keyUrl, '_blank', 'noopener,noreferrer')}
                      className="inline-flex w-fit items-center gap-1 text-[12px] text-primary transition-colors duration-150 hover:text-primary-strong"
                    >
                      Where do I find my {provider.label} key?
                      <ExternalLink size={12} />
                    </button>
                  ) : null}
                </div>
              ) : (
                <p className="text-pretty text-[12px] text-ink-3">No API key required.</p>
              )}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </SettingsCard>
  )
}

function ProviderPill({ state }: { state: ProviderConnection }) {
  const tone =
    state === 'connected'
      ? 'bg-primary-soft text-primary-strong'
      : state === 'no-key'
        ? 'bg-surface-2 text-ink-3'
        : 'bg-accent-soft text-accent-strong'
  const text =
    state === 'connected' ? 'Connected' : state === 'no-key' ? 'No key needed' : 'Not connected'
  return (
    <span
      className={`inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium ${tone}`}
    >
      {text}
    </span>
  )
}
