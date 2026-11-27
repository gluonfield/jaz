import { Check, CreditCard, KeyRound, LoaderCircle, Lock, ShieldCheck, UserRound, X } from 'lucide-react'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  buildVaultItemPayload,
  initialVaultFormValues,
  VaultItemFormFields,
  type VaultFormValues,
} from '@/components/vault/VaultItemFormFields'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { createVaultItem, type VaultKind } from '@/lib/api/vault'
import { useCloudSession } from '@/lib/cloudSession'
import { activeCloudWorkspace } from '@/lib/cloudWorkspaceDisplay'
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
  const cloud = useCloudSession()
  const workspaceID = activeCloudWorkspace(cloud)?.id
  const kind = normalizeRequestKind(req?.kind)

  const [values, setValues] = useState<VaultFormValues>(() =>
    initialVaultFormValues({ label: req?.label, domain: req?.domain }),
  )
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
      const created = await createVaultItem(buildVaultItemPayload(kind, values, false), workspaceID)
      const optionID = `saved:${created.id}`
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        option_id: optionID,
      })
      setLocalSelection(optionID)
      setValues((prev) => ({ ...prev, password: '', totpSecret: '', pan: '', cvv: '' }))
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
          <VaultItemFormFields
            kind={kind}
            values={values}
            compact
            disabled={locked}
            onChange={(patch) => setValues((prev) => ({ ...prev, ...patch }))}
          />
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
