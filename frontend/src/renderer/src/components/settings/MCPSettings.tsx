import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  KeyRound,
  Loader2,
  Pencil,
  Plus,
  Power,
  RefreshCcw,
  Trash2,
} from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { DashedCta } from '@/components/ui/DashedCta'
import { IconButton } from '@/components/ui/IconButton'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import {
  authorizeMCPServer,
  createMCPServer,
  deleteMCPServer,
  mcpServersQuery,
  setMCPServerEnabled,
  testMCPServer,
  updateMCPServer,
} from '@/lib/api/mcp'
import type {
  MCPEnvHeader,
  MCPHeader,
  MCPOAuthConfig,
  MCPServer,
  MCPServerInput,
} from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

type Draft = MCPServerInput & { id?: string }

function emptyDraft(): Draft {
  return {
    name: '',
    url: '',
    enabled: true,
    bearer_token_env_var: '',
    headers: [],
    env_headers: [],
    oauth: {},
  }
}

function draftFromServer(server: MCPServer): Draft {
  return {
    id: server.id,
    name: server.name,
    url: server.url,
    enabled: server.enabled,
    bearer_token_env_var: server.bearer_token_env_var ?? '',
    headers: server.headers ?? [],
    env_headers: server.env_headers ?? [],
    oauth: server.oauth ?? {},
  }
}

export function MCPSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const servers = useQuery(mcpServersQuery)
  const [draft, setDraft] = useState<Draft | null>(null)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.mcpServers })
  const save = useMutation({
    mutationFn: (input: Draft) =>
      input.id ? updateMCPServer(input.id, input) : createMCPServer(input),
    onSuccess: (server) => {
      toast(`Saved ${server.name}`)
      setDraft(null)
    },
    onSettled: invalidate,
  })
  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      setMCPServerEnabled(id, enabled),
    onError: (error: Error) => toast(`Couldn't update MCP server: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })
  const remove = useMutation({
    mutationFn: deleteMCPServer,
    onSuccess: () => toast('Deleted MCP server'),
    onError: (error: Error) => toast(`Couldn't delete MCP server: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })
  const test = useMutation({
    mutationFn: testMCPServer,
    onSuccess: (status) => {
      if (status.status === 'connected') {
        toast(`Connected, ${status.tool_count} tool${status.tool_count === 1 ? '' : 's'}`)
      } else {
        toast(status.error ? `Connection failed: ${status.error}` : 'Connection failed', 'danger')
      }
    },
    onError: (error: Error) => toast(`Connection failed: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })
  const authorize = useMutation({
    mutationFn: authorizeMCPServer,
    onSuccess: (status) => {
      if (status.status === 'connected') {
        toast(`Connected, ${status.tool_count} tool${status.tool_count === 1 ? '' : 's'}`)
      } else {
        toast(status.error ? `Sign-in failed: ${status.error}` : 'Sign-in failed', 'danger')
      }
    },
    onError: (error: Error) => toast(`Sign-in failed: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })

  const openAdd = () => {
    save.reset()
    setDraft(emptyDraft())
  }
  const openEdit = (server: MCPServer) => {
    save.reset()
    setDraft(draftFromServer(server))
  }
  const close = () => {
    save.reset()
    setDraft(null)
  }

  const sortedServers = useMemo(
    () => [...(servers.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [servers.data],
  )

  const busy = toggle.isPending || remove.isPending || test.isPending || authorize.isPending
  const isEdit = Boolean(draft?.id)
  const canSave = draft != null && draft.name.trim() !== '' && draft.url.trim() !== ''

  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">MCP servers</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Remote Streamable HTTP connections available to Jaz and capable ACP agents.
          </p>
        </div>
        <Button variant="secondary" size="md" onClick={openAdd}>
          <Plus size={14} />
          Add server
        </Button>
      </div>

      <div className="mt-4">
        {servers.isPending ? (
          <SkeletonRows count={3} />
        ) : servers.isError ? (
          <p className="py-2 text-[13px] text-danger">{servers.error.message}</p>
        ) : sortedServers.length === 0 ? (
          <DashedCta
            title="No MCP servers yet"
            subtitle="Connect a remote Streamable HTTP MCP server to give Jaz and capable agents access to its tools."
            onClick={openAdd}
          />
        ) : (
          <div className="flex flex-col gap-px">
            {sortedServers.map((server) => (
              <MCPServerRow
                key={server.id}
                server={server}
                busy={busy}
                authorizing={authorize.isPending && authorize.variables === server.id}
                onEdit={() => openEdit(server)}
                onToggle={() => toggle.mutate({ id: server.id, enabled: !server.enabled })}
                onDelete={() => {
                  if (window.confirm(`Delete ${server.name}?`)) remove.mutate(server.id)
                }}
                onTest={() => test.mutate(server.id)}
                onAuthorize={() => authorize.mutate(server.id)}
              />
            ))}
          </div>
        )}
      </div>

      <Modal
        open={draft !== null}
        onClose={close}
        size="md"
        title={isEdit ? 'Edit MCP server' : 'Add MCP server'}
        description={
          isEdit
            ? 'Update the connection details for this server.'
            : 'Connect a remote Streamable HTTP MCP server.'
        }
        footer={
          <>
            <p className="min-w-0 truncate text-[12px] text-danger" role="alert">
              {save.isError ? save.error.message : ''}
            </p>
            <div className="flex shrink-0 items-center gap-1">
              <Button variant="ghost" size="md" onClick={close}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="md"
                disabled={!canSave || save.isPending}
                onClick={() => draft && save.mutate(draft)}
              >
                {save.isPending ? 'Saving…' : isEdit ? 'Save changes' : 'Add server'}
              </Button>
            </div>
          </>
        }
      >
        {draft ? <MCPServerForm draft={draft} onChange={setDraft} /> : null}
      </Modal>
    </section>
  )
}

function MCPServerRow({
  server,
  busy,
  authorizing,
  onEdit,
  onToggle,
  onDelete,
  onTest,
  onAuthorize,
}: {
  server: MCPServer
  busy: boolean
  authorizing: boolean
  onEdit: () => void
  onToggle: () => void
  onDelete: () => void
  onTest: () => void
  onAuthorize: () => void
}) {
  const needsAuth = server.status === 'needs_auth'
  const canAuthorize = needsAuth || Boolean(server.oauth?.client_id || server.oauth?.issuer)
  return (
    <div className="flex items-center gap-3 rounded-card px-3 py-2 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface">
      <StatusIcon server={server} authorizing={authorizing} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate font-medium text-ink" title={server.name}>
            {server.name}
          </span>
          <span className="rounded-full bg-surface-2 px-1.5 py-[3px] text-[11px] leading-none text-ink-2">HTTP</span>
        </div>
        <p className="mt-0.5 truncate text-[12px] text-ink-3" title={server.url}>
          {server.url}
        </p>
      </div>
      <span className="hidden shrink-0 text-[12px] text-ink-3 sm:inline">
        {authorizing ? 'Waiting for sign-in…' : statusText(server)}
      </span>
      {canAuthorize ? (
        <Button
          variant="secondary"
          size="sm"
          aria-label="Sign in to MCP server"
          title="Sign in"
          disabled={busy || !server.enabled}
          onClick={onAuthorize}
        >
          <KeyRound size={13} />
          Sign in
        </Button>
      ) : (
        <IconButton
          variant="ghost"
          size="sm"
          aria-label="Test MCP server"
          title="Test MCP server"
          disabled={busy || !server.enabled}
          onClick={onTest}
        >
          <RefreshCcw size={14} />
        </IconButton>
      )}
      <IconButton
        variant="ghost"
        size="sm"
        aria-label={server.enabled ? 'Disable MCP server' : 'Enable MCP server'}
        title={server.enabled ? 'Disable MCP server' : 'Enable MCP server'}
        disabled={busy}
        onClick={onToggle}
      >
        <Power size={14} />
      </IconButton>
      <IconButton
        variant="ghost"
        size="sm"
        aria-label="Edit MCP server"
        title="Edit MCP server"
        disabled={busy}
        onClick={onEdit}
      >
        <Pencil size={14} />
      </IconButton>
      <IconButton
        variant="danger"
        size="sm"
        aria-label="Delete MCP server"
        title="Delete MCP server"
        disabled={busy}
        onClick={onDelete}
      >
        <Trash2 size={14} />
      </IconButton>
    </div>
  )
}

function StatusIcon({ server, authorizing }: { server: MCPServer; authorizing: boolean }) {
  if (authorizing) return <Loader2 size={15} className="animate-spin text-ink-3" />
  if (!server.enabled) return <span className="size-2 rounded-full bg-ink-3/45" />
  if (server.status === 'connected') return <CheckCircle2 size={15} className="text-ok" />
  if (server.status === 'needs_auth') return <KeyRound size={14} className="text-accent" />
  if (server.status === 'error') return <CircleAlert size={15} className="text-danger" />
  return <span className="size-2 rounded-full bg-running" />
}

function statusText(server: MCPServer): string {
  if (!server.enabled) return 'Disabled'
  if (server.status === 'connected') {
    return `${server.tool_count} tool${server.tool_count === 1 ? '' : 's'}`
  }
  if (server.status === 'needs_auth') return 'Sign in required'
  if (server.status === 'error') return server.error || 'Connection error'
  return 'Not checked'
}

function MCPServerForm({
  draft,
  onChange,
}: {
  draft: Draft
  onChange: (draft: Draft) => void
}) {
  const headerCount = (draft.headers?.length ?? 0) + (draft.env_headers?.length ?? 0)
  const oauthSet = Boolean(
    draft.oauth?.client_id || draft.oauth?.client_secret_env_var || draft.oauth?.issuer,
  )
  const [advanced, setAdvanced] = useState(headerCount > 0 || oauthSet)

  return (
    <div className="space-y-4">
      <Field label="Name">
        <Input
          placeholder="Linear"
          value={draft.name}
          onChange={(event) => onChange({ ...draft, name: event.target.value })}
        />
      </Field>

      <Field label="Server URL" hint="The remote Streamable HTTP endpoint.">
        <Input
          placeholder="https://mcp.example.com/mcp"
          value={draft.url}
          onChange={(event) => onChange({ ...draft, url: event.target.value })}
        />
      </Field>

      <Field label="Bearer token" hint="Optional — read from this environment variable.">
        <Input
          placeholder="MCP_TOKEN"
          value={draft.bearer_token_env_var ?? ''}
          onChange={(event) => onChange({ ...draft, bearer_token_env_var: event.target.value })}
        />
      </Field>

      <div className="border-t border-border">
        <button
          type="button"
          onClick={() => setAdvanced((open) => !open)}
          className="flex w-full cursor-pointer items-center gap-1.5 py-2.5 text-[12px] font-medium text-ink-2 transition-colors duration-150 hover:text-ink"
        >
          <ChevronRight
            size={13}
            className={`transition-transform duration-200 ${advanced ? 'rotate-90' : ''}`}
          />
          Advanced
          {(headerCount > 0 || oauthSet) && !advanced ? (
            <span className="font-normal text-ink-3">
              · {headerCount + (oauthSet ? 1 : 0)} set
            </span>
          ) : null}
        </button>
        <AnimatePresence initial={false}>
          {advanced ? (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.18, ease: 'easeOut' }}
              className="overflow-hidden"
            >
              <div className="space-y-4 pb-1">
                <OAuthEditor
                  value={draft.oauth ?? {}}
                  onChange={(oauth) => onChange({ ...draft, oauth })}
                />
                <HeaderEditor
                  title="Custom headers"
                  headers={draft.headers ?? []}
                  onChange={(headers) => onChange({ ...draft, headers })}
                />
                <EnvHeaderEditor
                  title="Headers from environment variables"
                  headers={draft.env_headers ?? []}
                  onChange={(envHeaders) => onChange({ ...draft, env_headers: envHeaders })}
                />
              </div>
            </motion.div>
          ) : null}
        </AnimatePresence>
      </div>

      <div className="flex items-center gap-2.5 border-t border-border pt-4 text-[13px] text-ink-2">
        <Switch
          checked={draft.enabled}
          onChange={(enabled) => onChange({ ...draft, enabled })}
          aria-label="Enabled"
        />
        <span>
          Enabled <span className="text-ink-3">— make its tools available to agents</span>
        </span>
      </div>
    </div>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
      {hint ? <span className="mt-1 block text-[12px] text-ink-3">{hint}</span> : null}
    </label>
  )
}

function OAuthEditor({
  value,
  onChange,
}: {
  value: MCPOAuthConfig
  onChange: (value: MCPOAuthConfig) => void
}) {
  return (
    <div className="space-y-2">
      <p className="text-[12px] font-medium text-ink-2">OAuth</p>
      <div className="grid gap-2 sm:grid-cols-2">
        <Input
          placeholder="Client ID"
          value={value.client_id ?? ''}
          onChange={(event) => onChange({ ...value, client_id: event.target.value })}
        />
        <Input
          placeholder="Client secret env var"
          value={value.client_secret_env_var ?? ''}
          onChange={(event) =>
            onChange({ ...value, client_secret_env_var: event.target.value })
          }
        />
      </div>
      <Input
        placeholder="Issuer override"
        value={value.issuer ?? ''}
        onChange={(event) => onChange({ ...value, issuer: event.target.value })}
      />
    </div>
  )
}

function HeaderEditor({
  title,
  headers,
  onChange,
}: {
  title: string
  headers: MCPHeader[]
  onChange: (headers: MCPHeader[]) => void
}) {
  return (
    <div className="space-y-2">
      <p className="text-[12px] font-medium text-ink-2">{title}</p>
      <div className="flex flex-col gap-2">
        {headers.map((header, index) => (
          <div key={index} className="grid gap-2 sm:grid-cols-[1fr_1fr_28px]">
            <Input
              placeholder="Header"
              value={header.name}
              onChange={(event) =>
                replaceHeader(headers, index, { ...header, name: event.target.value }, onChange)
              }
            />
            <Input
              placeholder="Value"
              value={header.value}
              onChange={(event) =>
                replaceHeader(headers, index, { ...header, value: event.target.value }, onChange)
              }
            />
            <RemoveButton onClick={() => onChange(headers.filter((_, i) => i !== index))} />
          </div>
        ))}
        <AddRowButton onClick={() => onChange([...headers, { name: '', value: '' }])}>
          Add header
        </AddRowButton>
      </div>
    </div>
  )
}

function EnvHeaderEditor({
  title,
  headers,
  onChange,
}: {
  title: string
  headers: MCPEnvHeader[]
  onChange: (headers: MCPEnvHeader[]) => void
}) {
  return (
    <div className="space-y-2">
      <p className="text-[12px] font-medium text-ink-2">{title}</p>
      <div className="flex flex-col gap-2">
        {headers.map((header, index) => (
          <div key={index} className="grid gap-2 sm:grid-cols-[1fr_1fr_28px]">
            <Input
              placeholder="Header"
              value={header.name}
              onChange={(event) =>
                replaceEnvHeader(headers, index, { ...header, name: event.target.value }, onChange)
              }
            />
            <Input
              placeholder="ENV_VAR"
              value={header.env_var}
              onChange={(event) =>
                replaceEnvHeader(
                  headers,
                  index,
                  { ...header, env_var: event.target.value },
                  onChange,
                )
              }
            />
            <RemoveButton onClick={() => onChange(headers.filter((_, i) => i !== index))} />
          </div>
        ))}
        <AddRowButton onClick={() => onChange([...headers, { name: '', env_var: '' }])}>
          Add variable
        </AddRowButton>
      </div>
    </div>
  )
}

function replaceHeader(
  headers: MCPHeader[],
  index: number,
  header: MCPHeader,
  onChange: (headers: MCPHeader[]) => void,
) {
  const next = [...headers]
  next[index] = header
  onChange(next)
}

function replaceEnvHeader(
  headers: MCPEnvHeader[],
  index: number,
  header: MCPEnvHeader,
  onChange: (headers: MCPEnvHeader[]) => void,
) {
  const next = [...headers]
  next[index] = header
  onChange(next)
}

function RemoveButton({ onClick }: { onClick: () => void }) {
  return (
    <IconButton
      variant="ghost"
      size="sm"
      className="self-center justify-self-end"
      aria-label="Remove row"
      title="Remove row"
      onClick={onClick}
    >
      <Trash2 size={14} />
    </IconButton>
  )
}

function AddRowButton({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return (
    <button
      type="button"
      className="-ml-1 flex cursor-pointer items-center gap-1.5 rounded-full px-2 py-1 text-[12px] font-medium text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      onClick={onClick}
    >
      <Plus size={13} />
      {children}
    </button>
  )
}
