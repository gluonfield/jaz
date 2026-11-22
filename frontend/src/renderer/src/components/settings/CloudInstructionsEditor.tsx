import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Lock, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Segmented } from '@/components/ui/Segmented'
import { Skeleton } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { botsQuery } from '@/lib/api/bots'
import {
  cloudInstructionQuery,
  getInstruction,
  INSTRUCTION_KIND_FILENAME,
  INSTRUCTION_SCOPE_KINDS,
  saveInstruction,
  spacesQuery,
  type CloudInstructionDocument,
  type InstructionKind,
  type InstructionScope,
} from '@/lib/api/cloud'
import { ApiError } from '@/lib/api/response'
import { useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'
import { FileTabs } from './FileTabs'

const SCOPE_OPTIONS: { value: InstructionScope; label: string }[] = [
  { value: 'org', label: 'Org' },
  { value: 'space', label: 'Space' },
  { value: 'user', label: 'Personal' },
  { value: 'bot', label: 'Bot' },
]

const KIND_DESCRIPTIONS: Record<InstructionKind, string> = {
  agents: 'Standing instructions applied across matching threads.',
  soul: 'Assistant personality, tone, and working values.',
  bots: 'Coordination and delegation rules for bot threads.',
  memory: 'Durable working memory for this bot.',
}

export function CloudInstructionsEditor() {
  const { user } = useConnection()
  const queryClient = useQueryClient()
  const toast = useToast()

  const [scope, setScope] = useState<InstructionScope>('user')
  const [selectedSpaceId, setSelectedSpaceId] = useState('')
  const [selectedBotId, setSelectedBotId] = useState('')
  const [selectedKind, setSelectedKind] = useState<InstructionKind>('agents')
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [conflictKey, setConflictKey] = useState<string | null>(null)
  const [editorResetSeq, setEditorResetSeq] = useState(0)

  const spaces = useQuery({ ...spacesQuery, enabled: scope === 'space' })
  const bots = useQuery({ ...botsQuery, enabled: scope === 'bot' })

  const validKinds = INSTRUCTION_SCOPE_KINDS[scope]
  const kind: InstructionKind = validKinds.includes(selectedKind) ? selectedKind : validKinds[0]

  const scopeId =
    scope === 'org'
      ? user?.org_id || 'org'
      : scope === 'user'
        ? user?.id || 'me'
        : scope === 'space'
          ? selectedSpaceId || spaces.data?.[0]?.id || ''
          : selectedBotId || bots.data?.[0]?.id || ''

  const docKey = `${scope}:${scopeId}:${kind}`
  const docQuery = useQuery(cloudInstructionQuery(scope, scopeId, kind))
  const readOnly = (scope === 'org' && user?.role !== 'admin') || docQuery.data?.read_only === true

  const save = useMutation({
    mutationFn: ({ content, expectedVersion }: { content: string; expectedVersion: string }) =>
      saveInstruction(scope, scopeId, kind, { content, expected_version: expectedVersion }),
    onSuccess: (saved: CloudInstructionDocument) => {
      queryClient.setQueryData(keys.cloudInstruction(scope, scopeId, kind), saved)
      setDrafts((prev) => {
        const next = { ...prev }
        delete next[docKey]
        return next
      })
      setConflictKey(null)
      toast(`Saved ${INSTRUCTION_KIND_FILENAME[kind]}`)
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.status === 409) {
        setConflictKey(docKey)
        return
      }
      toast(`Couldn't save ${INSTRUCTION_KIND_FILENAME[kind]}: ${error.message}`, 'danger')
    },
  })

  const reloadLatest = async () => {
    setConflictKey(null)
    setDrafts((prev) => {
      const next = { ...prev }
      delete next[docKey]
      return next
    })
    await queryClient.invalidateQueries({ queryKey: keys.cloudInstruction(scope, scopeId, kind) })
    setEditorResetSeq((n) => n + 1)
  }

  const overwriteWithMine = async () => {
    const draft = drafts[docKey]
    if (draft === undefined || save.isPending) return
    try {
      const latest = await getInstruction(scope, scopeId, kind)
      queryClient.setQueryData(keys.cloudInstruction(scope, scopeId, kind), latest)
      save.mutate({ content: draft, expectedVersion: latest.version })
    } catch (error) {
      toast((error as Error).message || 'Could not reload version', 'danger')
    }
  }

  const doc = docQuery.data
  const draft = drafts[docKey]
  const value = draft ?? doc?.content ?? ''
  const dirty = !readOnly && draft !== undefined && draft !== (doc?.content ?? '')
  const hasConflict = conflictKey === docKey

  const handleSave = () => {
    if (readOnly || save.isPending || !doc) return
    save.mutate({ content: value, expectedVersion: doc.version })
  }

  return (
    <div className="mx-auto flex h-full max-w-[860px] flex-col px-10 pb-8 pt-2 max-sm:px-4">
      <header className="flex flex-wrap items-start justify-between gap-4 pb-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-lg font-semibold text-ink">Instructions</h1>
            {readOnly ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-medium text-ink-2">
                <Lock size={11} />
                Read-only · Org admin required
              </span>
            ) : null}
          </div>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">
            {KIND_DESCRIPTIONS[kind]} Saved versions sync immediately to cloud turns.
          </p>
        </div>

        <div className="flex h-8 shrink-0 items-center">
          {readOnly ? null : dirty || save.isPending ? (
            <Button variant="primary" size="md" onClick={handleSave} disabled={save.isPending}>
              {save.isPending ? 'Saving…' : 'Save changes'}
              <kbd className="font-mono text-[11px] text-on-primary/75">⌘S</kbd>
            </Button>
          ) : (
            <span className="text-[12px] text-ink-3">All changes saved</span>
          )}
        </div>
      </header>

      <div className="mb-3 flex flex-wrap items-center gap-2.5">
        <Segmented
          layoutId="cloud-instruction-scope"
          value={scope}
          options={SCOPE_OPTIONS}
          onChange={(next) => {
            setScope(next)
            setConflictKey(null)
          }}
        />

        {scope === 'space' && (spaces.data?.length ?? 0) > 0 ? (
          <select
            aria-label="Select space"
            value={scopeId}
            onChange={(event) => {
              setSelectedSpaceId(event.target.value)
              setConflictKey(null)
            }}
            className="h-8 cursor-pointer rounded-full bg-surface px-3 text-[12.5px] font-medium text-ink outline-none ring-1 ring-border/70 focus-visible:ring-primary"
          >
            {spaces.data!.map((space) => (
              <option key={space.id} value={space.id}>
                {space.name}
              </option>
            ))}
          </select>
        ) : null}

        {scope === 'bot' && (bots.data?.length ?? 0) > 0 ? (
          <select
            aria-label="Select bot"
            value={scopeId}
            onChange={(event) => {
              setSelectedBotId(event.target.value)
              setConflictKey(null)
            }}
            className="h-8 cursor-pointer rounded-full bg-surface px-3 text-[12.5px] font-medium text-ink outline-none ring-1 ring-border/70 focus-visible:ring-primary"
          >
            {bots.data!.map((bot) => (
              <option key={bot.id} value={bot.id}>
                {bot.name}
              </option>
            ))}
          </select>
        ) : null}
      </div>

      {scope === 'space' && !spaces.isPending && (spaces.data?.length ?? 0) === 0 ? (
        <EmptyState title="No spaces yet">
          <p>Create a Space in the sidebar to manage shared Space instructions.</p>
        </EmptyState>
      ) : scope === 'bot' && !bots.isPending && (bots.data?.length ?? 0) === 0 ? (
        <EmptyState title="No bots yet">
          <p>Create a bot to edit its BOTS.md and MEMORY.md instructions.</p>
        </EmptyState>
      ) : (
        <>
          <FileTabs
            className="border-b border-border"
            underlineId="cloud-instructions-tab-underline"
            active={INSTRUCTION_KIND_FILENAME[kind]}
            onSelect={(filename) => {
              const found = validKinds.find((item) => INSTRUCTION_KIND_FILENAME[item] === filename)
              if (found) {
                setSelectedKind(found)
                setConflictKey(null)
              }
            }}
            tabs={validKinds.map((item) => {
              const key = `${scope}:${scopeId}:${item}`
              const itemDraft = drafts[key]
              return {
                name: INSTRUCTION_KIND_FILENAME[item],
                dirty: itemDraft !== undefined && item === kind ? itemDraft !== (doc?.content ?? '') : itemDraft !== undefined,
                badge: scope === 'org' && readOnly ? 'locked' : undefined,
              }
            })}
          />

          {hasConflict ? (
            <div
              role="alert"
              className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-accent-soft px-3.5 py-2 text-[12.5px] text-accent-strong"
            >
              <span className="flex items-center gap-2">
                <AlertTriangle size={14} className="shrink-0" />
                {INSTRUCTION_KIND_FILENAME[kind]} was updated elsewhere (409 Conflict). Reload to see the latest version.
              </span>
              <span className="flex items-center gap-1.5">
                <Button size="sm" variant="secondary" onClick={() => void reloadLatest()}>
                  <RefreshCw size={12} />
                  Reload latest
                </Button>
                <Button size="sm" variant="primary" onClick={() => void overwriteWithMine()} disabled={save.isPending}>
                  Overwrite with mine
                </Button>
              </span>
            </div>
          ) : null}

          <div className="min-h-0 flex-1 overflow-hidden rounded-b-card bg-surface">
            {docQuery.isPending ? (
              <div className="p-5">
                <Skeleton className="mb-3 h-5 w-48" />
                <Skeleton className="h-64" />
              </div>
            ) : docQuery.isError ? (
              <div className="p-6">
                <EmptyState title="Couldn't load instructions">
                  <p>{docQuery.error.message}</p>
                </EmptyState>
              </div>
            ) : (
              <MarkdownEditor
                key={`${docKey}:${doc?.version ?? 'empty'}:${editorResetSeq}`}
                initialValue={value}
                readOnly={readOnly}
                placeholder={readOnly ? 'No organization instructions configured.' : KIND_DESCRIPTIONS[kind]}
                onChange={(nextDoc) => {
                  if (readOnly) return
                  setDrafts((prev) => {
                    if (nextDoc === (doc?.content ?? '')) {
                      if (!(docKey in prev)) return prev
                      const next = { ...prev }
                      delete next[docKey]
                      return next
                    }
                    return { ...prev, [docKey]: nextDoc }
                  })
                }}
                onSave={handleSave}
              />
            )}
          </div>
        </>
      )}
    </div>
  )
}
