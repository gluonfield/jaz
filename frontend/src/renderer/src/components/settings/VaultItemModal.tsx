import { X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Segmented } from '@/components/ui/Segmented'
import {
  buildVaultItemPayload,
  initialVaultFormValues,
  VaultItemFormFields,
  type VaultFormValues,
} from '@/components/vault/VaultItemFormFields'
import type { VaultItem, VaultItemInput, VaultKind } from '@/lib/api/vault'

const KINDS: { value: VaultKind; label: string }[] = [
  { value: 'login', label: 'Login' },
  { value: 'card', label: 'Card' },
  { value: 'identity', label: 'Identity' },
]

export function VaultItemModal({
  item,
  initialKind = 'login',
  saving,
  error,
  onClose,
  onSubmit,
}: {
  item: VaultItem | null
  initialKind?: VaultKind
  saving: boolean
  error: string
  onClose: () => void
  onSubmit: (input: VaultItemInput & { kind: VaultKind }) => Promise<void>
}) {
  const isEdit = Boolean(item)
  const [kind, setKind] = useState<VaultKind>(item?.kind ?? initialKind)
  const [values, setValues] = useState<VaultFormValues>(() => initialVaultFormValues({ item }))

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !saving) {
        e.preventDefault()
        e.stopPropagation()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => document.removeEventListener('keydown', onKey, true)
  }, [onClose, saving])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    await onSubmit(buildVaultItemPayload(kind, values, isEdit))
  }

  return (
    <div
      data-escape-surface
      className="fixed inset-0 z-50 grid place-items-center bg-black/45 p-4 backdrop-blur-[1px]"
      onClick={(e) => {
        if (e.target === e.currentTarget && !saving) onClose()
      }}
    >
      <div className="w-full max-w-[460px] rounded-card border border-border bg-surface p-5 shadow-xl">
        <div className="flex items-center justify-between gap-3">
          {isEdit ? (
            <h2 className="truncate text-[15px] font-semibold text-ink">{item?.label}</h2>
          ) : (
            <Segmented value={kind} options={KINDS} onChange={setKind} layoutId="vault-kind" />
          )}
          <IconButton size="sm" onClick={onClose} disabled={saving} aria-label="Close">
            <X size={16} />
          </IconButton>
        </div>

        <form onSubmit={(e) => void handleSubmit(e)} className="mt-4 space-y-3">
          <VaultItemFormFields
            kind={kind}
            values={values}
            item={item}
            disabled={saving}
            onChange={(patch) => setValues((prev) => ({ ...prev, ...patch }))}
          />

          {error ? <p className="text-[12px] text-danger">{error}</p> : null}

          <div className="flex justify-end gap-2 pt-2">
            <Button onClick={onClose} disabled={saving}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={saving}>
              Save
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
