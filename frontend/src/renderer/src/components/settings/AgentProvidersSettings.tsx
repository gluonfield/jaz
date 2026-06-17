import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ChevronDown, ExternalLink, Pencil, Plus, Trash2 } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import type { ReactNode } from 'react'
import { ProviderLogo } from '@/components/settings/ProviderLogo'
import { SettingsSection, useAgentSettingsDraft } from '@/components/settings/agentSettingsShell'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { providerHidden, providerKeyUrl } from '@/lib/agentRuntimes'
import { createProvider, deleteProvider, updateProvider } from '@/lib/api/providers'
import type { AgentSettings as AgentSettingsData, ProviderInput } from '@/lib/api/types'
import { isLoopbackUrl, useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'

const EASE = [0.22, 1, 0.36, 1] as const

const PROVIDER_API_TYPES = [{ value: 'openai-compatible', label: 'OpenAI-compatible' }]

type ProviderOption = AgentSettingsData['providers'][number]
type ProviderConnection = 'connected' | 'disconnected' | 'no-key'
type ProviderDraft = ProviderInput & { id?: string }

function prettyEndpoint(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function emptyProviderDraft(): ProviderDraft {
  return { label: '', base_url: '', api_type: 'openai-compatible', default_model: '', icon: '', api_key: '' }
}

function draftFromProvider(provider: ProviderOption): ProviderDraft {
  return {
    id: provider.id,
    label: provider.label,
    base_url: provider.base_url,
    api_type: provider.api_type || 'openai-compatible',
    default_model: provider.default_model ?? '',
    icon: provider.icon ?? '',
    api_key: '',
  }
}

export function AgentProvidersSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const remote = !isLoopbackUrl(useConnection().url)
  const { settings, draft, providerKeys, setProviderKeys, save, providerKeyDirty } =
    useAgentSettingsDraft('providers')
  const [providerDraft, setProviderDraft] = useState<ProviderDraft | null>(null)

  // Custom-provider create/edit/delete refresh the provider list (which rides
  // inside the agent-settings query) plus anything derived from it.
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.agentSettings })
    queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    queryClient.invalidateQueries({ queryKey: keys.onboarding })
  }
  const upsert = useMutation({
    mutationFn: (input: ProviderDraft) =>
      input.id ? updateProvider(input.id, input) : createProvider(input),
    onSuccess: (provider) => {
      toast(`Saved ${provider.label}`)
      setProviderDraft(null)
    },
    onSettled: invalidate,
  })
  const remove = useMutation({
    mutationFn: deleteProvider,
    onSuccess: () => toast('Removed provider'),
    onError: (error: Error) => toast(`Couldn't remove provider: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })

  const openCreate = () => {
    upsert.reset()
    setProviderDraft(emptyProviderDraft())
  }
  const openEdit = (provider: ProviderOption) => {
    upsert.reset()
    setProviderDraft(draftFromProvider(provider))
  }
  const closeEditor = () => {
    upsert.reset()
    setProviderDraft(null)
  }

  const providers = (draft?.providers ?? []).filter((provider) => !providerHidden(provider.id))
  const canSave = draft != null && providerKeyDirty && !save.isPending

  return (
    <>
      <SettingsSection
        title="Model Providers"
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
                remote={remote}
                onKeyChange={(value) => setProviderKeys({ ...providerKeys, [provider.id]: value })}
                onEdit={provider.custom ? () => openEdit(provider) : undefined}
                onDelete={
                  provider.custom
                    ? () => {
                        if (window.confirm(`Remove ${provider.label}?`)) remove.mutate(provider.id)
                      }
                    : undefined
                }
              />
            ))}
            <button
              type="button"
              onClick={openCreate}
              className="flex w-full items-center gap-2.5 rounded-[12px] border border-dashed border-border px-3 py-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:border-primary/50 hover:bg-surface hover:text-ink"
            >
              <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-surface-2 text-ink-3">
                <Plus size={16} />
              </span>
              Add a provider
            </button>
          </div>
        )}
      </SettingsSection>

      <ProviderEditorModal
        draft={providerDraft}
        remote={remote}
        saving={upsert.isPending}
        error={upsert.isError ? upsert.error.message : ''}
        onChange={setProviderDraft}
        onClose={closeEditor}
        onSave={() => providerDraft && upsert.mutate(providerDraft)}
      />
    </>
  )
}

// One row in the providers list: a collapsed header with the brand mark, a
// connection pill and a check, expanding to the key field. Custom providers get
// an edit/remove footer; built-ins are key-only.
function ProviderRow({
  provider,
  keyDraft,
  disabled,
  remote,
  onKeyChange,
  onEdit,
  onDelete,
}: {
  provider: ProviderOption
  keyDraft: string
  disabled: boolean
  remote: boolean
  onKeyChange: (value: string) => void
  onEdit?: () => void
  onDelete?: () => void
}) {
  const needsKey = Boolean(provider.api_key_env) && provider.requires_api_key !== false
  const connected = needsKey ? Boolean(provider.configured || keyDraft.trim()) : true
  const state: ProviderConnection = needsKey ? (connected ? 'connected' : 'disconnected') : 'no-key'
  const keyUrl = providerKeyUrl(provider.id)
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="overflow-hidden rounded-[12px] bg-surface">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
          <ProviderLogo provider={provider.icon || provider.id} />
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
                  {remote ? (
                    <p className="text-pretty text-[12px] text-ink-3">
                      API keys can only be added from the machine running jaz.
                    </p>
                  ) : null}
                </div>
              ) : (
                <p className="text-pretty text-[12px] text-ink-3">No API key required.</p>
              )}

              {onEdit || onDelete ? (
                <div className="flex items-center gap-1 border-t border-border/70 pt-3">
                  {onEdit ? (
                    <Button variant="ghost" size="sm" disabled={disabled} onClick={onEdit}>
                      <Pencil size={13} />
                      Edit
                    </Button>
                  ) : null}
                  {onDelete ? (
                    <Button variant="danger" size="sm" disabled={disabled} onClick={onDelete}>
                      <Trash2 size={13} />
                      Remove
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
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

// Create/edit a custom OpenAI-compatible provider. Built-ins never open this —
// their key is edited inline on the row.
function ProviderEditorModal({
  draft,
  remote,
  saving,
  error,
  onChange,
  onClose,
  onSave,
}: {
  draft: ProviderDraft | null
  remote: boolean
  saving: boolean
  error: string
  onChange: (draft: ProviderDraft) => void
  onClose: () => void
  onSave: () => void
}) {
  const isEdit = Boolean(draft?.id)
  const canSave = Boolean(draft && draft.label.trim() && draft.base_url.trim())
  return (
    <Modal
      open={draft !== null}
      onClose={onClose}
      size="md"
      title={isEdit ? 'Edit provider' : 'Add a provider'}
      description="Connect any OpenAI-compatible endpoint with your own API key."
      footer={
        <>
          <p className="min-w-0 truncate text-[12px] text-danger" role="alert">
            {error}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" size="md" disabled={!canSave || saving} onClick={onSave}>
              {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Add provider'}
            </Button>
          </div>
        </>
      }
    >
      {draft ? (
        <div className="flex flex-col gap-4">
          <ProviderField label="Name">
            <Input
              value={draft.label}
              onChange={(event) => onChange({ ...draft, label: event.target.value })}
              placeholder="Groq"
              aria-label="Provider name"
            />
          </ProviderField>
          <ProviderField label="Endpoint URL" hint="The base URL of the OpenAI-compatible API.">
            <Input
              value={draft.base_url}
              onChange={(event) => onChange({ ...draft, base_url: event.target.value })}
              placeholder="https://api.groq.com/openai/v1"
              autoComplete="off"
              spellCheck={false}
              className="font-mono text-[12px]"
              aria-label="Endpoint URL"
            />
          </ProviderField>
          <ProviderField label="API type">
            <Select
              value={draft.api_type || 'openai-compatible'}
              options={PROVIDER_API_TYPES}
              onChange={(api_type) => onChange({ ...draft, api_type })}
              aria-label="API type"
              className="w-full"
            />
          </ProviderField>
          <ProviderField
            label="API key"
            hint={remote ? 'API keys can only be added from the machine running jaz.' : undefined}
          >
            <Input
              type="password"
              value={draft.api_key ?? ''}
              onChange={(event) => onChange({ ...draft, api_key: event.target.value })}
              placeholder={isEdit ? 'Leave blank to keep the current key' : 'Paste an API key'}
              autoComplete="off"
              spellCheck={false}
              className="font-mono text-[12px]"
              aria-label="API key"
            />
          </ProviderField>
          <ProviderField label="Icon">
            <IconPicker value={draft.icon ?? ''} onChange={(icon) => onChange({ ...draft, icon })} />
          </ProviderField>
        </div>
      ) : null}
    </Modal>
  )
}

function ProviderField({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[13px] font-medium text-ink">{label}</span>
      {children}
      {hint ? <span className="text-[12px] text-ink-3">{hint}</span> : null}
    </div>
  )
}

function IconPicker({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const options = ['', 'openai', 'openrouter']
  return (
    <div className="flex items-center gap-1.5">
      {options.map((icon) => {
        const selected = value === icon
        return (
          <button
            key={icon || 'generic'}
            type="button"
            onClick={() => onChange(icon)}
            aria-label={icon || 'Generic'}
            className={`grid size-9 place-items-center rounded-[8px] bg-bg text-ink ring-1 ring-inset transition-colors duration-150 ${
              selected ? 'ring-primary' : 'ring-border hover:ring-ink/30'
            }`}
          >
            <ProviderLogo provider={icon || 'custom'} size={18} />
          </button>
        )
      })}
    </div>
  )
}
