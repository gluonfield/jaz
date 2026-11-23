import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { agentFilesQuery, saveAgentFile } from '@/lib/api/agentFiles'
import {
  canEditCloudInstruction,
  cloudInstructionQuery,
  extractInstructionConflictCurrent,
  getInstruction,
  saveInstruction,
  type CloudInstructionDocument,
} from '@/lib/api/cloudInstructions'
import { ApiError } from '@/lib/api/response'
import type { AgentFile, AgentFilesResponse } from '@/lib/api/types'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const DESCRIPTION =
  'Your standing AGENTS instructions shared across threads. Saved changes apply on the next turn.'

export function PersonalizationSettings() {
  const cloud = useCloudSession()
  if (cloud.active) {
    return <CloudAgentsEditor userId={cloud.user?.id || 'me'} readOnly={!canEditCloudInstruction(cloud.user, 'user')} />
  }
  return <LocalAgentsEditor />
}

function CloudAgentsEditor({ userId, readOnly }: { userId: string; readOnly: boolean }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState<string>()
  const [hasConflict, setHasConflict] = useState(false)
  const [editorResetSeq, setEditorResetSeq] = useState(0)

  const docQuery = useQuery(cloudInstructionQuery('user', userId, 'agents'))
  const doc = docQuery.data
  const locked = readOnly || doc?.read_only === true

  const save = useMutation({
    mutationFn: ({ body_md, expected_version }: { body_md: string; expected_version: string }) =>
      saveInstruction('user', userId, 'agents', { body_md, expected_version }),
    onSuccess: (saved: CloudInstructionDocument, vars) => {
      queryClient.setQueryData(keys.cloudInstruction('user', userId, 'agents'), saved)
      setDraft((prev) => (prev === vars.body_md ? undefined : prev))
      setHasConflict(false)
      toast('Saved AGENTS')
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.status === 409) {
        const current = extractInstructionConflictCurrent(error, 'user', userId, 'agents')
        if (current) {
          queryClient.setQueryData(keys.cloudInstruction('user', userId, 'agents'), current)
        }
        setHasConflict(true)
        return
      }
      toast(`Couldn't save AGENTS: ${error.message}`, 'danger')
    },
  })

  if (docQuery.isPending) {
    return <EditorSkeleton />
  }

  if (docQuery.isError) {
    return (
      <EmptyState title="Couldn't load instructions">
        <p>{docQuery.error.message}</p>
      </EmptyState>
    )
  }

  const savedBody = doc?.body_md ?? ''
  const value = draft ?? savedBody
  const dirty = !locked && draft !== undefined && draft !== savedBody

  const handleSave = () => {
    if (locked || save.isPending || !doc) return
    save.mutate({ body_md: value, expected_version: doc.version })
  }

  const reloadLatest = async () => {
    setHasConflict(false)
    setDraft(undefined)
    await queryClient.invalidateQueries({ queryKey: keys.cloudInstruction('user', userId, 'agents') })
    setEditorResetSeq((n) => n + 1)
  }

  const overwriteWithMine = async () => {
    if (draft === undefined || save.isPending) return
    try {
      const latest = await getInstruction('user', userId, 'agents')
      queryClient.setQueryData(keys.cloudInstruction('user', userId, 'agents'), latest)
      save.mutate({ body_md: draft, expected_version: latest.version })
    } catch (error) {
      toast((error as Error).message || 'Could not reload version', 'danger')
    }
  }

  return (
    <PersonalizationFrame
      dirty={dirty}
      saving={save.isPending}
      readOnly={locked}
      onSave={handleSave}
      banner={
        hasConflict ? (
          <div
            role="alert"
            className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-card border border-border bg-accent-soft px-3.5 py-2 text-[12.5px] text-accent-strong"
          >
            <span className="flex items-center gap-2">
              <AlertTriangle size={14} className="shrink-0" />
              AGENTS was updated elsewhere. Reload to see the latest version.
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
        ) : null
      }
    >
      <MarkdownEditor
        key={`cloud:${userId}:${doc?.version ?? 'empty'}:${editorResetSeq}`}
        initialValue={value}
        readOnly={locked}
        placeholder={DESCRIPTION}
        onChange={(nextDoc) => {
          if (locked) return
          setDraft(nextDoc === savedBody ? undefined : nextDoc)
        }}
        onSave={handleSave}
      />
    </PersonalizationFrame>
  )
}

function LocalAgentsEditor() {
  const files = useQuery(agentFilesQuery)
  const queryClient = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState<string>()

  const save = useMutation({
    mutationFn: (content: string) => saveAgentFile('AGENTS.md', content),
    onSuccess: (saved: AgentFile) => {
      queryClient.setQueryData<AgentFilesResponse>(keys.agentFiles, (prev) =>
        prev
          ? { ...prev, files: prev.files.map((f) => (f.name === saved.name ? saved : f)) }
          : prev,
      )
      setDraft(undefined)
      toast('Saved AGENTS')
    },
    onError: (error: Error) => {
      toast(`Couldn't save AGENTS: ${error.message}`, 'danger')
    },
  })

  if (files.isPending) {
    return <EditorSkeleton />
  }

  if (files.isError) {
    return (
      <EmptyState title="Couldn't load instructions">
        <p>{files.error.message}</p>
      </EmptyState>
    )
  }

  const agentsFile = files.data.files.find((f) => f.name === 'AGENTS.md')
  const savedBody = agentsFile?.content ?? ''
  const value = draft ?? savedBody
  const dirty = draft !== undefined && draft !== savedBody

  const handleSave = () => {
    if (save.isPending) return
    save.mutate(value)
  }

  return (
    <PersonalizationFrame dirty={dirty} saving={save.isPending} readOnly={false} onSave={handleSave}>
      <MarkdownEditor
        key="local:AGENTS.md"
        initialValue={value}
        placeholder={DESCRIPTION}
        onChange={(nextDoc) => setDraft(nextDoc === savedBody ? undefined : nextDoc)}
        onSave={handleSave}
      />
    </PersonalizationFrame>
  )
}

function PersonalizationFrame({
  dirty,
  saving,
  readOnly,
  onSave,
  banner,
  children,
}: {
  dirty: boolean
  saving: boolean
  readOnly: boolean
  onSave: () => void
  banner?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="mx-auto flex h-full max-w-[860px] flex-col px-10 pb-8 pt-2 max-sm:px-4">
      <header className="flex items-start justify-between gap-4 pb-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Personalization</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">{DESCRIPTION}</p>
        </div>
        <div className="flex h-8 shrink-0 items-center">
          {readOnly ? (
            <span className="text-[12px] text-ink-3">Read-only</span>
          ) : dirty || saving ? (
            <Button variant="primary" size="md" onClick={onSave} disabled={saving}>
              {saving ? 'Saving…' : 'Save changes'}
              <kbd className="font-mono text-[11px] text-on-primary/75">⌘S</kbd>
            </Button>
          ) : (
            <span className="text-[12px] text-ink-3">All changes saved</span>
          )}
        </div>
      </header>
      {banner}
      <div className="min-h-0 flex-1 overflow-hidden rounded-card border border-border bg-surface">
        {children}
      </div>
    </div>
  )
}

function EditorSkeleton() {
  return (
    <div className="mx-auto max-w-[860px] px-10">
      <Skeleton className="mb-4 h-7 w-40" />
      <Skeleton className="h-72" />
    </div>
  )
}
