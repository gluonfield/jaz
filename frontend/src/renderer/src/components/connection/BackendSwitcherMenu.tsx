import { Check, Cloud, Laptop, LogIn, Pencil, Plus, X } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { Input } from '@/components/ui/Input'
import { Popover } from '@/components/ui/Popover'
import type { CloudUser } from '@/lib/api/serverTypes'
import { type KnownBackend, renameBackend } from '@/lib/backends'
import { clientRuntime } from '@/lib/clientRuntime'
import { forgetBackend } from '@/lib/connection'
import { describeBackend, localBackendLabel, sameBackend } from '@/lib/connectionDisplay'
import type { useBackendSwitcher } from './useBackendSwitcher'

type SwitcherState = ReturnType<typeof useBackendSwitcher>

export function BackendSwitcherMenu({
  switcher,
  placement,
  trigger,
  onConnectServer,
  editable = false,
  showRemember = false,
}: {
  switcher: SwitcherState
  placement: 'above' | 'below'
  trigger: ReactNode
  onConnectServer: () => void
  editable?: boolean
  showRemember?: boolean
}) {
  const {
    open,
    setOpen,
    status,
    url,
    mode,
    auth,
    oidcLoginUrl,
    user,
    remotes,
    switchLocal,
    switchRemote,
    signInWithSSO,
    remember,
  } = switcher
  const localEnabled = clientRuntime.capabilities.localBackend
  const isCloud = mode === 'cloud'

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement={placement}
      align="start"
      trigger={trigger}
    >
      {isCloud && user ? (
        <>
          <CloudIdentityBadge user={user} />
          <div className="my-1.5 h-px bg-border/70" />
        </>
      ) : null}
      <p className="px-2.5 pb-1 pt-1 text-[11px] font-medium text-ink-3">
        {localEnabled ? 'Run jaz on' : 'Cloud workspaces'}
      </p>
      {localEnabled ? (
        <SwitchRow
          label={localBackendLabel()}
          icon={<Laptop size={12} className="shrink-0 text-ink-3" />}
          current={describeBackend(url).local}
          onSwitch={switchLocal}
        />
      ) : null}
      {remotes.map((remote) => (
        <RemoteRow
          key={remote.url}
          backend={remote}
          current={sameBackend(url, remote.url)}
          editable={editable}
          onSwitch={() => switchRemote(remote.url)}
          onRename={(label) => renameBackend(remote.url, label)}
          onForget={() => forgetBackend(remote.url)}
        />
      ))}
      <div className="my-1 h-px bg-border/70" />
      {isCloud && auth === 'oidc' ? (
        <button
          type="button"
          onClick={() => signInWithSSO(url, oidcLoginUrl)}
          className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <LogIn size={13} />
          {user ? 'Re-authenticate SSO' : 'Sign in with SSO'}
        </button>
      ) : null}
      {showRemember && clientRuntime.encryptCredential && status === 'connected' && !describeBackend(url).local ? (
        <button
          type="button"
          onClick={remember}
          className="flex h-7 w-full items-center rounded-full px-2.5 text-left text-[13px] text-ink-2 hover:bg-surface-2"
        >
          Remember Connection
        </button>
      ) : null}
      <button
        type="button"
        onClick={() => {
          setOpen(false)
          onConnectServer()
        }}
        className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      >
        <Plus size={13} />
        Connect to a server
      </button>
    </Popover>
  )
}

export function CloudIdentityBadge({ user }: { user: CloudUser }) {
  const initials = (user.display_name || user.email || '?')
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
  return (
    <div className="flex items-center gap-2.5 px-2.5 py-1.5">
      <span className="grid size-7 shrink-0 place-items-center rounded-full bg-primary-soft text-[11px] font-semibold text-primary">
        {initials}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-[12.5px] font-medium text-ink">{user.display_name || user.email}</span>
          {user.role ? (
            <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-2">
              {user.role}
            </span>
          ) : null}
        </div>
        <p className="truncate text-[11px] text-ink-3">
          {[user.email, user.org_name || user.org_id].filter(Boolean).join(' · ')}
        </p>
      </div>
    </div>
  )
}

function SwitchRow({
  label,
  icon,
  current,
  onSwitch,
}: {
  label: string
  icon?: ReactNode
  current: boolean
  onSwitch: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSwitch}
      className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
    >
      {icon}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {current ? <Check size={13} className="shrink-0 text-primary" /> : null}
    </button>
  )
}

function RemoteRow({
  backend,
  current,
  editable,
  onSwitch,
  onRename,
  onForget,
}: {
  backend: KnownBackend
  current: boolean
  editable: boolean
  onSwitch: () => void
  onRename: (label: string) => void
  onForget: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(backend.label)
  useEffect(() => setDraft(backend.label), [backend.label])
  const isCloud = backend.mode === 'cloud' || backend.auth === 'oidc'

  const save = () => {
    setEditing(false)
    const next = draft.trim()
    if (next && next !== backend.label) onRename(next)
  }

  if (editable && editing) {
    return (
      <div className="px-1 py-0.5">
        <Input
          autoFocus
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={save}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              save()
            } else if (event.key === 'Escape') {
              event.stopPropagation()
              setDraft(backend.label)
              setEditing(false)
            }
          }}
          aria-label="Backend name"
          className="h-7 text-[13px]"
        />
      </div>
    )
  }

  return (
    <div className="group/row flex h-7 items-center rounded-full pr-1 transition-colors duration-150 hover:bg-surface-2">
      <button
        type="button"
        onClick={onSwitch}
        className="flex min-w-0 flex-1 items-center gap-2 px-2.5 text-left text-[13px] text-ink"
      >
        {isCloud ? <Cloud size={12} className="shrink-0 text-primary" /> : null}
        <span className="min-w-0 flex-1 truncate">{backend.label}</span>
        {current ? <Check size={13} className="shrink-0 text-primary" /> : null}
      </button>
      {editable ? (
        <>
          <RowAction icon={<Pencil size={11} />} label={`Rename ${backend.label}`} onClick={() => setEditing(true)} />
          {!current && !backend.managed ? <RowAction icon={<X size={11} />} label={`Forget ${backend.label}`} onClick={onForget} /> : null}
        </>
      ) : null}
    </div>
  )
}

function RowAction({ icon, label, onClick }: { icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className="grid size-5 shrink-0 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface hover:text-ink focus-visible:opacity-100 group-hover/row:opacity-100"
    >
      {icon}
    </button>
  )
}
