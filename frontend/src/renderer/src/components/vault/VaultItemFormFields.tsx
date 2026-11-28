import type { VaultItem, VaultItemInput, VaultKind } from '@/lib/api/vault'

export interface VaultFormValues {
  label: string
  origins: string
  username: string
  password: string
  totpSecret: string
  cardholderName: string
  pan: string
  cardExpMonth: string
  cardExpYear: string
  cvv: string
  billingAddress: string
  fullName: string
  email: string
  phone: string
  addressLines: string
  postalCode: string
  country: string
}

export function initialVaultFormValues(init?: {
  item?: VaultItem | null
  label?: string
  domain?: string
}): VaultFormValues {
  const item = init?.item
  return {
    label: item?.label ?? init?.label ?? '',
    origins: item?.origins?.join(', ') ?? init?.domain ?? '',
    username: item?.username ?? '',
    password: '',
    totpSecret: '',
    cardholderName: item?.cardholder_name ?? '',
    pan: '',
    cardExpMonth: item?.card_exp_month ? String(item.card_exp_month) : '',
    cardExpYear: item?.card_exp_year ? String(item.card_exp_year) : '',
    cvv: '',
    billingAddress: '',
    fullName: '',
    email: item?.email ?? '',
    phone: '',
    addressLines: '',
    postalCode: '',
    country: '',
  }
}

export function buildVaultItemPayload(
  kind: VaultKind,
  values: VaultFormValues,
  isEdit: boolean,
): VaultItemInput & { kind: VaultKind } {
  const input: VaultItemInput & { kind: VaultKind } = {
    kind,
    label: values.label.trim() || undefined,
  }
  if (kind === 'login') {
    input.origins = values.origins
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (isEdit) input.replace_origins = true
    input.username = values.username.trim()
    if (values.password) input.password = values.password
    if (values.totpSecret.trim()) input.totp_secret = values.totpSecret.trim()
    return input
  }
  if (kind === 'card') {
    input.cardholder_name = values.cardholderName.trim()
    if (values.pan.trim()) input.pan = values.pan.trim()
    if (values.cardExpMonth.trim()) input.card_exp_month = Number.parseInt(values.cardExpMonth, 10) || 0
    if (values.cardExpYear.trim()) input.card_exp_year = Number.parseInt(values.cardExpYear, 10) || 0
    if (values.cvv.trim()) input.cvv = values.cvv.trim()
    if (values.billingAddress.trim()) input.billing_address = values.billingAddress.trim()
    return input
  }
  if (values.fullName.trim()) input.full_name = values.fullName.trim()
  input.email = values.email.trim()
  if (values.phone.trim()) input.phone = values.phone.trim()
  if (values.addressLines.trim()) input.address_lines = values.addressLines.trim()
  if (values.postalCode.trim()) input.postal_code = values.postalCode.trim()
  if (values.country.trim()) input.country = values.country.trim()
  return input
}

export function VaultItemFormFields({
  kind,
  values,
  item,
  compact = false,
  disabled = false,
  onChange,
}: {
  kind: VaultKind
  values: VaultFormValues
  item?: VaultItem | null
  compact?: boolean
  disabled?: boolean
  onChange: (patch: Partial<VaultFormValues>) => void
}) {
  const isEdit = Boolean(item)
  const inputClass = compact
    ? 'h-8 w-full rounded-control border border-border bg-bg px-2.5 text-[12px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none disabled:opacity-60'
    : 'h-9 w-full rounded-control border border-border bg-bg px-3 text-[13px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none disabled:opacity-60'
  const labelClass = 'mb-1 block text-[12px] font-medium text-ink-2'
  const field = (label: string, control: React.ReactNode, className?: string) =>
    compact ? (
      <div className={className}>{control}</div>
    ) : (
      <div className={className}>
        <label className={labelClass}>{label}</label>
        {control}
      </div>
    )

  return (
    <div className={compact ? 'space-y-2' : 'space-y-3'}>
      {field(
        'Label',
        <input
          type="text"
          value={values.label}
          onChange={(e) => onChange({ label: e.target.value })}
          placeholder={
            compact
              ? 'Label (optional)'
              : kind === 'card'
                ? 'Personal Visa'
                : kind === 'identity'
                  ? 'Home shipping'
                  : 'GitHub'
          }
          disabled={disabled}
          className={inputClass}
        />,
      )}

      {kind === 'login' ? (
        <>
          {field(
            'Website',
            <input
              type="text"
              value={values.origins}
              onChange={(e) => onChange({ origins: e.target.value })}
              placeholder="https://github.com"
              required
              disabled={disabled}
              className={inputClass}
            />,
          )}
          <div className={compact ? 'grid grid-cols-2 gap-2' : 'space-y-3'}>
            {field(
              'Username or email',
              <input
                type="text"
                value={values.username}
                onChange={(e) => onChange({ username: e.target.value })}
                placeholder={compact ? 'Username or email' : undefined}
                required
                autoComplete="off"
                disabled={disabled}
                className={inputClass}
              />,
            )}
            {field(
              'Password',
              <input
                type="password"
                value={values.password}
                onChange={(e) => onChange({ password: e.target.value })}
                placeholder={isEdit ? 'Leave blank to keep current password' : 'Password'}
                required={!isEdit}
                autoComplete="new-password"
                disabled={disabled}
                className={inputClass}
              />,
            )}
          </div>
          {field(
            '2FA secret (optional)',
            <input
              type="password"
              value={values.totpSecret}
              onChange={(e) => onChange({ totpSecret: e.target.value })}
              placeholder={
                isEdit && item?.has_totp
                  ? 'Configured — enter new secret to replace'
                  : compact
                    ? '2FA secret (optional)'
                    : undefined
              }
              autoComplete="off"
              disabled={disabled}
              className={inputClass}
            />,
          )}
        </>
      ) : null}

      {kind === 'card' ? (
        <>
          {field(
            'Cardholder name',
            <input
              type="text"
              value={values.cardholderName}
              onChange={(e) => onChange({ cardholderName: e.target.value })}
              placeholder={compact ? 'Cardholder name' : undefined}
              autoComplete="off"
              disabled={disabled}
              className={inputClass}
            />,
          )}
          {field(
            'Card number',
            <input
              type="password"
              inputMode="numeric"
              value={values.pan}
              onChange={(e) => onChange({ pan: e.target.value })}
              placeholder={isEdit ? `•••• ${item?.card_last4 ?? ''} — leave blank to keep` : 'Card number'}
              required={!isEdit}
              autoComplete="off"
              disabled={disabled}
              className={inputClass}
            />,
          )}
          <div className="grid grid-cols-3 gap-2">
            {field(
              'Exp month',
              <input
                type="number"
                min={1}
                max={12}
                value={values.cardExpMonth}
                onChange={(e) => onChange({ cardExpMonth: e.target.value })}
                placeholder="MM"
                required
                disabled={disabled}
                className={inputClass}
              />,
            )}
            {field(
              'Exp year',
              <input
                type="number"
                min={2024}
                max={2100}
                value={values.cardExpYear}
                onChange={(e) => onChange({ cardExpYear: e.target.value })}
                placeholder="YYYY"
                required
                disabled={disabled}
                className={inputClass}
              />,
            )}
            {field(
              'CVV',
              <input
                type="password"
                inputMode="numeric"
                value={values.cvv}
                onChange={(e) => onChange({ cvv: e.target.value })}
                placeholder={isEdit ? 'Keep' : 'CVV'}
                required={!isEdit}
                autoComplete="off"
                disabled={disabled}
                className={inputClass}
              />,
            )}
          </div>
          {field(
            'Billing address (optional)',
            <input
              type="text"
              value={values.billingAddress}
              onChange={(e) => onChange({ billingAddress: e.target.value })}
              placeholder={isEdit && item?.billing_summary ? item.billing_summary : 'Billing address (optional)'}
              autoComplete="off"
              disabled={disabled}
              className={inputClass}
            />,
          )}
        </>
      ) : null}

      {kind === 'identity' ? (
        <>
          <div className="grid grid-cols-2 gap-2">
            {field(
              'Full name',
              <input
                type="text"
                value={values.fullName}
                onChange={(e) => onChange({ fullName: e.target.value })}
                placeholder={compact ? 'Full name' : isEdit ? 'Leave blank to keep' : undefined}
                disabled={disabled}
                className={inputClass}
              />,
            )}
            {field(
              'Email',
              <input
                type="email"
                value={values.email}
                onChange={(e) => onChange({ email: e.target.value })}
                placeholder={compact ? 'Email' : undefined}
                disabled={disabled}
                className={inputClass}
              />,
            )}
          </div>
          <div className="grid grid-cols-2 gap-2">
            {field(
              'Phone',
              <input
                type="tel"
                value={values.phone}
                onChange={(e) => onChange({ phone: e.target.value })}
                placeholder={compact ? 'Phone' : isEdit ? 'Leave blank to keep' : undefined}
                disabled={disabled}
                className={inputClass}
              />,
            )}
            {field(
              'Country',
              <input
                type="text"
                value={values.country}
                onChange={(e) => onChange({ country: e.target.value })}
                placeholder={compact ? 'Country' : isEdit ? 'Leave blank to keep' : undefined}
                disabled={disabled}
                className={inputClass}
              />,
            )}
          </div>
          <div className="grid grid-cols-3 gap-2">
            {field(
              'Address',
              <input
                type="text"
                value={values.addressLines}
                onChange={(e) => onChange({ addressLines: e.target.value })}
                placeholder={isEdit ? 'Leave blank to keep existing address' : 'Street address'}
                disabled={disabled}
                className={inputClass}
              />,
              'col-span-2',
            )}
            {field(
              'Postal code',
              <input
                type="text"
                value={values.postalCode}
                onChange={(e) => onChange({ postalCode: e.target.value })}
                placeholder={isEdit ? 'Keep' : 'Postal code'}
                disabled={disabled}
                className={inputClass}
              />,
            )}
          </div>
        </>
      ) : null}
    </div>
  )
}
