import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, CheckCircle2, ChevronDown, Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { ProviderLogo } from '@/components/settings/ProviderLogo'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { SettingsSection } from '@/components/settings/agentSettingsShell'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { modelProviderRequiresKey } from '@/lib/agentRuntimes'
import { createProvider, deleteProvider, getProviderStatuses, updateProvider } from '@/lib/api/providers'
import { agentSettingsQuery, updateAgentSettings } from '@/lib/api/settings'
import type { AgentSettings as AgentSettingsData, ProviderInput } from '@/lib/api/types'
import { isLocalBackendUrl, useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'

const PROVIDER_CAPABILITIES = [
  { value: 'chat_completions', label: 'Chat Completions' },
  { value: 'responses', label: 'Responses' },
]

type ProviderOption = AgentSettingsData['providers'][number]
type ProviderConnection = 'connected' | 'disconnected' | 'checking'
type ProviderDraft = ProviderInput & { id?: string }
type ProviderKeyDraft = { provider: ProviderOption; apiKey: string }

function prettyEndpoint(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function endpointRequiresKey(raw: string): boolean {
  const value = raw.trim()
  if (!value) return true
  try {
    const parsed = new URL(/^https?:\/\//i.test(value) ? value : `http://${value}`)
    const host = parsed.hostname.toLowerCase()
    return host !== 'localhost' && host !== '::1' && host !== '[::1]' && !host.startsWith('127.')
  } catch {
    return true
  }
}

function draftWithEndpoint(draft: ProviderDraft, baseUrl: string): ProviderDraft {
  const next = { ...draft, base_url: baseUrl }
  return endpointRequiresKey(baseUrl) ? next : { ...next, api_key: '' }
}

function emptyProviderDraft(): ProviderDraft {
  return {
    label: '',
    base_url: '',
    capabilities: ['chat_completions'],
    default_model: '',
    icon: '',
    api_key: '',
  }
}

function draftFromProvider(provider: ProviderOption): ProviderDraft {
  return {
    id: provider.id,
    label: provider.label,
    base_url: provider.base_url,
    capabilities: provider.capabilities?.length ? [...provider.capabilities] : ['chat_completions'],
    default_model: provider.default_model ?? '',
    icon: provider.icon ?? '',
    api_key: '',
  }
}

export function AgentProvidersSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const remote = !isLocalBackendUrl(useConnection().url)
  const settings = useQuery(agentSettingsQuery)
  const [providerDraft, setProviderDraft] = useState<ProviderDraft | null>(null)
  const [keyDraft, setKeyDraft] = useState<ProviderKeyDraft | null>(null)
  const providerStatuses = useQuery({
    queryKey: keys.providerStatuses,
    queryFn: getProviderStatuses,
  })
  const statusByProvider = useMemo(
    () =>
      new Map(
        (providerStatuses.data?.providers ?? []).map((provider) => [
          provider.id,
          provider.connection_status,
        ]),
      ),
    [providerStatuses.data],
  )

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.agentSettings })
    queryClient.invalidateQueries({ queryKey: keys.providerStatuses })
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
  const saveKey = useMutation({
    mutationFn: ({ settings, draft }: { settings: AgentSettingsData; draft: ProviderKeyDraft }) =>
      updateAgentSettings(settings, { [draft.provider.id]: draft.apiKey }),
    onSuccess: (_, { draft }) => {
      toast(`Saved ${draft.provider.label}`)
      setKeyDraft(null)
    },
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
  const openKeyEdit = (provider: ProviderOption) => {
    saveKey.reset()
    setKeyDraft({ provider, apiKey: '' })
  }
  const closeEditor = () => {
    upsert.reset()
    setProviderDraft(null)
  }
  const closeKeyEditor = () => {
    saveKey.reset()
    setKeyDraft(null)
  }

  const providers = settings.data?.providers ?? []

  return (
    <>
      <SettingsSection
        title="Model Providers"
        description="Configure model providers once. Provider-backed ACP agents can reuse them."
      >
        {settings.isError ? (
          <p className="py-2 text-[13px] text-danger">{settings.error.message}</p>
        ) : settings.isPending ? (
          <SkeletonRows count={3} />
        ) : (
          <div className="flex flex-col gap-1.5">
            {providers.map((provider) => {
              const status = statusByProvider.get(provider.id)
              const providerWithStatus = status ? { ...provider, connection_status: status } : provider
              return (
                <ProviderRow
                  key={provider.id}
                  provider={providerWithStatus}
                  statusPending={providerStatuses.isPending && !status}
                  disabled={upsert.isPending || remove.isPending || saveKey.isPending}
                  onEdit={
                    provider.custom
                      ? () => openEdit(provider)
                      : modelProviderRequiresKey(provider)
                        ? () => openKeyEdit(provider)
                        : undefined
                  }
                  onDelete={
                    provider.custom
                      ? () => {
                          if (window.confirm(`Remove ${provider.label}?`)) remove.mutate(provider.id)
                        }
                      : undefined
                  }
                />
              )
            })}
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
      <ProviderKeyEditorModal
        draft={keyDraft}
        remote={remote}
        saving={saveKey.isPending}
        error={saveKey.isError ? saveKey.error.message : ''}
        onChange={setKeyDraft}
        onClose={closeKeyEditor}
        onSave={() =>
          settings.data &&
          keyDraft &&
          saveKey.mutate({ settings: settings.data, draft: keyDraft })
        }
      />
    </>
  )
}

function ProviderRow({
  provider,
  statusPending,
  disabled,
  onEdit,
  onDelete,
}: {
  provider: ProviderOption
  statusPending: boolean
  disabled: boolean
  onEdit?: () => void
  onDelete?: () => void
}) {
  const needsKey = modelProviderRequiresKey(provider)
  const connected =
    provider.connection_status === 'connected' ||
    (!provider.connection_status && needsKey && Boolean(provider.configured))
  const state: ProviderConnection = connected
    ? 'connected'
    : !needsKey && statusPending
      ? 'checking'
      : 'disconnected'
  const [expanded, setExpanded] = useState(false)
  const hasActions = Boolean(onEdit || onDelete)

  return (
    <SettingsCard className="overflow-hidden">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        className="flex w-full items-center gap-2.5 px-3 py-2 text-left outline-none transition-[background-color] duration-150 hover:bg-surface-2/50 focus-visible:bg-surface-2/70 focus-visible:outline-none"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-bg text-ink">
          <ProviderLogo provider={provider.icon || provider.id} />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-[13.5px] font-medium text-ink">{provider.label}</span>
            {state === 'connected' ? null : <ProviderPill state={state} />}
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

      <div
        aria-hidden={!expanded}
        inert={expanded ? undefined : true}
        className={`grid transition-[grid-template-rows,opacity] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] ${
          expanded ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
        }`}
      >
        <div className="min-h-0 overflow-hidden">
          <div className="flex flex-col gap-3 px-3 pb-3 pt-0.5">
            {!needsKey ? (
              <p className="text-pretty text-[12px] text-ink-3">No API key required.</p>
            ) : null}

            {hasActions ? (
              <div className="flex items-center gap-1">
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
        </div>
      </div>
    </SettingsCard>
  )
}

function ProviderPill({ state }: { state: ProviderConnection }) {
  const tone = {
    connected: 'bg-primary-soft text-primary-strong',
    disconnected: 'bg-accent-soft text-accent-strong',
    checking: 'bg-surface-2 text-ink-2',
  }[state]
  const text = {
    connected: 'Connected',
    disconnected: 'Not connected',
    checking: 'Checking',
  }[state]
  return (
    <span
      className={`inline-flex shrink-0 items-center rounded-full px-2 py-[3px] text-[11px] font-medium ${tone}`}
    >
      {text}
    </span>
  )
}

function ProviderKeyEditorModal({
  draft,
  remote,
  saving,
  error,
  onChange,
  onClose,
  onSave,
}: {
  draft: ProviderKeyDraft | null
  remote: boolean
  saving: boolean
  error: string
  onChange: (draft: ProviderKeyDraft) => void
  onClose: () => void
  onSave: () => void
}) {
  return (
    <Modal
      open={draft !== null}
      onClose={onClose}
      size="sm"
      title={draft ? `Edit ${draft.provider.label}` : 'Edit provider'}
      description={
        draft?.provider.configured
          ? 'Paste a new API key to replace the configured key.'
          : 'Add the API key used by this provider.'
      }
      footer={
        <>
          <p className="min-w-0 truncate text-[12px] text-danger" role="alert">
            {error}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={onClose}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="md"
              disabled={!draft?.apiKey.trim() || saving}
              onClick={onSave}
            >
              {saving ? 'Saving…' : 'Save changes'}
            </Button>
          </div>
        </>
      }
    >
      {draft ? (
        <ProviderField
          label="API key"
          hint={remote ? 'API keys can only be added from the machine running jaz.' : undefined}
        >
          <Input
            type="password"
            value={draft.apiKey}
            onChange={(event) => onChange({ ...draft, apiKey: event.target.value })}
            placeholder={draft.provider.configured ? 'Paste a new API key' : 'Paste an API key'}
            autoComplete="off"
            spellCheck={false}
            className="font-mono text-[12px]"
            aria-label={`${draft.provider.label} API key`}
          />
        </ProviderField>
      ) : null}
    </Modal>
  )
}

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
  const needsKey = !draft || endpointRequiresKey(draft.base_url)
  return (
    <Modal
      open={draft !== null}
      onClose={onClose}
      size="md"
      title={isEdit ? 'Edit provider' : 'Add a provider'}
      description={
        isEdit
          ? 'Update the provider details and API support.'
          : needsKey
            ? 'Connect any OpenAI-compatible endpoint with your own API key.'
            : 'Connect a local OpenAI-compatible endpoint.'
      }
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
              onChange={(event) => onChange(draftWithEndpoint(draft, event.target.value))}
              placeholder="https://api.groq.com/openai/v1"
              autoComplete="off"
              spellCheck={false}
              className="font-mono text-[12px]"
              aria-label="Endpoint URL"
            />
          </ProviderField>
          <ProviderField
            label="API support"
            hint="Enable only protocols implemented by this endpoint. At least one is required."
          >
            <CapabilityPicker
              value={draft.capabilities}
              onChange={(capabilities) => onChange({ ...draft, capabilities })}
            />
          </ProviderField>
          {needsKey ? (
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
          ) : null}
          <ProviderField label="Icon">
            <IconPicker value={draft.icon ?? ''} onChange={(icon) => onChange({ ...draft, icon })} />
          </ProviderField>
        </div>
      ) : null}
    </Modal>
  )
}

function CapabilityPicker({ value, onChange }: { value: string[]; onChange: (value: string[]) => void }) {
  return (
    <div role="group" aria-label="API support" className="grid grid-cols-2 gap-2">
      {PROVIDER_CAPABILITIES.map((capability) => {
        const checked = value.includes(capability.value)
        const required = checked && value.length === 1
        return (
          <button
            key={capability.value}
            type="button"
            role="checkbox"
            aria-checked={checked}
            disabled={required}
            onClick={() =>
              onChange(
                checked
                  ? value.filter((item) => item !== capability.value)
                  : [...value, capability.value],
              )
            }
            className={`flex min-h-10 min-w-0 items-center gap-2 rounded-control px-3 text-left text-[13px] outline-none transition-[background-color,box-shadow,scale] duration-150 active:scale-[0.96] focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-default ${
              checked
                ? 'bg-primary-soft text-ink ring-1 ring-primary/40'
                : 'bg-bg text-ink-2 ring-1 ring-border hover:bg-surface-2 hover:text-ink'
            }`}
          >
            <span
              aria-hidden
              className={`grid size-4 shrink-0 place-items-center rounded-[5px] transition-[background-color,box-shadow] duration-150 ${
                checked
                  ? 'bg-primary text-on-primary'
                  : 'bg-transparent text-transparent ring-1 ring-border'
              }`}
            >
              <Check
                size={11}
                strokeWidth={3}
                className={`transition-[opacity,scale,filter] duration-150 ease-[cubic-bezier(0.2,0,0,1)] ${
                  checked
                    ? 'scale-100 opacity-100 blur-0'
                    : 'scale-[0.25] opacity-0 blur-[4px]'
                }`}
              />
            </span>
            <span className="truncate">{capability.label}</span>
          </button>
        )
      })}
    </div>
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
