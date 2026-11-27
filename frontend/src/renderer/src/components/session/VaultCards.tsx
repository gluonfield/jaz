import { Check, CreditCard, KeyRound, LoaderCircle, Lock, ShieldCheck, UserRound, X } from 'lucide-react'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { createVaultItem, type VaultItemInput, type VaultKind } from '@/lib/api/vault'
import { keys } from '@/lib/query/keys'
import { normalized } from './TranscriptUtils'

function formatCardExpiry(month?: number, year?: number): string {
  if (!month || !year) return ''
  return `${String(month).padStart(2, '0')}/${String(year).slice(-2)}`
}

export function VaultApprovalCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const approval = permission?.vault_approval
  const [localSelection, setLocalSelection] = useState('')
  const [submitting, setSubmitting] = useState('')
  const [error, setError] = useState('')
  if (!permission || !approval) return null

  const selected = localSelection || resolution?.selected_option_id || permission.selected_option_id || ''
  const status = normalized(resolution?.status || permission.status)
  const cancelled = status === 'cancelled'
  const approved = selected === 'approve_once'
  const denied = selected === 'deny' || cancelled
  const locked = Boolean(selected) || cancelled || Boolean(submitting)

  const decide = async (optionID: 'approve_once' | 'deny') => {
    setSubmitting(optionID)
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        option_id: optionID,
      })
      setLocalSelection(optionID)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Approval response failed.')
    } finally {
      setSubmitting('')
    }
  }

  const expiry = formatCardExpiry(approval.card_exp_month, approval.card_exp_year)

  return (
    <div className="rounded-card border border-border bg-surface px-3.5 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-lg border border-border bg-bg text-ink-2">
            <CreditCard className="size-4" aria-hidden />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-medium text-ink">Approve payment card autofill</p>
            <p className="mt-0.5 text-[12px] text-ink-2">
              {approval.label}
              {approval.card_brand || approval.card_last4
                ? ` · ${[approval.card_brand, approval.card_last4 ? `•••• ${approval.card_last4}` : ''].filter(Boolean).join(' ')}`
                : ''}
              {expiry ? ` · Exp ${expiry}` : ''}
            </p>
          </div>
        </div>
        {approved ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] font-medium text-ok">
            <Check className="size-3.5" aria-hidden />
            Approved once
          </span>
        ) : denied ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] font-medium text-danger">
            <X className="size-3.5" aria-hidden />
            Denied
          </span>
        ) : null}
      </div>

      <div className="mt-2.5 grid gap-1 rounded-control border border-border bg-bg px-2.5 py-2 text-[12px]">
        <div className="flex items-center justify-between gap-2">
          <span className="text-ink-3">Merchant</span>
          <span className="truncate font-mono text-ink">{approval.merchant_origin}</span>
        </div>
        {approval.checkout_summary ? (
          <div className="flex items-center justify-between gap-2">
            <span className="text-ink-3">Summary</span>
            <span className="truncate text-ink">{approval.checkout_summary}</span>
          </div>
        ) : null}
      </div>

      {!approved && !denied ? (
        <div className="mt-2.5 flex flex-wrap items-center gap-2">
          <button
            type="button"
            disabled={locked}
            onClick={() => void decide('approve_once')}
            className="inline-flex h-7 items-center gap-1.5 rounded-full bg-primary px-3 text-[12px] font-medium text-primary-fg transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting === 'approve_once' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <ShieldCheck className="size-3.5" aria-hidden />
            )}
            Approve once
          </button>
          <button
            type="button"
            disabled={locked}
            onClick={() => void decide('deny')}
            className="inline-flex h-7 items-center gap-1.5 rounded-full border border-border bg-bg px-3 text-[12px] font-medium text-ink-2 transition hover:border-danger hover:text-danger disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting === 'deny' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <X className="size-3.5" aria-hidden />
            )}
            Deny
          </button>
        </div>
      ) : null}

      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
    </div>
  )
}

function normalizeRequestKind(kind: string | undefined): VaultKind {
  if (kind === 'card' || kind === 'identity') return kind
  return 'login'
}

export function VaultRequestCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const req = permission?.vault_request
  const queryClient = useQueryClient()
  const kind = normalizeRequestKind(req?.kind)

  const [label, setLabel] = useState(req?.label || '')
  const [origin, setOrigin] = useState(req?.domain || '')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [cardholderName, setCardholderName] = useState('')
  const [cardNumber, setCardNumber] = useState('')
  const [cardExpMonth, setCardExpMonth] = useState('')
  const [cardExpYear, setCardExpYear] = useState('')
  const [cardCvv, setCardCvv] = useState('')
  const [billingAddress, setBillingAddress] = useState('')
  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [phone, setPhone] = useState('')
  const [addressLines, setAddressLines] = useState('')
  const [postalCode, setPostalCode] = useState('')
  const [country, setCountry] = useState('')

  const [localSelection, setLocalSelection] = useState('')
  const [submitting, setSubmitting] = useState<'save' | 'cancel' | ''>('')
  const [error, setError] = useState('')

  if (!permission || !req) return null

  const selected = localSelection || resolution?.selected_option_id || permission.selected_option_id || ''
  const status = normalized(resolution?.status || permission.status)
  const saved = selected.startsWith('saved:')
  const cancelled = status === 'cancelled' || selected === 'deny'
  const locked = saved || cancelled || Boolean(submitting)

  const Icon = kind === 'card' ? CreditCard : kind === 'identity' ? UserRound : KeyRound
  const kindLabel = kind === 'card' ? 'payment card' : kind === 'identity' ? 'identity' : 'login'

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (locked) return
    setSubmitting('save')
    setError('')
    try {
      const payload: VaultItemInput & { kind: VaultKind } = {
        kind,
        label: label.trim() || undefined,
      }
      if (kind === 'login') {
        payload.origins = origin.trim() ? [origin.trim()] : []
        payload.username = username.trim()
        payload.password = password
        if (totpSecret.trim()) payload.totp_secret = totpSecret.trim()
      } else if (kind === 'card') {
        payload.cardholder_name = cardholderName.trim()
        payload.pan = cardNumber.trim()
        payload.card_exp_month = Number.parseInt(cardExpMonth, 10) || 0
        payload.card_exp_year = Number.parseInt(cardExpYear, 10) || 0
        payload.cvv = cardCvv.trim()
        if (billingAddress.trim()) payload.billing_address = billingAddress.trim()
      } else {
        payload.full_name = fullName.trim()
        payload.email = email.trim()
        payload.phone = phone.trim()
        payload.address_lines = addressLines.trim()
        payload.postal_code = postalCode.trim()
        payload.country = country.trim()
      }
      const created = await createVaultItem(payload)
      const optionID = `saved:${created.id}`
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        option_id: optionID,
      })
      setLocalSelection(optionID)
      setPassword('')
      setTotpSecret('')
      setCardNumber('')
      setCardCvv('')
      void queryClient.invalidateQueries({ queryKey: keys.vaultRoot })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save to Vault.')
    } finally {
      setSubmitting('')
    }
  }

  const handleCancel = async () => {
    if (locked) return
    setSubmitting('cancel')
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        option_id: 'deny',
      })
      setLocalSelection('deny')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to cancel request.')
    } finally {
      setSubmitting('')
    }
  }

  const inputClass =
    'h-8 w-full rounded-control border border-border bg-bg px-2.5 text-[12px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none disabled:opacity-60'

  return (
    <div className="rounded-card border border-border bg-surface px-3.5 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-lg border border-border bg-bg text-ink-2">
            <Icon className="size-4" aria-hidden />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-medium text-ink">
              {req.label ? `Save ${req.label} to Vault` : `Save ${kindLabel} to personal Vault`}
            </p>
            <p className="mt-0.5 flex items-center gap-1 text-[11.5px] text-ink-3">
              <Lock className="size-3 shrink-0" aria-hidden />
              <span>
                {req.reason ? `${req.reason} · ` : ''}
                Encrypted directly in your Vault — secrets never enter chat history.
              </span>
            </p>
          </div>
        </div>
        {saved ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] font-medium text-ok">
            <Check className="size-3.5" aria-hidden />
            Saved to Vault
          </span>
        ) : cancelled ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] font-medium text-ink-3">
            <X className="size-3.5" aria-hidden />
            Cancelled
          </span>
        ) : null}
      </div>

      {!saved && !cancelled ? (
        <form onSubmit={(e) => void handleSave(e)} className="mt-3 space-y-2">
          <input
            type="text"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="Label (optional)"
            disabled={locked}
            className={inputClass}
          />

          {kind === 'login' ? (
            <>
              <input
                type="text"
                value={origin}
                onChange={(e) => setOrigin(e.target.value)}
                placeholder="Website origin (e.g. https://github.com)"
                required
                disabled={locked}
                className={inputClass}
              />
              <div className="grid grid-cols-2 gap-2">
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="Username or email"
                  required
                  autoComplete="off"
                  disabled={locked}
                  className={inputClass}
                />
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Password"
                  required
                  autoComplete="new-password"
                  disabled={locked}
                  className={inputClass}
                />
              </div>
              <input
                type="password"
                value={totpSecret}
                onChange={(e) => setTotpSecret(e.target.value)}
                placeholder="Authenticator TOTP secret (optional)"
                autoComplete="off"
                disabled={locked}
                className={inputClass}
              />
            </>
          ) : null}

          {kind === 'card' ? (
            <>
              <input
                type="text"
                value={cardholderName}
                onChange={(e) => setCardholderName(e.target.value)}
                placeholder="Cardholder name"
                autoComplete="off"
                disabled={locked}
                className={inputClass}
              />
              <input
                type="password"
                inputMode="numeric"
                value={cardNumber}
                onChange={(e) => setCardNumber(e.target.value)}
                placeholder="Card number"
                required
                autoComplete="off"
                disabled={locked}
                className={inputClass}
              />
              <div className="grid grid-cols-3 gap-2">
                <input
                  type="number"
                  min={1}
                  max={12}
                  value={cardExpMonth}
                  onChange={(e) => setCardExpMonth(e.target.value)}
                  placeholder="MM"
                  required
                  disabled={locked}
                  className={inputClass}
                />
                <input
                  type="number"
                  min={2024}
                  max={2100}
                  value={cardExpYear}
                  onChange={(e) => setCardExpYear(e.target.value)}
                  placeholder="YYYY"
                  required
                  disabled={locked}
                  className={inputClass}
                />
                <input
                  type="password"
                  inputMode="numeric"
                  value={cardCvv}
                  onChange={(e) => setCardCvv(e.target.value)}
                  placeholder="CVV"
                  required
                  autoComplete="off"
                  disabled={locked}
                  className={inputClass}
                />
              </div>
              <input
                type="text"
                value={billingAddress}
                onChange={(e) => setBillingAddress(e.target.value)}
                placeholder="Billing address (optional)"
                autoComplete="off"
                disabled={locked}
                className={inputClass}
              />
            </>
          ) : null}

          {kind === 'identity' ? (
            <>
              <div className="grid grid-cols-2 gap-2">
                <input
                  type="text"
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  placeholder="Full name"
                  disabled={locked}
                  className={inputClass}
                />
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="Email"
                  disabled={locked}
                  className={inputClass}
                />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <input
                  type="tel"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  placeholder="Phone"
                  disabled={locked}
                  className={inputClass}
                />
                <input
                  type="text"
                  value={country}
                  onChange={(e) => setCountry(e.target.value)}
                  placeholder="Country"
                  disabled={locked}
                  className={inputClass}
                />
              </div>
              <div className="grid grid-cols-3 gap-2">
                <input
                  type="text"
                  value={addressLines}
                  onChange={(e) => setAddressLines(e.target.value)}
                  placeholder="Street address"
                  disabled={locked}
                  className={`${inputClass} col-span-2`}
                />
                <input
                  type="text"
                  value={postalCode}
                  onChange={(e) => setPostalCode(e.target.value)}
                  placeholder="Postal code"
                  disabled={locked}
                  className={inputClass}
                />
              </div>
            </>
          ) : null}

          <div className="flex items-center gap-2 pt-1">
            <button
              type="submit"
              disabled={locked}
              className="inline-flex h-7 items-center gap-1.5 rounded-full bg-primary px-3 text-[12px] font-medium text-primary-fg transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting === 'save' ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
              ) : (
                <Lock className="size-3.5" aria-hidden />
              )}
              Save to Vault
            </button>
            <button
              type="button"
              disabled={locked}
              onClick={() => void handleCancel()}
              className="inline-flex h-7 items-center gap-1.5 rounded-full border border-border bg-bg px-3 text-[12px] font-medium text-ink-2 transition hover:text-ink disabled:cursor-not-allowed disabled:opacity-60"
            >
              Cancel
            </button>
          </div>
          {error ? <p className="text-[12px] text-danger">{error}</p> : null}
        </form>
      ) : null}
    </div>
  )
}
