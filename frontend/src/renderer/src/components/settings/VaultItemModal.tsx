import { Lock, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import {
  buildVaultItemPayload,
  initialVaultFormValues,
  VaultItemFormFields,
  type VaultFormValues,
} from '@/components/vault/VaultItemFormFields'
import type { VaultItem, VaultItemInput, VaultKind } from '@/lib/api/vault'

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
          <div className="flex items-center gap-2">
            <Lock className="size-4 text-ink-2" aria-hidden />
            <h2 className="text-[15px] font-semibold text-ink">
              {isEdit ? 'Edit vault item' : 'Add vault item'}
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={saving}
            aria-label="Close"
            className="grid size-7 place-items-center rounded-md text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink"
          >
            <X className="size-4" />
          </button>
        </div>

        {!isEdit ? (
          <div className="mt-3 flex gap-1.5">
            {(['login', 'card', 'identity'] as const).map((option) => (
              <button
                key={option}
                type="button"
                onClick={() => setKind(option)}
                className={`h-7 rounded-full px-3 text-[12px] font-medium capitalize transition-colors ${
                  kind === option
                    ? 'bg-primary text-primary-fg'
                    : 'border border-border bg-bg text-ink-2 hover:text-ink'
                }`}
              >
                {option === 'card' ? 'Payment card' : option}
              </button>
            ))}
          </div>
        ) : null}

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
            <button
              type="button"
              onClick={onClose}
              disabled={saving}
              className="h-8 rounded-control border border-border bg-bg px-3 text-[13px] font-medium text-ink-2 transition-colors hover:text-ink"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving}
              className="h-8 rounded-control bg-primary px-3.5 text-[13px] font-medium text-primary-fg transition-opacity hover:opacity-90 disabled:opacity-60"
            >
              {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Save item'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
