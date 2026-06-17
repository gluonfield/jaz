import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, ExternalLink } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import type { ReactNode } from 'react'
import { ProviderLogo } from '@/components/settings/ProviderLogo'
import { SettingsSection, useAgentSettingsDraft } from '@/components/settings/agentSettingsShell'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { ModelCombobox } from '@/components/ui/ModelCombobox'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { nativeModelForProvider, providerKeyUrl } from '@/lib/agentRuntimes'
import type { AgentSettings as AgentSettingsData } from '@/lib/api/types'
import {
  type ModelSuggestion,
  modelSuggestionsForProvider,
  openRouterModelsQuery,
} from '@/lib/models'
import { settingsReasoningOptions } from '@/lib/reasoningEfforts'

const EASE = [0.22, 1, 0.36, 1] as const

type ProviderOption = AgentSettingsData['providers'][number]
type ProviderConnection = 'connected' | 'disconnected' | 'no-key'

// Drops the scheme so an endpoint reads as a compact host+path chip.
function prettyEndpoint(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

export function AgentProvidersSettings() {
  const { settings, draft, setDraft, providerKeys, setProviderKeys, save, dirty, providerKeyDirty } =
    useAgentSettingsDraft('providers')

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: draft?.native.model_provider === 'openrouter',
  })
  // Every known model provider — keys are shared, so this one list connects the
  // native agent and every ACP agent set to provider defaults. Implemented ones
  // (the native agent can run them) sort first; locals/customs follow.
  const allProviders = draft?.providers ?? []
  const nativeProviders = allProviders.filter((provider) => provider.implemented)
  const invalid = draft
    ? (draft.native.model_provider ?? '').trim() === '' || draft.native.model.trim() === ''
    : true
  const canSave = draft != null && !invalid && (dirty || providerKeyDirty) && !save.isPending

  const selectedProvider = draft?.native.model_provider ?? ''
  const selectedNativeProvider = nativeProviders.find((provider) => provider.id === selectedProvider)
  const nativeModelSuggestions = modelSuggestionsForProvider(
    selectedNativeProvider,
    openRouterModels.data ?? [],
  )

  const setNativeProvider = (model_provider: string) => {
    if (!draft) return
    const model = nativeModelForProvider(
      nativeProviders,
      draft.native.model_provider,
      model_provider,
      draft.native.model,
    )
    setDraft({ ...draft, native: { ...draft.native, model_provider, model } })
  }

  return (
    <SettingsSection
      title="Providers"
      description="Configure model providers once. ACP agents can reuse them when they are set to use provider defaults."
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
          {allProviders.map((provider) => {
            const isNativeDefault = provider.implemented && provider.id === selectedProvider
            return (
              <ProviderRow
                key={provider.id}
                provider={provider}
                keyDraft={providerKeys[provider.id] ?? ''}
                isNativeDefault={isNativeDefault}
                disabled={save.isPending}
                onKeyChange={(value) => setProviderKeys({ ...providerKeys, [provider.id]: value })}
              >
                {!provider.implemented ? (
                  <p className="text-pretty text-[12px] text-ink-3">
                    Available to ACP agents set to use this provider.
                  </p>
                ) : isNativeDefault ? (
                  <NativeDefaultEditor
                    model={draft.native.model}
                    reasoning={draft.native.reasoning_effort ?? ''}
                    suggestions={nativeModelSuggestions}
                    loading={openRouterModels.isLoading}
                    disabled={save.isPending}
                    onModelChange={(model) =>
                      setDraft({ ...draft, native: { ...draft.native, model } })
                    }
                    onReasoningChange={(reasoning_effort) =>
                      setDraft({ ...draft, native: { ...draft.native, reasoning_effort } })
                    }
                  />
                ) : (
                  <Button
                    variant="secondary"
                    size="md"
                    disabled={save.isPending}
                    onClick={() => setNativeProvider(provider.id)}
                    className="w-fit ring-1 ring-border ring-inset"
                  >
                    Use for native agent
                  </Button>
                )}
              </ProviderRow>
            )
          })}
        </div>
      )}
    </SettingsSection>
  )
}

// One row in the providers list: a collapsed header with the brand mark, a
// connection pill and a check, expanding to the key field plus a body slot. The
// slot carries the native-default model/reasoning editor (or the "use for native"
// affordance) so this row stays purely about connecting a provider. Mirrors the
// onboarding provider card so the connect-a-provider gesture reads the same.
function ProviderRow({
  provider,
  keyDraft,
  isNativeDefault,
  disabled,
  onKeyChange,
  children,
}: {
  provider: ProviderOption
  keyDraft: string
  isNativeDefault: boolean
  disabled: boolean
  onKeyChange: (value: string) => void
  children: ReactNode
}) {
  // A provider needs a key only if it has an env var to store one into — the
  // backend omits requires_api_key when false, so a missing api_key_env (Ollama)
  // is the reliable "no key" signal.
  const needsKey = Boolean(provider.api_key_env) && provider.requires_api_key !== false
  const connected = needsKey ? Boolean(provider.configured || keyDraft.trim()) : true
  const state: ProviderConnection = needsKey ? (connected ? 'connected' : 'disconnected') : 'no-key'
  const keyUrl = providerKeyUrl(provider.id)
  // The native default opens by default so its model/reasoning are one glance
  // away; the rest stay collapsed until tapped.
  const [expanded, setExpanded] = useState(isNativeDefault)

  return (
    <div className="overflow-hidden rounded-[12px] bg-surface">
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
            {isNativeDefault ? (
              <span className="inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium text-ink-2 ring-1 ring-border ring-inset">
                Native default
              </span>
            ) : null}
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
                        ? 'Configured — paste a new key to replace it'
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
                <p className="text-pretty text-[12px] text-ink-3">
                  Runs locally on your machine — no API key required.
                </p>
              )}

              {children}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

// The native agent's model + reasoning, shown only under the provider currently
// serving as the native default.
function NativeDefaultEditor({
  model,
  reasoning,
  suggestions,
  loading,
  disabled,
  onModelChange,
  onReasoningChange,
}: {
  model: string
  reasoning: string
  suggestions: ModelSuggestion[]
  loading: boolean
  disabled: boolean
  onModelChange: (value: string) => void
  onReasoningChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-3 rounded-[10px] bg-bg p-3">
      <p className="text-[11px] font-medium text-ink-3">Native agent default</p>
      <NativeDefaultField label="Model">
        <ModelCombobox
          value={model}
          suggestions={suggestions}
          loading={loading}
          disabled={disabled}
          onChange={onModelChange}
          aria-label="Native model"
          className="w-full sm:w-[230px]"
        />
      </NativeDefaultField>
      <NativeDefaultField label="Reasoning">
        <Select
          value={reasoning}
          options={settingsReasoningOptions()}
          disabled={disabled}
          onChange={onReasoningChange}
          aria-label="Native reasoning effort"
          className="w-full sm:w-[230px]"
        />
      </NativeDefaultField>
    </div>
  )
}

function NativeDefaultField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:justify-between">
      <span className="text-[13px] text-ink-2">{label}</span>
      {children}
    </div>
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
