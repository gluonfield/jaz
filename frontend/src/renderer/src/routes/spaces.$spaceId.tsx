import type { CloudGitRepoSpec } from '@/lib/api/cloudThreadRepos'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  AppWindow,
  FileText,
  Folder,
  FolderMinus,
  GitBranch,
  Lock,
  MessageSquare,
  Settings2,
  Sparkles,
  Trash2,
  UserCheck,
  Users,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { ThreadRepoPicker } from '@/components/cloud/GitHubRepoPicker'
import { CloudSpendBanner } from '@/components/session/CloudSpendBanner'
import { ComposerCard } from '@/components/session/Composer'
import { FastModeOption } from '@/components/session/NativeModelOptions'
import { AgentModelControls, useNewThreadControls } from '@/components/session/useNewThreadControls'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { sessionLabel, stateDot } from '@/components/sidebar/SessionRow'
import { SpaceModal } from '@/components/sidebar/SpaceModal'
import { ProjectInstructionsPanel, ProjectSourcesPanel } from '@/components/spaces/ProjectSourcesPanel'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { FileDropScope } from '@/components/ui/FileDrop'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { appsQuery } from '@/lib/api/apps'
import { spaceDefaultGitRepos, spaceQuery, spaceSourcesQuery, spacesQuery } from '@/lib/api/cloudSpaces'
import { isSpendCapError } from '@/lib/api/cloudUsage'
import { deleteSession, moveSessionToSpace, sidebarSessionsQuery } from '@/lib/api/sessions'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { submitNewSession } from './-newSessionSubmission'

export const Route = createFileRoute('/spaces/$spaceId')({
  component: SpaceProjectPage,
})

type ProjectTab = 'chats' | 'apps' | 'sources' | 'instructions'

function formatRelativeDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const now = new Date()
  if (date.toDateString() === now.toDateString()) {
    return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  }
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function SpaceProjectPage() {
  const { spaceId } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isMobile = useIsMobile()
  const isViewer = useIsCloudViewer()

  const [tab, setTab] = useState<ProjectTab>('chats')
  const [creating, setCreating] = useState(false)
  const [spendCapError, setSpendCapError] = useState<string | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [repoChoice, setRepoChoice] = useState<CloudGitRepoSpec[]>()

  const detail = useQuery(spaceQuery(spaceId))
  const allSpaces = useQuery(spacesQuery)
  const space = detail.data ?? allSpaces.data?.find((item) => item.id === spaceId)
  const sessions = useQuery(sidebarSessionsQuery)
  const sources = useQuery(spaceSourcesQuery(spaceId))
  const spaceApps = useQuery(appsQuery({ scope: 'space', spaceId }))
  const controls = useNewThreadControls('cloud')

  useEffect(() => {
    setRepoChoice(undefined)
  }, [spaceId])

  const defaultRepos = spaceDefaultGitRepos(space)

  const projectChats = useMemo(
    () => (sessions.data ?? []).filter((s) => s.space_id === spaceId && !s.archived),
    [sessions.data, spaceId],
  )

  const removeFromProject = useMutation({
    mutationFn: (sessionId: string) => moveSessionToSpace(sessionId, ''),
    onSuccess: (_updated, sessionId) => {
      invalidateSessionLists(queryClient, { session: sessionId })
      toast('Removed chat from project')
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not update chat', 'danger'),
  })

  const deleteChat = useMutation({
    mutationFn: (sessionId: string) => deleteSession(sessionId),
    onSuccess: () => {
      invalidateSessionLists(queryClient)
      toast('Deleted chat')
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not delete chat', 'danger'),
  })

  const handleStartChat = async (text: string, options: SendMessageOptions = {}) => {
    if (isViewer) {
      toast('Viewers have read-only access in this workspace.', 'danger')
      return
    }
    if (!controls.runtimeAvailable) {
      toast('No cloud models are configured for your workspace.', 'danger')
      return
    }
    if (controls.reasoningBlocked) {
      toast('Model capabilities are still loading. Try again in a moment.', 'danger')
      return
    }
    setCreating(true)
    setSpendCapError(null)
    try {
      const baseInput = controls.sessionConfig(
        { directory: '', worktree: false, canvas: false },
        text.trim() || undefined,
      )
      await submitNewSession(
        {
          ...baseInput,
          space_id: spaceId,
          git_repos: repoChoice,
        },
        text,
        options,
        (initialSessionPrompt) => {
          invalidateSessionLists(queryClient, { session: initialSessionPrompt.sessionId })
          void navigate({
            to: '/sessions/$sessionId',
            params: { sessionId: initialSessionPrompt.sessionId },
            state: { initialSessionPrompt },
          })
        },
      )
    } catch (error) {
      if (isSpendCapError(error)) {
        setSpendCapError((error as Error).message || 'Monthly spend cap reached.')
      }
      toast(`Couldn't start chat: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  if (detail.isPending && !space) {
    return (
      <div className="mx-auto w-full max-w-[780px] px-6 py-10">
        <SkeletonRows count={4} />
      </div>
    )
  }

  if (detail.isError && !space) {
    return (
      <EmptyState title="Project not found">
        <p className="text-[13px] text-ink-3">This project may have been deleted or you no longer have access.</p>
        <Button className="mt-3" variant="primary" onClick={() => void navigate({ to: '/new' })}>
          Back to New chat
        </Button>
      </EmptyState>
    )
  }

  if (!space) return null

  const canEdit = !isViewer && space.can_edit !== false
  const visibilityMeta =
    space.visibility === 'private'
      ? { label: 'Private', Icon: Lock }
      : space.visibility === 'team'
        ? { label: 'Team', Icon: Users }
        : { label: 'Selected people', Icon: UserCheck }
  const VisibilityIcon = visibilityMeta.Icon
  const sourceCount = sources.data?.length ?? space.source_count ?? 0
  const controlPlacement = isMobile ? 'below' : 'above'

  return (
    <FileDropScope className="flex h-full flex-col overflow-y-auto">
      <div className="mx-auto flex w-full max-w-[780px] flex-1 flex-col gap-6 px-6 py-8 max-sm:px-4">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid size-10 shrink-0 place-items-center rounded-card bg-surface text-primary ring-1 ring-border/70">
              <Folder size={20} />
            </span>
            <div className="min-w-0">
              <h1 className="truncate text-[20px] font-semibold tracking-tight text-ink">{space.name}</h1>
              <div className="mt-0.5 flex items-center gap-2 text-[12px] text-ink-3">
                <span className="inline-flex items-center gap-1">
                  <VisibilityIcon size={12} />
                  {visibilityMeta.label}
                </span>
                <span>·</span>
                <span>{projectChats.length} {projectChats.length === 1 ? 'chat' : 'chats'}</span>
                <span>·</span>
                <span>{sourceCount} {sourceCount === 1 ? 'source' : 'sources'}</span>
                {defaultRepos.length > 0 ? (
                  <>
                    <span>·</span>
                    <span className="inline-flex items-center gap-1 text-[11.5px] text-ink-2" title={defaultRepos.map(({ repo }) => repo).join(', ')}>
                      <GitBranch size={11} />
                      {defaultRepos.length} {defaultRepos.length === 1 ? 'repository' : 'repositories'}
                    </span>
                  </>
                ) : null}
              </div>
            </div>
          </div>

          <Button variant="secondary" size="sm" onClick={() => setSettingsOpen(true)}>
            <Settings2 size={13} />
            {canEdit ? 'Settings & sharing' : 'Access'}
          </Button>
        </header>

        <div>
          <CloudSpendBanner capError={spendCapError} onClearError={() => setSpendCapError(null)} />
          {isViewer ? (
            <div className="rounded-card border border-border bg-surface px-4 py-3 text-center text-[13px] text-ink-3">
              View-only access — viewers cannot start new chats in this project.
            </div>
          ) : (
            <div>
              <div className="mb-1.5 flex items-center gap-1">
                <ThreadRepoPicker
                  value={repoChoice ?? defaultRepos}
                  placement={controlPlacement}
                  disabled={creating}
                  includePersonal={space.visibility === 'private'}
                  onChange={setRepoChoice}
                />
              </div>
              <ComposerCard
                streaming={creating}
                placeholder={`New chat in ${space.name}…`}
                planAvailable
                goalControlVisible={false}
                goalAvailable={false}
                disabled={creating || !controls.runtimeAvailable}
                leftSlot={<AgentModelControls controls={controls} placement={controlPlacement} disabled={creating} />}
                optionsSlot={controls.fastMode ? <FastModeOption {...controls.fastMode} disabled={creating} /> : null}
                draftStorageKey={`jaz.project.draft.${spaceId}`}
                clearTiming="never"
                onSend={handleStartChat}
              />
            </div>
          )}
        </div>

        <div className="flex flex-col gap-4">
          <div role="tablist" aria-label="Project sections" className="flex items-center gap-1 border-b border-border/70">
            {(
              [
                { id: 'chats', label: 'Chats', count: projectChats.length, Icon: MessageSquare },
                { id: 'apps', label: 'Apps', count: (spaceApps.data ?? []).length, Icon: AppWindow },
                { id: 'sources', label: 'Sources', count: sourceCount, Icon: FileText },
                { id: 'instructions', label: 'Instructions', Icon: Sparkles },
              ] as const
            ).map((item) => {
              const active = tab === item.id
              const TabIcon = item.Icon
              return (
                <button
                  key={item.id}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  onClick={() => setTab(item.id)}
                  className={`relative -mb-px flex cursor-pointer items-center gap-1.5 border-b-2 px-3 py-2 text-[13px] font-medium transition-colors ${
                    active
                      ? 'border-primary text-ink'
                      : 'border-transparent text-ink-3 hover:text-ink'
                  }`}
                >
                  <TabIcon size={13} />
                  <span>{item.label}</span>
                  {'count' in item ? (
                    <span className="rounded-full bg-surface-2 px-1.5 py-0.5 text-[11px] text-ink-3 tabular-nums">
                      {item.count}
                    </span>
                  ) : null}
                </button>
              )
            })}
          </div>

          {tab === 'chats' ? (
            projectChats.length === 0 ? (
              <p className="py-6 text-center text-[13px] text-ink-3">
                No chats in this project yet. Start one above or move an existing chat from the sidebar.
              </p>
            ) : (
              <ul className="divide-y divide-border/70 rounded-card border border-border bg-surface">
                {projectChats.map((chat) => {
                  const dot = stateDot(chat)
                  return (
                    <li key={chat.id} className="group flex items-center gap-2 px-3 py-2 transition-colors hover:bg-surface-2/50">
                      <Link
                        to="/sessions/$sessionId"
                        params={{ sessionId: chat.id }}
                        className="flex min-w-0 flex-1 items-center gap-2.5 py-0.5"
                      >
                        <RuntimeBadge session={chat} compact />
                        <span className="min-w-0 flex-1 truncate text-[13.5px] font-medium text-ink">
                          {sessionLabel(chat)}
                        </span>
                        {dot ? (
                          <span title={dot.title} className={`size-1.5 shrink-0 rounded-full ${dot.className}`} />
                        ) : null}
                        <span className="shrink-0 text-[12px] text-ink-3 tabular-nums">
                          {formatRelativeDate(chat.updated_at)}
                        </span>
                      </Link>
                      {!isViewer ? (
                        <>
                          <button
                            type="button"
                            title="Remove from project"
                            aria-label={`Remove ${sessionLabel(chat)} from project`}
                            onClick={() => removeFromProject.mutate(chat.id)}
                            className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 opacity-0 transition-colors hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover:opacity-100"
                          >
                            <FolderMinus size={13} />
                          </button>
                          <button
                            type="button"
                            title="Delete chat"
                            aria-label={`Delete ${sessionLabel(chat)}`}
                            onClick={() => deleteChat.mutate(chat.id)}
                            className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 opacity-0 transition-colors hover:bg-surface-2 hover:text-danger focus-visible:opacity-100 group-hover:opacity-100"
                          >
                            <Trash2 size={13} />
                          </button>
                        </>
                      ) : null}
                    </li>
                  )
                })}
              </ul>
            )
          ) : tab === 'apps' ? (
            (spaceApps.data ?? []).length === 0 ? (
              <p className="py-6 text-center text-[13px] text-ink-3">
                No apps shared in this Space yet. Publish any artifact or document and choose this Space to share it with members.
              </p>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2">
                {(spaceApps.data ?? []).map((app) => (
                  <Link
                    key={app.id}
                    to="/apps/$appId"
                    params={{ appId: app.id }}
                    className="flex flex-col justify-between gap-2 rounded-card border border-border bg-surface p-3.5 transition-colors hover:border-primary/40"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate text-[13.5px] font-semibold text-ink">{app.title}</span>
                      <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-[10.5px] text-ink-3">
                        v{app.current_version} · {app.kind}
                      </span>
                    </div>
                    {app.description ? (
                      <p className="line-clamp-2 text-[12px] text-ink-2">{app.description}</p>
                    ) : null}
                  </Link>
                ))}
              </div>
            )
          ) : tab === 'sources' ? (
            <ProjectSourcesPanel spaceId={spaceId} canEdit={canEdit} />
          ) : (
            <ProjectInstructionsPanel spaceId={spaceId} canEdit={canEdit} />
          )}
        </div>
      </div>

      {settingsOpen ? (
        <SpaceModal
          space={space}
          onClose={() => setSettingsOpen(false)}
          onDeleted={() => void navigate({ to: '/new' })}
        />
      ) : null}
    </FileDropScope>
  )
}
