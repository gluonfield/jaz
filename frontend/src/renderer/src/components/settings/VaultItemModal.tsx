import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
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
  initialKind,
  saving,
  error,
  onClose,
  onSubmit,
}: {
  item: VaultItem | null
  initialKind: VaultKind
  saving: boolean
  error: string
  onClose: () => void
  onSubmit: (input: VaultItemInput & { kind: VaultKind }) => void
}) {
  const [kind, setKind] = useState<VaultKind>(item?.kind ?? initialKind)
  const [values, setValues] = useState<VaultFormValues>(() => initialVaultFormValues({ item }))
  const close = () => {
    if (!saving) onClose()
  }

  return (
    <Modal open onClose={close} title={item?.label ?? 'New item'} size="sm">
      <form
        onSubmit={(event) => {
          event.preventDefault()
          onSubmit(buildVaultItemPayload(kind, values, Boolean(item)))
        }}
        className="-mt-3 space-y-3"
      >
        {item ? null : <Segmented value={kind} options={KINDS} onChange={setKind} layoutId="vault-kind" />}
        <VaultItemFormFields
          kind={kind}
          values={values}
          item={item}
          disabled={saving}
          onChange={(patch) => setValues((prev) => ({ ...prev, ...patch }))}
        />

        {error ? <p className="text-[12px] text-danger">{error}</p> : null}

        <div className="flex justify-end gap-2 pt-2">
          <Button onClick={close} disabled={saving}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={saving}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  )
}
