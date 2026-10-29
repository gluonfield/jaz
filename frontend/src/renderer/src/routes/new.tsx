import { createFileRoute, useLocation, useNavigate } from '@tanstack/react-router'
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { NewSessionHome } from '@/components/home/NewSessionHome'
import { NewThreadOptions } from '@/components/home/NewThreadOptions'
import { ThreadModeSwitch } from '@/components/home/ThreadModeSwitch'
import { deleteAttachmentDraft } from '@/components/session/composerAttachmentDraftStore'
import { ProjectPicker } from '@/components/session/NewThreadControls'
import { AgentModelControls, useNewThreadControls } from '@/components/session/useNewThreadControls'
import { FastModeOption } from '@/components/session/NativeModelOptions'
import { TaskWorkspaceControl } from '@/components/session/TaskWorkspaceControl'
import { useToast } from '@/components/ui/toast'
import { ApiError } from '@/lib/api/client'
import { voiceAvailableQuery } from '@/lib/api/liveVoice'
import { createSession, listFilesystemDirs, projectsQuery } from '@/lib/api/sessions'
import { agentLabel } from '@/lib/agentLabel'
import { acpAgentSupportsGoal } from '@/lib/agentRuntimes'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { modelSuggestionLabel } from '@/lib/models'
import { NEW_SESSION_DIRECTORY_KEY, NEW_SESSION_DRAFT_KEY } from '@/lib/newSessionConfig'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { submitNewSession } from './-newSessionSubmission'

type NewSearch = {
  project?: string
}

declare module '@tanstack/history' {
  interface HistoryState {
    startVoice?: boolean
    appPrompt?: string
  }
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
  const voice = useGlobalVoice()
  const voiceAvailable = useQuery(voiceAvailableQuery).data
  const navigate = useNavigate()
  const search = Route.useSearch()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const controls = useNewThreadControls()
  const { runtimeAvailable, runtime } = controls
  const [directory, setDirectory] = useState(
    () => search.project ?? storedString(NEW_SESSION_DIRECTORY_KEY),
  )
  const [useWorktree, setUseWorktree] = useState(true)
  // Canvas threads show a board instead of a transcript; fixed at creation.
  const [canvas, setCanvas] = useState(false)
  const projects = useQuery(projectsQuery)
  const project = projects.data?.find((item) => item.path === directory)
  const directoryInfo = useQuery({
    queryKey: keys.filesystemDirs(directory),
    queryFn: () => listFilesystemDirs(directory),
    enabled: directory !== '',
    staleTime: 30_000,
    retry: false,
  })
  const directoryIsGit = directoryInfo.data?.git ?? project?.git ?? false
  const workspaceReady = !directory || !useWorktree || directoryInfo.isSuccess
  const worktree = directoryIsGit && useWorktree

  useEffect(() => {
    if (search.project === undefined) return
    setDirectory(search.project)
  }, [search.project])

  useEffect(() => {
    localStorage.setItem(NEW_SESSION_DIRECTORY_KEY, directory)
  }, [directory])

  // A directory from another backend won't exist here; drop it on a 4xx reject.
  useEffect(() => {
    const error = directoryInfo.error
    if (directory && project === undefined && error instanceof ApiError && error.status >= 400 && error.status < 500) {
      setDirectory('')
    }
  }, [directory, project, directoryInfo.error])

  const handleStart = async (text?: string, options: SendMessageOptions = {}, fromApp = false) => {
    if (text === undefined && voice.sessionId && voice.phase !== 'off') {
      await navigate({ to: '/sessions/$sessionId', params: { sessionId: voice.sessionId } })
      return
    }
    if (!runtimeAvailable) {
      toast('Connect an agent in Settings before starting a session.', 'danger')
      return
    }
    if (!workspaceReady) {
      toast('Check the project workspace before starting a chat.', 'danger')
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
      const input = controls.sessionConfig({ directory, worktree, canvas }, text?.trim() || undefined)
      if (text === undefined) {
        const session = await createSession(input)
        invalidateSessionLists(queryClient, { session: session.id })
        await navigate({
          to: '/sessions/$sessionId',
          params: { sessionId: session.id },
          state: { startVoice: true },
        })
        return
      }
      await submitNewSession(
        input,
        text,
        options,
        (initialSessionPrompt) => {
          const sessionId = initialSessionPrompt.sessionId
          if (!fromApp) {
            sessionStorage.removeItem(NEW_SESSION_DRAFT_KEY)
            void deleteAttachmentDraft(NEW_SESSION_DRAFT_KEY, 'session')
          }
          invalidateSessionLists(queryClient, { session: sessionId })
          void navigate({
            to: '/sessions/$sessionId',
            params: { sessionId },
            replace: fromApp,
            state: { initialSessionPrompt },
          })
        },
      )
    } catch (error) {
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  // An app's jaz/threads/create opens this page with a prompt. It starts once
  // the agent and model are ready, leaves the composer's draft alone and
  // replaces this page in history, so Back returns to the app.
  const appPrompt = useLocation({ select: (location) => location.state.appPrompt })
  const startedPrompt = useRef<string>(undefined)
  const startAppPrompt = useEffectEvent((prompt: string) => {
    if (startedPrompt.current === prompt) {
      return
    }
    startedPrompt.current = prompt
    void navigate({ replace: true, state: (previous) => ({ ...previous, appPrompt: undefined }) })
    void handleStart(prompt, {}, true)
  })
  useEffect(() => {
    if (appPrompt && controls.ready && workspaceReady && (search.project === undefined || search.project === directory)) {
      startAppPrompt(appPrompt)
    }
  }, [appPrompt, controls.ready, workspaceReady, search.project, directory])

  // Phone: the controls live in a header dropdown, which sits above the
  // composer, so their own popovers open downward.
  const isMobile = useIsMobile()
  const controlPlacement = isMobile ? 'below' : 'above'
  const modelLabel =
    controls.model === '' ? agentLabel(runtime) : modelSuggestionLabel(controls.modelSuggestions, controls.model)
  const directoryLabel = directory
    ? (project?.name ?? directory.split(/[\\/]+/).filter(Boolean).at(-1) ?? directory)
    : ''
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

  const agentControls = <AgentModelControls controls={controls} placement={controlPlacement} disabled={creating} />
  // The thread's setup sits on a shelf above the composer: where it runs, and
  // whether it is a chat or a canvas.
  const setupControls = (
    <>
      {controls.showProjectPicker ? (
        <ProjectPicker
          value={directory}
          placement={controlPlacement}
          disabled={creating}
          onChange={setDirectory}
        />
      ) : null}
      {directoryIsGit ? (
        <TaskWorkspaceControl worktree={worktree} revision={directoryInfo.data?.revision} directory={directory} disabled={creating} onChange={setUseWorktree} />
      ) : null}
      {directoryInfo.isError ? (
        <button type="button" onClick={() => void directoryInfo.refetch()} className="px-2 text-[12px] text-danger">Retry project check</button>
      ) : null}
      <span className="flex-1" />
      <ThreadModeSwitch canvas={canvas} onChange={setCanvas} disabled={creating} />
    </>
  )

  return (
    <>
      {isMobile ? (
        <NewThreadOptions title={optionsTitle} subtitle={optionsSubtitle || undefined}>
          {agentControls}
          {setupControls}
        </NewThreadOptions>
      ) : null}
      <NewSessionHome
        creating={creating}
        disabled={!runtimeAvailable || !workspaceReady}
        goalAvailable={acpAgentSupportsGoal(runtime)}
        leftSlot={isMobile ? null : agentControls}
        shelf={isMobile ? null : setupControls}
        canvas={canvas}
        optionsSlot={controls.fastMode ? <FastModeOption {...controls.fastMode} disabled={creating} /> : null}
        draftStorageKey={NEW_SESSION_DRAFT_KEY}
        // Tokens freeze their absolute expansion at insert time, so re-picking
        // the directory after tagging keeps old tags valid rather than rebasing
        // them.
        fileRoot={directory}
        onSend={handleStart}
        onVoice={voiceAvailable ? () => void handleStart() : undefined}
      />
    </>
  )
}
