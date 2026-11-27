import { Lock, X } from 'lucide-react'
import { useState } from 'react'
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
  const [label, setLabel] = useState(item?.label ?? '')
  const [originsText, setOriginsText] = useState((item?.origins ?? []).join(', '))
  const [username, setUsername] = useState(item?.username ?? '')
  const [password, setPassword] = useState('')
  const [totpSecret, setTotpSecret] = useState('')

  const [cardholderName, setCardholderName] = useState(item?.cardholder_name ?? '')
  const [cardNumber, setCardNumber] = useState('')
  const [cardExpMonth, setCardExpMonth] = useState(item?.card_exp_month ? String(item.card_exp_month) : '')
  const [cardExpYear, setCardExpYear] = useState(item?.card_exp_year ? String(item.card_exp_year) : '')
  const [cardCvv, setCardCvv] = useState('')
  const [billingAddress, setBillingAddress] = useState('')

  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState(item?.email ?? '')
  const [phone, setPhone] = useState('')
  const [addressLines, setAddressLines] = useState('')
  const [postalCode, setPostalCode] = useState('')
  const [country, setCountry] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const input: VaultItemInput & { kind: VaultKind } = {
      kind,
      label: label.trim() || undefined,
    }
    if (kind === 'login') {
      input.origins = originsText
        .split(/[\n,]/)
        .map((s) => s.trim())
        .filter(Boolean)
      if (isEdit) input.replace_origins = true
      input.username = username.trim()
      if (password) input.password = password
      if (totpSecret.trim()) input.totp_secret = totpSecret.trim()
    } else if (kind === 'card') {
      input.cardholder_name = cardholderName.trim()
      if (cardNumber.trim()) input.pan = cardNumber.trim()
      if (cardExpMonth.trim()) input.card_exp_month = Number.parseInt(cardExpMonth, 10) || 0
      if (cardExpYear.trim()) input.card_exp_year = Number.parseInt(cardExpYear, 10) || 0
      if (cardCvv.trim()) input.cvv = cardCvv.trim()
      if (billingAddress.trim()) input.billing_address = billingAddress.trim()
    } else {
      if (fullName.trim()) input.full_name = fullName.trim()
      input.email = email.trim()
      if (phone.trim()) input.phone = phone.trim()
      if (addressLines.trim()) input.address_lines = addressLines.trim()
      if (postalCode.trim()) input.postal_code = postalCode.trim()
      if (country.trim()) input.country = country.trim()
    }
    await onSubmit(input)
  }

  const inputClass =
    'h-9 w-full rounded-control border border-border bg-bg px-3 text-[13px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none'

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
          <div>
            <label className="mb-1 block text-[12px] font-medium text-ink-2">Label</label>
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder={kind === 'card' ? 'Personal Visa' : kind === 'identity' ? 'Home shipping' : 'GitHub'}
              className={inputClass}
            />
          </div>

          {kind === 'login' ? (
            <>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">
                  Website origins (exact match)
                </label>
                <input
                  type="text"
                  value={originsText}
                  onChange={(e) => setOriginsText(e.target.value)}
                  placeholder="https://github.com"
                  required
                  className={inputClass}
                />
              </div>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">Username or email</label>
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  autoComplete="off"
                  className={inputClass}
                />
              </div>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">Password</label>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={isEdit ? 'Leave blank to keep current password' : 'Enter password'}
                  required={!isEdit}
                  autoComplete="new-password"
                  className={inputClass}
                />
              </div>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">
                  Authenticator TOTP secret (optional)
                </label>
                <input
                  type="password"
                  value={totpSecret}
                  onChange={(e) => setTotpSecret(e.target.value)}
                  placeholder={isEdit && item?.has_totp ? 'Configured — enter new secret to replace' : 'Base32 secret'}
                  autoComplete="off"
                  className={inputClass}
                />
              </div>
            </>
          ) : null}

          {kind === 'card' ? (
            <>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">Cardholder name</label>
                <input
                  type="text"
                  value={cardholderName}
                  onChange={(e) => setCardholderName(e.target.value)}
                  autoComplete="off"
                  className={inputClass}
                />
              </div>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">Card number</label>
                <input
                  type="password"
                  inputMode="numeric"
                  value={cardNumber}
                  onChange={(e) => setCardNumber(e.target.value)}
                  placeholder={isEdit ? `•••• ${item?.card_last4 ?? ''} — leave blank to keep` : 'Card number'}
                  required={!isEdit}
                  autoComplete="off"
                  className={inputClass}
                />
              </div>
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Exp month</label>
                  <input
                    type="number"
                    min={1}
                    max={12}
                    value={cardExpMonth}
                    onChange={(e) => setCardExpMonth(e.target.value)}
                    placeholder="MM"
                    required
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Exp year</label>
                  <input
                    type="number"
                    min={2024}
                    max={2100}
                    value={cardExpYear}
                    onChange={(e) => setCardExpYear(e.target.value)}
                    placeholder="YYYY"
                    required
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">CVV</label>
                  <input
                    type="password"
                    inputMode="numeric"
                    value={cardCvv}
                    onChange={(e) => setCardCvv(e.target.value)}
                    placeholder={isEdit ? 'Keep' : 'CVV'}
                    required={!isEdit}
                    autoComplete="off"
                    className={inputClass}
                  />
                </div>
              </div>
              <div>
                <label className="mb-1 block text-[12px] font-medium text-ink-2">Billing address (optional)</label>
                <input
                  type="text"
                  value={billingAddress}
                  onChange={(e) => setBillingAddress(e.target.value)}
                  placeholder={isEdit && item?.billing_summary ? item.billing_summary : 'Street, city, postcode'}
                  autoComplete="off"
                  className={inputClass}
                />
              </div>
            </>
          ) : null}

          {kind === 'identity' ? (
            <>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Full name</label>
                  <input
                    type="text"
                    value={fullName}
                    onChange={(e) => setFullName(e.target.value)}
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Email</label>
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className={inputClass}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Phone</label>
                  <input
                    type="tel"
                    value={phone}
                    onChange={(e) => setPhone(e.target.value)}
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Country</label>
                  <input
                    type="text"
                    value={country}
                    onChange={(e) => setCountry(e.target.value)}
                    className={inputClass}
                  />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                <div className="col-span-2">
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Address</label>
                  <input
                    type="text"
                    value={addressLines}
                    onChange={(e) => setAddressLines(e.target.value)}
                    placeholder={isEdit ? 'Leave blank to keep existing address' : 'Street address'}
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-[12px] font-medium text-ink-2">Postal code</label>
                  <input
                    type="text"
                    value={postalCode}
                    onChange={(e) => setPostalCode(e.target.value)}
                    placeholder={isEdit ? 'Keep' : 'Postcode'}
                    className={inputClass}
                  />
                </div>
              </div>
            </>
          ) : null}

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
