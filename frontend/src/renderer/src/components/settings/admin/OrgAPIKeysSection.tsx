import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, Trash2 } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { AgentLogo } from '@/components/acp/AgentLogo'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Input } from '@/components/ui/Input'
import { useToast } from '@/components/ui/toast'
import {
  adminAPIKeysQuery,
  deleteOrgAPIKey,
  INTEGRATION_KEY_PREFIX,
  type OrgAPIKey,
  saveOrgAPIKey,
} from '@/lib/api/cloudOrg'
import { keys } from '@/lib/query/keys'
import { AdminList, AdminSection } from './AdminSection'

const NEW_INTEGRATION = 'new-integration'

function integrationKeyID(name: string): string {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return slug ? INTEGRATION_KEY_PREFIX + slug : ''
}

function keySummary(key: OrgAPIKey): string {
  if (!key.configured) return 'Not set'
  const by = key.updated_by?.display_name || key.updated_by?.email
  return [
    key.label,
    key.last4 ? `••••${key.last4}` : 'Key saved',
    key.updated_at ? `Updated ${new Date(key.updated_at).toLocaleDateString([], { dateStyle: 'medium' })}` : '',
    by ? `by ${by}` : '',
  ]
    .filter(Boolean)
    .join(' · ')
}

// The key is write-only: Jaz Cloud seals it on save and only ever returns the last four characters.
function KeyForm({
  target,
  busy,
  onSubmit,
  onCancel,
}: {
  target: OrgAPIKey | null
  busy: boolean
  onSubmit: (keyID: string, input: { api_key: string; label: string }) => void
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [apiKey, setAPIKey] = useState('')
  const [label, setLabel] = useState(target?.label ?? '')
  const keyID = target?.key_id ?? integrationKeyID(name)

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (keyID && apiKey.trim() && !busy) onSubmit(keyID, { api_key: apiKey.trim(), label: label.trim() })
  }

  return (
    <form onSubmit={submit} className="space-y-2.5 px-3 py-3">
      <div className="grid gap-2.5 sm:grid-cols-2">
        {target ? null : (
          <label className="flex flex-col gap-1 text-[12px] text-ink-2 sm:col-span-2">
            Integration name
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="e.g. Salesforce"
              autoComplete="off"
              autoFocus
            />
          </label>
        )}
        <label className="flex flex-col gap-1 text-[12px] text-ink-2">
          {target?.configured ? 'New API key' : 'API key'}
          <Input
            type="password"
            value={apiKey}
            onChange={(event) => setAPIKey(event.target.value)}
            autoComplete="off"
            autoFocus={Boolean(target)}
          />
        </label>
        <label className="flex flex-col gap-1 text-[12px] text-ink-2">
          Label (optional)
          <Input
            value={label}
            maxLength={80}
            onChange={(event) => setLabel(event.target.value)}
            placeholder="e.g. Production"
            autoComplete="off"
          />
        </label>
      </div>
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" size="sm" disabled={busy || !keyID || !apiKey.trim()}>
          {busy ? 'Saving…' : 'Save key'}
        </Button>
      </div>
    </form>
  )
}

export function OrgAPIKeysSection() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const fail = (error: Error) => toast(error.message, 'danger')
  const apiKeys = useQuery(adminAPIKeysQuery)
  const [editing, setEditing] = useState<string | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.cloudAdminAPIKeys })

  const save = useMutation({
    mutationFn: ({ keyID, input }: { keyID: string; input: { api_key: string; label: string } }) =>
      saveOrgAPIKey(keyID, input),
    onSuccess: (saved) => {
      setEditing(null)
      toast(`Saved the ${saved.display_name} key`)
    },
    onError: fail,
    onSettled: invalidate,
  })

  const remove = useMutation({
    mutationFn: (key: OrgAPIKey) => deleteOrgAPIKey(key.key_id),
    onSuccess: (_, key) => toast(`Deleted the ${key.display_name} key`),
    onError: fail,
    onSettled: invalidate,
  })

  const form = (target: OrgAPIKey | null) => (
    <KeyForm
      target={target}
      busy={save.isPending}
      onSubmit={(keyID, input) => save.mutate({ keyID, input })}
      onCancel={() => setEditing(null)}
    />
  )

  return (
    <AdminSection
      title="API keys"
      description="Keys for model providers and integrations, encrypted by Jaz Cloud and shared by every member."
      action={
        <Button size="sm" onClick={() => setEditing(NEW_INTEGRATION)}>
          <Plus size={13} />
          Add integration key
        </Button>
      }
    >
      {editing === NEW_INTEGRATION ? <SettingsCard>{form(null)}</SettingsCard> : null}
      <AdminList items={apiKeys.data} error={apiKeys.error}>
        {(key) => (
        <div key={key.key_id}>
          <div className="flex items-center gap-3 px-3 py-2.5">
            <span className="grid size-7 shrink-0 place-items-center text-ink-2">
              {key.kind === 'llm_provider' ? <AgentLogo agent={key.key_id} size={18} /> : <KeyRound size={16} />}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px] font-medium text-ink">{key.display_name}</p>
              <p className="truncate text-[12px] text-ink-3">{keySummary(key)}</p>
            </div>
            <Button size="sm" onClick={() => setEditing(key.key_id)}>
              {key.configured ? 'Rotate' : 'Add key'}
            </Button>
            {key.configured ? (
              <IconButton
                variant="danger"
                size="sm"
                aria-label={`Delete the ${key.display_name} key`}
                title="Delete key"
                disabled={remove.isPending}
                onClick={() => {
                  if (window.confirm(`Delete the ${key.display_name} key? Anything using it stops working for every member.`)) {
                    remove.mutate(key)
                  }
                }}
              >
                <Trash2 size={13} />
              </IconButton>
            ) : null}
          </div>
          {editing === key.key_id ? form(key) : null}
        </div>
        )}
      </AdminList>
    </AdminSection>
  )
}
