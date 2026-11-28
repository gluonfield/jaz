import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CreditCard, KeyRound, Plus, Trash2, UserRound } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { SearchField } from '@/components/ui/SearchField'
import { Segmented } from '@/components/ui/Segmented'
import { Skeleton } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  createVaultItem,
  deleteVaultItem,
  updateVaultItem,
  vaultItemsQuery,
  type VaultItem,
  type VaultItemInput,
  type VaultKind,
} from '@/lib/api/vault'
import { useCloudSession } from '@/lib/cloudSession'
import { activeCloudWorkspace } from '@/lib/cloudWorkspaceDisplay'
import { keys } from '@/lib/query/keys'
import { VaultItemModal } from './VaultItemModal'

type FilterKind = 'all' | VaultKind

const FILTERS: { value: FilterKind; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'login', label: 'Logins' },
  { value: 'card', label: 'Cards' },
  { value: 'identity', label: 'Identities' },
]

const KIND_ICON = { login: KeyRound, card: CreditCard, identity: UserRound }

function itemSubtitle(item: VaultItem): string {
  if (item.kind === 'login') {
    return [item.origins?.[0]?.replace(/^https:\/\//, ''), item.username, item.has_totp ? '2FA' : ''].filter(Boolean).join(' · ')
  }
  if (item.kind === 'card') {
    const expiry =
      item.card_exp_month && item.card_exp_year
        ? `Exp ${String(item.card_exp_month).padStart(2, '0')}/${String(item.card_exp_year).slice(-2)}`
        : ''
    return [item.card_brand, item.card_last4 ? `•••• ${item.card_last4}` : '', expiry].filter(Boolean).join(' · ')
  }
  return [item.email, item.city_country].filter(Boolean).join(' · ')
}

export function VaultSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const workspaceID = activeCloudWorkspace(cloud)?.id
  const itemsQuery = useQuery(vaultItemsQuery('', '', workspaceID))

  const [filter, setFilter] = useState<FilterKind>('all')
  const [search, setSearch] = useState('')
  // The item being edited, or 'new' while adding one.
  const [editing, setEditing] = useState<VaultItem | 'new' | null>(null)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.vaultRoot })

  const saveMutation = useMutation({
    mutationFn: (input: VaultItemInput & { kind: VaultKind }) =>
      editing && editing !== 'new' ? updateVaultItem(editing.id, input, workspaceID) : createVaultItem(input, workspaceID),
    onSuccess: () => {
      void invalidate()
      setEditing(null)
    },
  })
  const edit = (target: VaultItem | 'new') => {
    saveMutation.reset()
    setEditing(target)
  }

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteVaultItem(id, workspaceID),
    onSuccess: () => {
      void invalidate()
      setConfirmDeleteId(null)
    },
    onError: (err: Error) => toast(`Couldn't delete item: ${err.message}`, 'danger'),
  })

  const filtered = useMemo(() => {
    const items = itemsQuery.data ?? []
    const needle = search.trim().toLowerCase()
    return items.filter((item) => {
      if (filter !== 'all' && item.kind !== filter) return false
      if (!needle) return true
      const haystack = [
        item.label,
        ...(item.origins ?? []),
        item.username,
        item.card_brand,
        item.card_last4,
        item.cardholder_name,
        item.email,
        item.city_country,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return haystack.includes(needle)
    })
  }, [itemsQuery.data, filter, search])

  if (itemsQuery.isPending) {
    return <Skeleton className="my-4 h-36" />
  }
  if (itemsQuery.isError) {
    return <EmptyState title="Couldn't load personal Vault">{itemsQuery.error.message}</EmptyState>
  }

  const empty = itemsQuery.data.length === 0

  return (
    <section className="py-4">
      <header className="flex items-center justify-between gap-3">
        <h1 className="text-lg font-semibold text-ink">Vault</h1>
        <Button variant="primary" onClick={() => edit('new')}>
          <Plus size={14} aria-hidden />
          Add
        </Button>
      </header>

      {empty ? (
        <EmptyState title="Nothing saved yet">Agents fill in what you save here without seeing it.</EmptyState>
      ) : (
        <>
          <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
            <Segmented value={filter} options={FILTERS} onChange={setFilter} layoutId="vault-filter" />
            <SearchField value={search} onChange={setSearch} placeholder="Search…" className="h-8 sm:w-[220px]" />
          </div>
          {filtered.length === 0 ? (
            <EmptyState title="No matches" />
          ) : (
            <ul className="mt-3">
              {filtered.map((item) => {
                const Icon = KIND_ICON[item.kind]
                const subtitle = itemSubtitle(item)
                const confirming = confirmDeleteId === item.id
                return (
                  <li
                    key={item.id}
                    className="group/row relative flex items-center gap-3 rounded-control px-2.5 py-2 transition-colors duration-150 hover:bg-list-hover"
                  >
                    <Icon className="size-4 shrink-0 text-ink-3" aria-hidden />
                    <button
                      type="button"
                      onClick={() => edit(item)}
                      className="min-w-0 flex-1 cursor-pointer text-left after:absolute after:inset-0"
                    >
                      <p className="truncate text-[13.5px] font-medium text-ink">{item.label}</p>
                      {subtitle ? <p className="truncate text-[12px] text-ink-2">{subtitle}</p> : null}
                    </button>
                    <div className="relative flex shrink-0 items-center gap-1">
                      {confirming ? (
                        <>
                          <Button size="sm" onClick={() => setConfirmDeleteId(null)}>
                            Cancel
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            disabled={deleteMutation.isPending}
                            onClick={() => deleteMutation.mutate(item.id)}
                          >
                            Delete
                          </Button>
                        </>
                      ) : (
                        <IconButton
                          size="sm"
                          variant="danger"
                          aria-label={`Delete ${item.label}`}
                          onClick={() => setConfirmDeleteId(item.id)}
                          className="opacity-0 transition-opacity duration-150 group-hover/row:opacity-100 focus-visible:opacity-100"
                        >
                          <Trash2 size={14} />
                        </IconButton>
                      )}
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </>
      )}

      {editing ? (
        <VaultItemModal
          item={editing === 'new' ? null : editing}
          initialKind={filter === 'all' ? 'login' : filter}
          saving={saveMutation.isPending}
          error={saveMutation.error?.message ?? ''}
          onClose={() => setEditing(null)}
          onSubmit={saveMutation.mutate}
        />
      ) : null}
    </section>
  )
}
