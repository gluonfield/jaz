import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  cloudInstructionQuery,
  extractInstructionConflictCurrent,
  getInstruction,
  saveInstruction,
  type CloudInstructionDocument,
  type InstructionKind,
  type InstructionScope,
} from '@/lib/api/cloudInstructions'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'

export interface CloudInstructionEditorFrameProps {
  dirty: boolean
  saving: boolean
  readOnly: boolean
  onSave: () => void
  banner: ReactNode
  children: ReactNode
}

export function CloudInstructionEditor({
  scope,
  scopeId,
  kind = 'agents',
  readOnly = false,
  placeholder,
  savedToast = 'Saved instructions',
  conflictLabel = 'Instructions were updated elsewhere.',
  loadingFallback,
  renderFrame,
}: {
  scope: InstructionScope
  scopeId: string
  kind?: InstructionKind
  readOnly?: boolean
  placeholder: string
  savedToast?: string
  conflictLabel?: string
  loadingFallback?: ReactNode
  renderFrame: (props: CloudInstructionEditorFrameProps) => ReactNode
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState<string>()
  const [hasConflict, setHasConflict] = useState(false)
  const [editorResetSeq, setEditorResetSeq] = useState(0)

  const docQuery = useQuery(cloudInstructionQuery(scope, scopeId, kind))
  const doc = docQuery.data
  const locked = readOnly || doc?.read_only === true

  const save = useMutation({
    mutationFn: ({ body_md, expected_version }: { body_md: string; expected_version: string }) =>
      saveInstruction(scope, scopeId, kind, { body_md, expected_version }),
    onSuccess: (saved: CloudInstructionDocument, vars) => {
      queryClient.setQueryData(keys.cloudInstruction(scope, scopeId, kind), saved)
      setDraft((prev) => (prev === vars.body_md ? undefined : prev))
      setHasConflict(false)
      toast(savedToast)
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.status === 409) {
        const current = extractInstructionConflictCurrent(error, scope, scopeId, kind)
        if (current) {
          queryClient.setQueryData(keys.cloudInstruction(scope, scopeId, kind), current)
        }
        setHasConflict(true)
        return
      }
      toast(`Couldn't save instructions: ${error.message}`, 'danger')
    },
  })

  if (docQuery.isPending) {
    return loadingFallback ?? <SkeletonRows count={3} />
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
    await queryClient.invalidateQueries({ queryKey: keys.cloudInstruction(scope, scopeId, kind) })
    setEditorResetSeq((n) => n + 1)
  }

  const overwriteWithMine = async () => {
    if (draft === undefined || save.isPending) return
    try {
      const latest = await getInstruction(scope, scopeId, kind)
      queryClient.setQueryData(keys.cloudInstruction(scope, scopeId, kind), latest)
      save.mutate({ body_md: draft, expected_version: latest.version })
    } catch (error) {
      toast((error as Error).message || 'Could not reload version', 'danger')
    }
  }

  const banner = hasConflict ? (
    <div
      role="alert"
      className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-card border border-border bg-accent-soft px-3.5 py-2 text-[12.5px] text-accent-strong"
    >
      <span className="flex items-center gap-2">
        <AlertTriangle size={14} className="shrink-0" />
        {conflictLabel}
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

  return renderFrame({
    dirty,
    saving: save.isPending,
    readOnly: locked,
    onSave: handleSave,
    banner,
    children: (
      <MarkdownEditor
        key={`cloud:${scope}:${scopeId}:${kind}:${editorResetSeq}`}
        initialValue={value}
        readOnly={locked}
        placeholder={placeholder}
        onChange={(nextDoc) => {
          if (locked) return
          setDraft(nextDoc === savedBody ? undefined : nextDoc)
        }}
        onSave={handleSave}
      />
    ),
  })
}
