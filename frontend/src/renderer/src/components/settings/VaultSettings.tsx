import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CreditCard, KeyRound, Pencil, Plus, Search, ShieldCheck, Trash2, Upload, UserRound } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  createVaultItem,
  deleteVaultItem,
  importVaultCSV,
  updateVaultItem,
  vaultItemsQuery,
  type VaultItem,
  type VaultItemInput,
  type VaultKind,
} from '@/lib/api/vault'
import { keys } from '@/lib/query/keys'
import { VaultItemModal } from './VaultItemModal'

type FilterKind = 'all' | VaultKind

function itemSubtitle(item: VaultItem): string {
  if (item.kind === 'login') {
    const parts = [item.origins?.[0], item.username].filter(Boolean)
    return parts.join(' · ')
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
  const itemsQuery = useQuery(vaultItemsQuery())

  const [filter, setFilter] = useState<FilterKind>('all')
  const [search, setSearch] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<VaultItem | null>(null)
  const [modalError, setModalError] = useState('')
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [csvText, setCsvText] = useState('')
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.vaultRoot })

  const saveMutation = useMutation({
    mutationFn: async (payload: { id?: string; input: VaultItemInput & { kind: VaultKind } }) => {
      if (payload.id) {
        return updateVaultItem(payload.id, payload.input)
      }
      return createVaultItem(payload.input)
    },
    onSuccess: () => {
      void invalidate()
      setModalOpen(false)
      setEditing(null)
      setModalError('')
      toast(editing ? 'Vault item updated.' : 'Saved to personal Vault.', 'ok')
    },
    onError: (err: Error) => {
      setModalError(err.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteVaultItem(id),
    onSuccess: () => {
      void invalidate()
      setConfirmDeleteId(null)
      toast('Removed from Vault.', 'ok')
    },
    onError: (err: Error) => toast(`Couldn't delete item: ${err.message}`, 'danger'),
  })

  const importMutation = useMutation({
    mutationFn: (csv: string) => importVaultCSV(csv),
    onSuccess: (res) => {
      void invalidate()
      setCsvText('')
      setImportOpen(false)
      toast(
        `Imported ${res.imported} login${res.imported === 1 ? '' : 's'}${res.skipped ? ` (${res.skipped} skipped)` : ''}.`,
        'ok',
      )
    },
    onError: (err: Error) => toast(`CSV import failed: ${err.message}`, 'danger'),
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

  const openAdd = () => {
    setEditing(null)
    setModalError('')
    setModalOpen(true)
  }

  const openEdit = (item: VaultItem) => {
    setEditing(item)
    setModalError('')
    setModalOpen(true)
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const text = await file.text()
    setCsvText(text)
    e.target.value = ''
  }

  if (itemsQuery.isPending) {
    return <Skeleton className="my-4 h-36" />
  }
  if (itemsQuery.isError) {
    return <EmptyState title="Couldn't load personal Vault">{itemsQuery.error.message}</EmptyState>
  }

  return (
    <section className="py-4">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-ink">Vault</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">
            Personal encrypted logins, payment cards, and identities for blind autofill. Secrets stay encrypted and never enter chat history or model context.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setImportOpen((v) => !v)}
            className="inline-flex h-8 items-center gap-1.5 rounded-control border border-border bg-bg px-3 text-[12.5px] font-medium text-ink-2 transition-colors hover:text-ink"
          >
            <Upload className="size-3.5" aria-hidden />
            Import CSV
          </button>
          <button
            type="button"
            onClick={openAdd}
            className="inline-flex h-8 items-center gap-1.5 rounded-control bg-primary px-3 text-[12.5px] font-medium text-primary-fg transition-opacity hover:opacity-90"
          >
            <Plus className="size-3.5" aria-hidden />
            Add item
          </button>
        </div>
      </header>

      <div className="mt-3 flex items-center gap-2 rounded-control border border-border bg-surface px-3 py-2 text-[12px] text-ink-2">
        <ShieldCheck className="size-4 shrink-0 text-ok" aria-hidden />
        <span>
          Logins autofill silently only on exact matching origins. Payment cards always prompt for per-transaction confirmation.
        </span>
      </div>

      {importOpen ? (
        <div className="mt-4 rounded-card border border-border bg-surface p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 className="text-[13.5px] font-medium text-ink">Import logins from CSV</h2>
              <p className="text-[12px] text-ink-3">
                Supports exports from 1Password, Bitwarden, LastPass, Chrome, Edge, and Safari.
              </p>
            </div>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="h-7 rounded-control border border-border bg-bg px-2.5 text-[12px] font-medium text-ink-2 hover:text-ink"
            >
              Choose CSV file
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".csv,text/csv"
              onChange={(e) => void handleFileChange(e)}
              className="hidden"
            />
          </div>
          <textarea
            value={csvText}
            onChange={(e) => setCsvText(e.target.value)}
            rows={4}
            placeholder="name,url,username,password"
            className="mt-2.5 w-full rounded-control border border-border bg-bg p-2.5 font-mono text-[12px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none"
          />
          <div className="mt-2.5 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setImportOpen(false)
                setCsvText('')
              }}
              className="h-7 rounded-control border border-border bg-bg px-3 text-[12px] font-medium text-ink-2 hover:text-ink"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={!csvText.trim() || importMutation.isPending}
              onClick={() => importMutation.mutate(csvText)}
              className="h-7 rounded-control bg-primary px-3 text-[12px] font-medium text-primary-fg disabled:opacity-60"
            >
              {importMutation.isPending ? 'Importing…' : 'Import logins'}
            </button>
          </div>
        </div>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1">
          {([
            ['all', 'All'],
            ['login', 'Logins'],
            ['card', 'Cards'],
            ['identity', 'Identities'],
          ] as const).map(([id, label]) => (
            <button
              key={id}
              type="button"
              onClick={() => setFilter(id)}
              className={`h-7 rounded-full px-3 text-[12px] font-medium transition-colors ${
                filter === id
                  ? 'bg-list-active text-ink'
                  : 'text-ink-2 hover:bg-list-hover hover:text-ink'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="relative min-w-[200px] flex-1 sm:max-w-[260px]">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-ink-3" />
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter vault items…"
            className="h-8 w-full rounded-control border border-border bg-bg pr-3 pl-8 text-[12.5px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none"
          />
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="mt-4">
          <EmptyState title={itemsQuery.data.length === 0 ? 'Your personal Vault is empty' : 'No matching items'}>
            {itemsQuery.data.length === 0
              ? 'Add logins, payment cards, or identities so agents can autofill forms without seeing your secrets.'
              : 'Try a different filter or search term.'}
          </EmptyState>
        </div>
      ) : (
        <ul className="mt-4 divide-y divide-border overflow-hidden rounded-card border border-border bg-surface">
          {filtered.map((item) => {
            const Icon = item.kind === 'card' ? CreditCard : item.kind === 'identity' ? UserRound : KeyRound
            const subtitle = itemSubtitle(item)
            return (
              <li key={item.id} className="flex items-center justify-between gap-3 px-3.5 py-2.5">
                <div className="flex min-w-0 items-center gap-3">
                  <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-border bg-bg text-ink-2">
                    <Icon className="size-4" aria-hidden />
                  </span>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <p className="truncate text-[13.5px] font-medium text-ink">{item.label}</p>
                      {item.has_totp ? (
                        <span className="rounded bg-bg px-1.5 py-px text-[10.5px] font-medium text-ink-3">
                          2FA
                        </span>
                      ) : null}
                    </div>
                    {subtitle ? (
                      <p className="truncate text-[12px] text-ink-2">{subtitle}</p>
                    ) : null}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {confirmDeleteId === item.id ? (
                    <>
                      <button
                        type="button"
                        disabled={deleteMutation.isPending}
                        onClick={() => deleteMutation.mutate(item.id)}
                        className="h-7 rounded-control bg-danger px-2.5 text-[12px] font-medium text-white disabled:opacity-60"
                      >
                        Delete
                      </button>
                      <button
                        type="button"
                        onClick={() => setConfirmDeleteId(null)}
                        className="h-7 rounded-control border border-border bg-bg px-2 text-[12px] text-ink-2"
                      >
                        Cancel
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        type="button"
                        onClick={() => openEdit(item)}
                        aria-label={`Edit ${item.label}`}
                        className="grid size-7 place-items-center rounded-md text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink"
                      >
                        <Pencil className="size-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setConfirmDeleteId(item.id)}
                        aria-label={`Delete ${item.label}`}
                        className="grid size-7 place-items-center rounded-md text-ink-3 transition-colors hover:bg-surface-2 hover:text-danger"
                      >
                        <Trash2 className="size-3.5" />
                      </button>
                    </>
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      )}

      {modalOpen ? (
        <VaultItemModal
          item={editing}
          initialKind={filter === 'all' ? 'login' : filter}
          saving={saveMutation.isPending}
          error={modalError}
          onClose={() => {
            if (!saveMutation.isPending) {
              setModalOpen(false)
              setEditing(null)
              setModalError('')
            }
          }}
          onSubmit={async (input) => {
            await saveMutation.mutateAsync({ id: editing?.id, input })
          }}
        />
      ) : null}
    </section>
  )
}
