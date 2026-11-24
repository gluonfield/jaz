import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ExternalLink, FileText, LoaderCircle, RefreshCw, Trash2, Upload } from 'lucide-react'
import { type DragEvent, useRef, useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { Button } from '@/components/ui/Button'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  cloudInstructionQuery,
  extractInstructionConflictCurrent,
  getInstruction,
  saveInstruction,
  type CloudInstructionDocument,
} from '@/lib/api/cloudInstructions'
import {
  deleteSpaceSource,
  spaceSourceContentUrl,
  spaceSourcesQuery,
  uploadSpaceSource,
  type SpaceSource,
} from '@/lib/api/cloudSpaces'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 1024) return `${Math.max(0, Math.round(bytes))} B`
  const units = ['KB', 'MB', 'GB']
  let value = bytes / 1024
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx += 1
  }
  return `${value >= 10 ? Math.round(value) : value.toFixed(1)} ${units[idx]}`
}

export function ProjectSourcesPanel({ spaceId, canEdit }: { spaceId: string; canEdit: boolean }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)
  const [uploadingCount, setUploadingCount] = useState(0)
  const sourcesQuery = useQuery(spaceSourcesQuery(spaceId))

  const remove = useMutation({
    mutationFn: (sourceId: string) => deleteSpaceSource(spaceId, sourceId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaceSources(spaceId) })
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpace(spaceId) })
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces })
      toast('Removed source')
    },
    onError: (err) => {
      toast(err instanceof Error ? err.message : 'Could not remove source', 'danger')
    },
  })

  const uploadFiles = async (fileList: FileList | File[]) => {
    const files = Array.from(fileList)
    if (!files.length) return
    setUploadingCount((n) => n + files.length)
    let uploaded = 0
    for (const file of files) {
      try {
        await uploadSpaceSource(spaceId, file)
        uploaded += 1
      } catch (err) {
        toast(`Failed to upload ${file.name}: ${err instanceof Error ? err.message : 'upload error'}`, 'danger')
      } finally {
        setUploadingCount((n) => Math.max(0, n - 1))
      }
    }
    if (uploaded > 0) {
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaceSources(spaceId) })
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpace(spaceId) })
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces })
      toast(uploaded === 1 ? 'Added 1 source' : `Added ${uploaded} sources`)
    }
  }

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setDragging(false)
    if (!canEdit || !e.dataTransfer.files?.length) return
    void uploadFiles(e.dataTransfer.files)
  }

  const sources = sourcesQuery.data ?? []

  return (
    <div className="flex flex-col gap-4">
      {canEdit ? (
        <div
          onDragOver={(e) => {
            e.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className={`flex flex-col items-center justify-center gap-2 rounded-card border border-dashed px-4 py-6 text-center transition-colors ${
            dragging ? 'border-primary bg-primary/5' : 'border-border bg-surface/60'
          }`}
        >
          <input
            ref={inputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => {
              if (e.target.files?.length) {
                void uploadFiles(e.target.files)
                e.target.value = ''
              }
            }}
          />
          <Upload size={18} className="text-ink-3" />
          <p className="text-[13px] text-ink-2">
            Drop files here, or{' '}
            <button
              type="button"
              onClick={() => inputRef.current?.click()}
              className="cursor-pointer font-medium text-primary hover:underline"
            >
              choose files
            </button>
          </p>
          <p className="text-[12px] text-ink-3">
            Text, Markdown, code, JSON, and CSV files are automatically included in every chat in this project.
          </p>
          {uploadingCount > 0 ? (
            <span className="inline-flex items-center gap-1.5 text-[12px] text-ink-2">
              <LoaderCircle size={13} className="animate-spin" />
              Uploading {uploadingCount} {uploadingCount === 1 ? 'file' : 'files'}…
            </span>
          ) : null}
        </div>
      ) : null}

      {sourcesQuery.isPending ? (
        <SkeletonRows count={3} />
      ) : sourcesQuery.isError ? (
        <p role="alert" className="text-[13px] text-danger">
          {(sourcesQuery.error as Error).message || 'Could not load project sources.'}
        </p>
      ) : sources.length === 0 ? (
        <p className="py-4 text-center text-[13px] text-ink-3">
          No sources yet. Add reference docs, specs, or notes for this project.
        </p>
      ) : (
        <ul className="divide-y divide-border/70 rounded-card border border-border bg-surface">
          {sources.map((src: SpaceSource) => (
            <li key={src.id} className="flex items-center gap-3 px-3.5 py-2.5">
              <span className="grid size-8 shrink-0 place-items-center rounded-control bg-bg text-ink-3">
                <FileText size={15} />
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium text-ink">{src.filename}</p>
                <p className="text-[11.5px] text-ink-3 tabular-nums">
                  {formatBytes(src.size_bytes)} · {src.content_type}
                </p>
              </div>
              <a
                href={spaceSourceContentUrl(spaceId, src.id)}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={`Open ${src.filename}`}
                title="Open file"
                className="grid size-7 shrink-0 place-items-center rounded-md text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink"
              >
                <ExternalLink size={14} />
              </a>
              {canEdit ? (
                <button
                  type="button"
                  aria-label={`Remove ${src.filename}`}
                  title="Remove source"
                  disabled={remove.isPending && remove.variables === src.id}
                  onClick={() => remove.mutate(src.id)}
                  className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors hover:bg-surface-2 hover:text-danger disabled:opacity-50"
                >
                  <Trash2 size={14} />
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function ProjectInstructionsPanel({ spaceId, canEdit }: { spaceId: string; canEdit: boolean }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState<string>()
  const [hasConflict, setHasConflict] = useState(false)
  const [editorResetSeq, setEditorResetSeq] = useState(0)

  const docQuery = useQuery(cloudInstructionQuery('space', spaceId, 'agents'))
  const doc = docQuery.data
  const locked = !canEdit || doc?.read_only === true

  const save = useMutation({
    mutationFn: ({ body_md, expected_version }: { body_md: string; expected_version: string }) =>
      saveInstruction('space', spaceId, 'agents', { body_md, expected_version }),
    onSuccess: (saved: CloudInstructionDocument, vars) => {
      queryClient.setQueryData(keys.cloudInstruction('space', spaceId, 'agents'), saved)
      setDraft((prev) => (prev === vars.body_md ? undefined : prev))
      setHasConflict(false)
      toast('Saved project instructions')
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.status === 409) {
        const current = extractInstructionConflictCurrent(error, 'space', spaceId, 'agents')
        if (current) {
          queryClient.setQueryData(keys.cloudInstruction('space', spaceId, 'agents'), current)
        }
        setHasConflict(true)
        return
      }
      toast(`Couldn't save instructions: ${error.message}`, 'danger')
    },
  })

  if (docQuery.isPending) return <SkeletonRows count={3} />
  if (docQuery.isError) {
    return (
      <p role="alert" className="text-[13px] text-danger">
        {(docQuery.error as Error).message || 'Could not load instructions.'}
      </p>
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
    await queryClient.invalidateQueries({ queryKey: keys.cloudInstruction('space', spaceId, 'agents') })
    setEditorResetSeq((n) => n + 1)
  }

  const overwriteWithMine = async () => {
    if (draft === undefined || save.isPending) return
    try {
      const latest = await getInstruction('space', spaceId, 'agents')
      queryClient.setQueryData(keys.cloudInstruction('space', spaceId, 'agents'), latest)
      save.mutate({ body_md: draft, expected_version: latest.version })
    } catch (error) {
      toast((error as Error).message || 'Could not reload version', 'danger')
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[12.5px] text-ink-3">
          Custom instructions applied to every chat in this project.
        </p>
        {!locked ? (
          <Button variant="primary" size="sm" disabled={!dirty || save.isPending} onClick={handleSave}>
            {save.isPending ? 'Saving…' : 'Save instructions'}
          </Button>
        ) : null}
      </div>

      {hasConflict ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-2 rounded-card border border-border bg-accent-soft px-3.5 py-2 text-[12.5px] text-accent-strong"
        >
          <span className="flex items-center gap-2">
            <AlertTriangle size={14} className="shrink-0" />
            Instructions were updated elsewhere.
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

      <div className="min-h-[260px] rounded-card border border-border bg-surface p-3">
        <MarkdownEditor
          key={`space:${spaceId}:${editorResetSeq}`}
          initialValue={value}
          readOnly={locked}
          placeholder="Add standing rules, tone, formatting requirements, or domain context for chats in this project…"
          onChange={(nextDoc) => {
            if (locked) return
            setDraft(nextDoc === savedBody ? undefined : nextDoc)
          }}
          onSave={handleSave}
        />
      </div>
    </div>
  )
}
