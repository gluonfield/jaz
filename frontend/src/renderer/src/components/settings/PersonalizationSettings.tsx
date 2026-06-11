import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { FileTabs } from './FileTabs'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { agentFilesQuery, saveAgentFile } from '@/lib/api/agentFiles'
import type { AgentFile, AgentFilesResponse } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

const FILE_DESCRIPTIONS: Record<string, string> = {
  'AGENTS.md': 'How your assistant works: roles, capabilities, and rules of engagement.',
  'SOUL.md': 'Who your assistant is: personality, voice, and values.',
}

export function PersonalizationSettings() {
  const files = useQuery(agentFilesQuery)
  const queryClient = useQueryClient()
  const toast = useToast()

  const [active, setActive] = useState<string>()
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  const save = useMutation({
    mutationFn: ({ name, content }: { name: string; content: string }) =>
      saveAgentFile(name, content),
    onSuccess: (saved: AgentFile) => {
      queryClient.setQueryData<AgentFilesResponse>(keys.agentFiles, (prev) =>
        prev
          ? {
              ...prev,
              files: prev.files.map((f) => (f.name === saved.name ? saved : f)),
            }
          : prev,
      )
      setDrafts((prev) => {
        const next = { ...prev }
        delete next[saved.name]
        return next
      })
      toast(`Saved ${saved.name}`)
    },
    onError: (error: Error, variables) => {
      toast(`Couldn't save ${variables.name}: ${error.message}`, 'danger')
    },
  })

  if (files.isPending) {
    return (
      <div className="mx-auto max-w-[860px] px-10">
        <Skeleton className="mb-4 h-7 w-40" />
        <Skeleton className="mb-4 h-9 w-96" />
        <Skeleton className="h-72" />
      </div>
    )
  }

  if (files.isError) {
    return (
      <EmptyState title="Couldn't load agent files">
        <p>{files.error.message}</p>
      </EmptyState>
    )
  }

  const fileList = files.data.files
  const activeName = active ?? fileList[0]?.name
  const activeFile = fileList.find((f) => f.name === activeName)

  if (!activeFile) {
    return <EmptyState title="The platform exposes no agent files" />
  }

  const draft = drafts[activeFile.name]
  const value = draft ?? activeFile.content
  const dirty = draft !== undefined && draft !== activeFile.content

  const handleSave = () => {
    if (save.isPending) return
    save.mutate({ name: activeFile.name, content: value })
  }

  return (
    <div className="mx-auto flex h-full max-w-[860px] flex-col px-10 pb-8 pt-2">
      <header className="flex items-start justify-between gap-4 pb-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Personalization</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">
            {FILE_DESCRIPTIONS[activeFile.name] ?? 'Agent definition file.'} Saved changes apply the
            next time the backend builds its prompt.
          </p>
        </div>
        <div className="flex h-8 shrink-0 items-center">
          {dirty || save.isPending ? (
            <Button variant="primary" size="md" onClick={handleSave} disabled={save.isPending}>
              {save.isPending ? 'Saving…' : 'Save changes'}
              <kbd className="font-mono text-[11px] text-on-primary/75">⌘S</kbd>
            </Button>
          ) : (
            <span className="text-[12px] text-ink-3">All changes saved</span>
          )}
        </div>
      </header>

      <FileTabs
        className="border-b border-border"
        underlineId="personalization-tab-underline"
        active={activeFile.name}
        onSelect={setActive}
        tabs={fileList.map((file) => {
          const fileDraft = drafts[file.name]
          return {
            name: file.name,
            dirty: fileDraft !== undefined && fileDraft !== file.content,
            badge: file.exists ? undefined : 'new',
          }
        })}
      />

      <div className="min-h-0 flex-1 overflow-hidden rounded-b-card border border-t-0 border-border">
        <MarkdownEditor
          key={activeFile.name}
          initialValue={value}
          placeholder={FILE_DESCRIPTIONS[activeFile.name] ?? 'Write markdown here.'}
          onChange={(doc) => setDrafts((prev) => ({ ...prev, [activeFile.name]: doc }))}
          onSave={handleSave}
        />
      </div>
    </div>
  )
}
