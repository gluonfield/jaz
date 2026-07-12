import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { NewSessionHome } from '@/components/home/NewSessionHome'
import { NewThreadOptions } from '@/components/home/NewThreadOptions'
import { deleteAttachmentDraft } from '@/components/session/composerAttachmentDraftStore'
import { ProjectPicker } from '@/components/session/NewThreadControls'
import { AgentModelControls, useNewThreadControls } from '@/components/session/useNewThreadControls'
import { Checkbox } from '@/components/ui/Checkbox'
import { useToast } from '@/components/ui/toast'
import { ApiError } from '@/lib/api/client'
import { createSession, listFilesystemDirs, projectsQuery } from '@/lib/api/sessions'
import { agentLabel } from '@/lib/agentLabel'
import { acpAgentSupportsGoal } from '@/lib/agentRuntimes'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { modelSuggestionLabel } from '@/lib/models'
import { NEW_SESSION_DIRECTORY_KEY, NEW_SESSION_DRAFT_KEY } from '@/lib/newSessionConfig'
import { setPendingMessage } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { useTheme } from '@/lib/theme'
import { useQuery, useQueryClient } from '@tanstack/react-query'

type NewSearch = {
  project?: string
}

function storedString(key: string): string {
  return localStorage.getItem(key) ?? ''
}

export const Route = createFileRoute('/new')({
  validateSearch: (search): NewSearch =>
    typeof search.project === 'string' ? { project: search.project } : {},
  component: NewSessionPage,
})

// Welcome mode (agent-council pattern): heading + composer centered as one
// group in the middle of the page; the conversation view takes over once the
// first message is on its way.
function NewSessionPage() {
  const navigate = useNavigate()
  const search = Route.useSearch()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const [composing, setComposing] = useState(false)
  const controls = useNewThreadControls()
  const { runtimeAvailable, runtime } = controls
  const [directory, setDirectory] = useState(
    () => search.project ?? storedString(NEW_SESSION_DIRECTORY_KEY),
  )
  // Worktree runs the session on a disposable git worktree (any agent);
  // only offered when the chosen directory is a git repository.
  const [worktree, setWorktree] = useState(false)
  const projects = useQuery(projectsQuery)
  const project = projects.data?.find((item) => item.path === directory)
  const directoryInfo = useQuery({
    queryKey: keys.filesystemDirs(directory),
    queryFn: () => listFilesystemDirs(directory),
    enabled: directory !== '' && project === undefined,
    staleTime: 30_000,
    retry: false,
  })
  const directoryIsGit = project?.git ?? directoryInfo.data?.git ?? false
  // PixelField samples the palette at mount; remount it when the theme flips.
  const { resolved } = useTheme()

  useEffect(() => {
    if (search.project === undefined) return
    setDirectory(search.project)
    setWorktree(false)
  }, [search.project])

  useEffect(() => {
    localStorage.setItem(NEW_SESSION_DIRECTORY_KEY, directory)
  }, [directory])

  useEffect(() => {
    if (!directoryIsGit) setWorktree(false)
  }, [directoryIsGit])

  // A directory from another backend won't exist here; drop it on a 4xx reject.
  useEffect(() => {
    const error = directoryInfo.error
    if (directory && project === undefined && error instanceof ApiError && error.status >= 400 && error.status < 500) {
      setDirectory('')
    }
  }, [directory, project, directoryInfo.error])

  const startThread = async (title: string | undefined, prepare: (sessionId: string) => void) => {
    if (!runtimeAvailable) {
      toast('Connect an agent in Settings before starting a session.', 'danger')
      return
    }
    if (controls.reasoningBlocked) {
      toast(controls.reasoningStatus === 'error'
        ? 'Model capabilities are unavailable. Try again in a moment.'
        : 'Model capabilities are still loading. Try again in a moment.', 'danger')
      return
    }
    setCreating(true)
    try {
      const session = await createSession(controls.sessionConfig({ directory, worktree }, title))
      prepare(session.id)
      sessionStorage.removeItem(NEW_SESSION_DRAFT_KEY)
      void deleteAttachmentDraft(NEW_SESSION_DRAFT_KEY, 'session')
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      navigate({ to: '/sessions/$sessionId', params: { sessionId: session.id } })
    } catch (error) {
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  const handleSend = (text: string, options: SendMessageOptions = {}) =>
    startThread(text.trim() || undefined, (id) =>
      setPendingMessage(id, {
        text,
        planRequested: Boolean(options.planRequested),
        goalRequested: Boolean(options.goalRequested),
        files: options.files ?? [],
      }),
    )

  // Phone: the controls live in a header dropdown, which sits above the
  // composer, so their own popovers open downward.
  const isMobile = useIsMobile()
  const controlPlacement = isMobile ? 'below' : 'above'
  const modelLabel =
    controls.model === '' ? agentLabel(runtime) : modelSuggestionLabel(controls.modelSuggestions, controls.model)
  const directoryLabel = directory
    ? (project?.name ?? directory.split(/[\\/]+/).filter(Boolean).at(-1) ?? directory)
    : ''
  // Mobile header: shown when there's a picker to surface or an agent to connect.
  const showMobileOptions =
    !runtimeAvailable ||
    controls.showAgentPicker ||
    controls.showModelPicker ||
    controls.showProjectPicker ||
    directoryIsGit
  const optionsTitle = controls.showModelPicker
    ? modelLabel
    : controls.showAgentPicker
      ? agentLabel(runtime)
      : 'Options'
  const optionsSubtitle = [
    controls.showModelPicker ? controls.effort : '',
    controls.showProjectPicker ? directoryLabel : '',
  ]
    .filter(Boolean)
    .join(' · ')

  const composerControls = (
    <>
      <AgentModelControls controls={controls} placement={controlPlacement} disabled={creating} />
      {controls.showProjectPicker ? (
        <ProjectPicker
          value={directory}
          placement={controlPlacement}
          disabled={creating}
          onChange={(path, git) => {
            setDirectory(path)
            if (!git) setWorktree(false)
          }}
        />
      ) : null}
      {directoryIsGit ? (
        <div className="flex items-center gap-1.5 text-[13px] text-ink-2">
          <Checkbox
            checked={worktree}
            onChange={setWorktree}
            disabled={creating}
            aria-label="Run on a git worktree"
          />
          <button
            type="button"
            tabIndex={-1}
            disabled={creating}
            onClick={() => setWorktree((v) => !v)}
            className="cursor-pointer select-none disabled:cursor-default disabled:opacity-50"
          >
            Worktree
          </button>
        </div>
      ) : null}
    </>
  )

  return (
    <>
      {isMobile && showMobileOptions ? (
        <NewThreadOptions title={optionsTitle} subtitle={optionsSubtitle || undefined}>
          {composerControls}
        </NewThreadOptions>
      ) : null}
      <NewSessionHome
        themeKey={resolved}
        calm={composing || creating}
        creating={creating}
        disabled={!runtimeAvailable}
        goalAvailable={acpAgentSupportsGoal(runtime)}
        leftSlot={isMobile ? null : composerControls}
        draftStorageKey={NEW_SESSION_DRAFT_KEY}
        // Tokens freeze their absolute expansion at insert time, so re-picking
        // the directory after tagging keeps old tags valid rather than rebasing
        // them.
        fileRoot={directory}
        onDraftActivity={setComposing}
        onSend={handleSend}
        onVoice={undefined}
      />
    </>
  )
}
