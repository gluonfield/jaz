import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, FileCode, Folder, Globe, Lock, ShieldCheck, UserCheck, Users } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/toast'
import {
  createAppDraft,
  publishApp,
  type AppDraft,
  type AppKind,
  type AppVisibility,
  type JazApp,
} from '@/lib/api/apps'
import { orgMembersQuery } from '@/lib/api/cloudMembers'
import { spacesQuery } from '@/lib/api/cloudSpaces'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const CAPABILITY_OPTIONS = [
  { id: 'storage:personal', label: 'Personal storage (20 MB)' },
  { id: 'storage:shared', label: 'Shared storage (Cloud org/Space)' },
  { id: 'ai:complete', label: 'AI completions (viewer-billed)' },
  { id: 'tools:call', label: 'Allowlisted MCP tools' },
] as const

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}

export function PublishAppModal({
  open,
  onClose,
  initialTitle,
  initialDescription = '',
  kind,
  html,
  markdown,
  entry,
  sourceThreadId,
  targetApp,
  initialDraft,
  onPublished,
}: {
  open: boolean
  onClose: () => void
  initialTitle: string
  initialDescription?: string
  kind: AppKind
  html?: string
  markdown?: string
  entry?: string
  sourceThreadId?: string
  targetApp?: JazApp | null
  initialDraft?: AppDraft | null
  onPublished?: (app: JazApp) => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const isCloud = cloud.active
  const hasRestageSource = Boolean(html?.trim() || markdown?.trim())

  const [title, setTitle] = useState(targetApp?.title ?? initialDraft?.title ?? initialTitle)
  const [description, setDescription] = useState(targetApp?.description ?? initialDraft?.description ?? initialDescription)
  const [visibility, setVisibility] = useState<AppVisibility>(targetApp?.visibility ?? 'private')
  const [spaceId, setSpaceId] = useState<string>(targetApp?.space_id ?? '')
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>(
    targetApp?.members?.map((m) => m.user_id) ?? [],
  )
  const [capabilities, setCapabilities] = useState<string[]>(
    targetApp?.capabilities ?? initialDraft?.capabilities ?? ['storage:personal'],
  )
  const [allowedToolsInput, setAllowedToolsInput] = useState(
    (targetApp?.allowed_tools ?? initialDraft?.allowed_tools ?? []).join(', '),
  )
  const [draft, setDraft] = useState<AppDraft | null>(initialDraft ?? null)
  const [confirmDigest, setConfirmDigest] = useState(false)

  const spaces = useQuery({ ...spacesQuery, enabled: open && isCloud })
  const members = useQuery({ ...orgMembersQuery(), enabled: open && isCloud && visibility === 'selected' })

  useEffect(() => {
    if (!open) return
    setTitle(targetApp?.title ?? initialDraft?.title ?? initialTitle)
    setDescription(targetApp?.description ?? initialDraft?.description ?? initialDescription)
    setVisibility(targetApp?.visibility ?? 'private')
    setSpaceId(targetApp?.space_id ?? '')
    setSelectedUserIds(targetApp?.members?.map((m) => m.user_id) ?? [])
    setCapabilities(targetApp?.capabilities ?? initialDraft?.capabilities ?? ['storage:personal'])
    setAllowedToolsInput((targetApp?.allowed_tools ?? initialDraft?.allowed_tools ?? []).join(', '))
    setDraft(initialDraft ?? null)
    setConfirmDigest(false)
  }, [open, initialTitle, initialDescription, targetApp, initialDraft])

  const stageMutation = useMutation({
    mutationFn: async () => {
      const allowedTools = allowedToolsInput
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      const res = await createAppDraft({
        target_app_id: targetApp?.id,
        source_thread_id: sourceThreadId,
        title: title.trim() || 'Untitled App',
        description: description.trim(),
        kind,
        entry: entry || (kind === 'document' ? 'index.md' : 'index.html'),
        html: kind === 'artifact' ? html : undefined,
        markdown: kind === 'document' ? markdown : undefined,
        capabilities,
        allowed_tools: capabilities.includes('tools:call') ? allowedTools : [],
      })
      return res.draft
    },
    onSuccess: (staged) => {
      setDraft(staged)
      setConfirmDigest(false)
    },
  })

  const publishMutation = useMutation({
    mutationFn: async () => {
      if (!draft) throw new Error('Run preflight check first')
      return publishApp(targetApp?.id ?? draft.target_app_id ?? 'new', {
        draft_id: draft.draft_id,
        bundle_sha256: draft.bundle_sha256,
        title: title.trim() || draft.title,
        description: description.trim(),
        visibility: isCloud ? visibility : 'private',
        space_id: isCloud && visibility === 'space' ? spaceId || null : null,
        member_user_ids: isCloud && visibility === 'selected' ? selectedUserIds : undefined,
      })
    },
    onSuccess: ({ app }) => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast(targetApp ? `Published v${app.current_version} of ${app.title}` : `Published "${app.title}"`)
      onPublished?.(app)
      onClose()
    },
  })

  const toggleCap = (cap: string) => {
    if (!hasRestageSource) return
    setDraft(null)
    setCapabilities((cur) => (cur.includes(cap) ? cur.filter((c) => c !== cap) : [...cur, cap]))
  }

  const visibilityOptions: { id: AppVisibility; label: string; Icon: typeof Lock }[] = isCloud
    ? [
        { id: 'private', label: 'Private', Icon: Lock },
        { id: 'selected', label: 'Selected people', Icon: UserCheck },
        { id: 'space', label: 'Space', Icon: Folder },
        { id: 'team', label: 'Team (Org)', Icon: Users },
      ]
    : [{ id: 'private', label: 'Private (Local)', Icon: Lock }]

  const canPublish =
    Boolean(draft) &&
    confirmDigest &&
    (!isCloud || visibility !== 'space' || Boolean(spaceId)) &&
    !publishMutation.isPending

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="lg"
      title={targetApp ? `Publish update · ${targetApp.title}` : 'Publish Jaz App'}
      description="Stage a validated jaz-app/1 bundle, inspect its preflight audit and SHA-256 digest, and choose authenticated sharing access."
      footer={
        <>
          <div className="min-w-0 truncate text-[12px] text-danger">
            {stageMutation.isError
              ? (stageMutation.error as Error).message
              : publishMutation.isError
                ? (publishMutation.error as Error).message
                : ''}
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button variant="ghost" size="md" onClick={onClose}>
              Cancel
            </Button>
            {!draft ? (
              <Button
                variant="primary"
                size="md"
                disabled={stageMutation.isPending || !title.trim()}
                onClick={() => stageMutation.mutate()}
              >
                {stageMutation.isPending ? 'Auditing bundle…' : 'Run preflight check'}
              </Button>
            ) : (
              <Button
                variant="primary"
                size="md"
                disabled={!canPublish}
                onClick={() => publishMutation.mutate()}
              >
                {publishMutation.isPending
                  ? 'Publishing…'
                  : targetApp
                    ? `Publish v${targetApp.current_version + 1}`
                    : 'Confirm & Publish'}
              </Button>
            )}
          </div>
        </>
      }
    >
      <div className="space-y-4 text-[13px]">
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-[12px] font-medium text-ink-2">App title</label>
            <Input
              value={title}
              onChange={(e) => {
                setTitle(e.target.value)
                if (hasRestageSource) setDraft(null)
              }}
              placeholder="e.g. Budget Calculator"
            />
          </div>
          <div>
            <label className="mb-1 block text-[12px] font-medium text-ink-2">Description</label>
            <Input
              value={description}
              onChange={(e) => {
                setDescription(e.target.value)
                if (hasRestageSource) setDraft(null)
              }}
              placeholder="Short summary shown in Apps library"
            />
          </div>
        </div>

        <div>
          <label className="mb-1.5 block text-[12px] font-medium text-ink-2">
            Sharing access (authenticated org/Space only)
          </label>
          <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
            {visibilityOptions.map(({ id, label, Icon }) => {
              const active = visibility === id
              return (
                <button
                  key={id}
                  type="button"
                  onClick={() => setVisibility(id)}
                  className={`flex cursor-pointer items-center gap-1.5 rounded-control border px-2.5 py-2 text-[12px] font-medium transition-colors ${
                    active
                      ? 'border-primary bg-primary-soft text-ink'
                      : 'border-border bg-bg text-ink-2 hover:bg-surface'
                  }`}
                >
                  <Icon size={13} aria-hidden />
                  <span className="truncate">{label}</span>
                </button>
              )
            })}
          </div>

          {isCloud && visibility === 'space' ? (
            <div className="mt-2">
              <select
                value={spaceId}
                onChange={(e) => setSpaceId(e.target.value)}
                aria-label="Target space"
                className="w-full rounded-control border border-border bg-bg px-2.5 py-1.5 text-[12.5px] text-ink outline-none focus:border-primary"
              >
                <option value="">Select a Space…</option>
                {(spaces.data ?? []).map((sp) => (
                  <option key={sp.id} value={sp.id}>
                    {sp.name}
                  </option>
                ))}
              </select>
            </div>
          ) : null}

          {isCloud && visibility === 'selected' ? (
            <div className="mt-2 max-h-32 space-y-1 overflow-y-auto rounded-control border border-border bg-surface p-2">
              {(members.data ?? []).length === 0 ? (
                <p className="text-[12px] text-ink-3">No other org members found.</p>
              ) : (
                (members.data ?? []).map((m) => {
                  const checked = selectedUserIds.includes(m.id)
                  return (
                    <label key={m.id} className="flex cursor-pointer items-center gap-2 text-[12px]">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() =>
                          setSelectedUserIds((cur) =>
                            cur.includes(m.id) ? cur.filter((id) => id !== m.id) : [...cur, m.id],
                          )
                        }
                        className="accent-primary"
                      />
                      <span className="truncate font-medium text-ink">{m.display_name || m.email}</span>
                      <span className="truncate text-ink-3">{m.email}</span>
                    </label>
                  )
                })
              )}
            </div>
          ) : null}
        </div>

        <div>
          <label className="mb-1.5 block text-[12px] font-medium text-ink-2">Declared runtime capabilities</label>
          <div className="grid gap-1.5 sm:grid-cols-2">
            {CAPABILITY_OPTIONS.map((opt) => {
              const disabled = !hasRestageSource || (opt.id === 'storage:shared' && !isCloud)
              return (
                <label
                  key={opt.id}
                  className={`flex items-center gap-2 rounded-control border border-border px-2.5 py-1.5 text-[12px] ${
                    disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer hover:bg-surface'
                  }`}
                >
                  <input
                    type="checkbox"
                    disabled={disabled}
                    checked={capabilities.includes(opt.id) && (opt.id !== 'storage:shared' || isCloud)}
                    onChange={() => toggleCap(opt.id)}
                    className="accent-primary"
                  />
                  <span className="truncate text-ink">{opt.label}</span>
                </label>
              )
            })}
          </div>
          {capabilities.includes('tools:call') ? (
            <div className="mt-2">
              <Input
                value={allowedToolsInput}
                disabled={!hasRestageSource}
                onChange={(e) => {
                  setAllowedToolsInput(e.target.value)
                  if (hasRestageSource) setDraft(null)
                }}
                placeholder="Explicit allowed_tools (comma-separated, e.g. crm_search, tasks_list)"
              />
            </div>
          ) : null}
        </div>

        {draft ? (
          <div className="space-y-3 rounded-card border border-border bg-surface p-3.5">
            <div className="flex items-center justify-between gap-2">
              <span className="flex items-center gap-1.5 text-[12.5px] font-semibold text-ink">
                <ShieldCheck size={15} className="text-primary" aria-hidden />
                Preflight Audit Card
              </span>
              <span className="rounded bg-bg px-2 py-0.5 font-mono text-[11px] text-ink-2">
                {draft.file_count} files · {formatBytes(draft.size_bytes)}
              </span>
            </div>

            <div className="space-y-1">
              <p className="text-[11px] font-medium uppercase tracking-wide text-ink-3">Bundle files</p>
              <ul className="max-h-28 space-y-1 overflow-y-auto rounded border border-border bg-bg p-2">
                {draft.files.map((f) => (
                  <li key={f.path} className="flex items-center justify-between gap-2 font-mono text-[11px]">
                    <span className="flex min-w-0 items-center gap-1.5 truncate text-ink">
                      <FileCode size={12} className="shrink-0 text-ink-3" />
                      {f.path}
                    </span>
                    <span className="shrink-0 text-ink-3">{formatBytes(f.size_bytes)}</span>
                  </li>
                ))}
              </ul>
            </div>

            {draft.external_urls.length > 0 ? (
              <div className="space-y-1">
                <p className="text-[11px] font-medium uppercase tracking-wide text-ink-3">External URLs</p>
                <ul className="max-h-24 space-y-1 overflow-y-auto rounded border border-border bg-bg p-2 text-[11px]">
                  {draft.external_urls.map((u) => (
                    <li key={`${u.source_file}:${u.url}`} className="flex items-center justify-between gap-2">
                      <span className="flex min-w-0 items-center gap-1.5 truncate font-mono text-ink-2">
                        <Globe size={11} className="shrink-0 text-ink-3" />
                        {u.url}
                      </span>
                      <span
                        className={`shrink-0 rounded-full px-1.5 py-px text-[10px] font-medium ${
                          u.allowlisted
                            ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
                            : 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
                        }`}
                      >
                        {u.allowlisted ? 'CDN allowlisted' : 'Blocked by CSP'}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}

            {draft.warnings.length > 0 ? (
              <div className="space-y-1 rounded border border-amber-500/35 bg-amber-500/10 p-2.5">
                <p className="flex items-center gap-1 text-[11.5px] font-semibold text-amber-700 dark:text-amber-300">
                  <AlertTriangle size={12} aria-hidden />
                  Preflight warnings
                </p>
                <ul className="list-disc space-y-0.5 pl-4 text-[11.5px] text-ink-2">
                  {draft.warnings.map((w) => (
                    <li key={w}>{w}</li>
                  ))}
                </ul>
              </div>
            ) : null}

            <div className="rounded border border-border bg-bg px-2.5 py-2 font-mono text-[11px] text-ink-2">
              <span className="text-ink-3">bundle_sha256: </span>
              <span className="break-all text-ink">{draft.bundle_sha256}</span>
            </div>

            <label className="flex cursor-pointer items-center gap-2 text-[12px] font-medium text-ink">
              <input
                type="checkbox"
                checked={confirmDigest}
                onChange={(e) => setConfirmDigest(e.target.checked)}
                className="accent-primary"
              />
              <span className="flex items-center gap-1">
                <Check size={12} className="text-primary" />
                I reviewed the files, warnings, and bundle digest above
              </span>
            </label>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
