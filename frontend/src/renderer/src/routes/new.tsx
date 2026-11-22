import { createFileRoute, useLocation, useNavigate } from '@tanstack/react-router'
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { Cloud, Laptop, Lock, Users } from 'lucide-react'
import { NewSessionHome } from '@/components/home/NewSessionHome'
import { NewThreadOptions } from '@/components/home/NewThreadOptions'
import { ThreadModeSwitch } from '@/components/home/ThreadModeSwitch'
import { CloudSpendBanner } from '@/components/session/CloudSpendBanner'
import { deleteAttachmentDraft, saveAttachmentDraft } from '@/components/session/composerAttachmentDraftStore'
import { fileAttachment } from '@/components/session/composerAttachmentTypes'
import { useComposerDraft } from '@/components/session/useComposerDraft'
import { ProjectPicker } from '@/components/session/NewThreadControls'
import { AgentModelControls, useNewThreadControls } from '@/components/session/useNewThreadControls'
import { FastModeOption } from '@/components/session/NativeModelOptions'
import { TaskWorkspaceControl } from '@/components/session/TaskWorkspaceControl'
import { useToast } from '@/components/ui/toast'
import { ApiError } from '@/lib/api/client'
import { spacesQuery } from '@/lib/api/cloudSpaces'
import { machinesQuery } from '@/lib/api/cloudThreads'
import { isSpendCapError } from '@/lib/api/cloudUsage'
import { voiceAvailableQuery } from '@/lib/api/liveVoice'
import { createSession, projectsQuery } from '@/lib/api/sessions'
import { agentLabel } from '@/lib/agentLabel'
import { acpAgentSupportsGoal } from '@/lib/agentRuntimes'
import { useIsCloud } from '@/lib/connection'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { useTaskWorkspace } from '@/lib/hooks/useTaskWorkspace'
import { modelSuggestionLabel } from '@/lib/models'
import { NEW_SESSION_DIRECTORY_KEY, NEW_SESSION_DRAFT_KEY, type NewThreadPrompt } from '@/lib/newSessionConfig'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { submitNewSession } from './-newSessionSubmission'

type NewSearch = {
  project?: string
  space?: string
}

declare module '@tanstack/history' {
  interface HistoryState {
    startVoice?: boolean
  }
}

function storedString(key: string): string {
  return localStorage.getItem(key) ?? ''
}

export const Route = createFileRoute('/new')({
  validateSearch: (search): NewSearch => ({
    ...(typeof search.project === 'string' ? { project: search.project } : {}),
    ...(typeof search.space === 'string' && search.space.trim() ? { space: search.space.trim() } : {}),
  }),
  component: NewSessionPage,
})

// Welcome mode (agent-council pattern): heading + composer centered as one
// group in the middle of the page; the conversation view takes over once the
// first message is on its way.
function NewSessionPage() {
  const voice = useGlobalVoice()
  const isCloud = useIsCloud()
  const voiceAvailable = useQuery({ ...voiceAvailableQuery, enabled: !isCloud }).data
  const navigate = useNavigate()
  const search = Route.useSearch()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const [spendCapError, setSpendCapError] = useState<string | null>(null)
  const [spaceId, setSpaceId] = useState(() => search.space ?? '')
  const [machineId, setMachineId] = useState('')
  const spaces = useQuery({ ...spacesQuery, enabled: isCloud })
  const machines = useQuery({ ...machinesQuery, enabled: isCloud })
  const onlineMachines = (machines.data ?? []).filter((machine) => machine.online)
  const controls = useNewThreadControls()
  const incomingPrompt = useLocation({ select: (location) => location.state.newThreadPrompt })
  const { setDraft } = useComposerDraft({ storageKey: NEW_SESSION_DRAFT_KEY })
  const { runtimeAvailable, runtime } = controls
  const [directory, setDirectory] = useState(
    () => search.project ?? storedString(NEW_SESSION_DIRECTORY_KEY),
  )
  // Canvas threads show a board instead of a transcript; fixed at creation.
  const [canvas, setCanvas] = useState(false)
  const projects = useQuery({ ...projectsQuery, enabled: !isCloud })
  const project = projects.data?.find((item) => item.path === directory)
  const workspace = useTaskWorkspace(isCloud ? '' : directory)
  const { source: directoryInfo, ready: workspaceReady, worktree } = workspace
  const directoryIsGit = directoryInfo.data?.git === true

  useEffect(() => {
    if (search.project === undefined) return
    setDirectory(search.project)
  }, [search.project])

  useEffect(() => {
    if (search.space !== undefined) setSpaceId(search.space)
  }, [search.space])

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
    setSpendCapError(null)
    try {
      const baseInput = controls.sessionConfig(
        { directory: isCloud ? '' : directory, worktree: isCloud ? false : worktree, canvas },
        text?.trim() || undefined,
      )
      const input = isCloud
        ? {
            ...baseInput,
            ...(spaceId ? { space_id: spaceId, visibility: 'space' as const } : { visibility: 'private' as const }),
            ...(machineId ? { machine_id: machineId } : {}),
          }
        : baseInput
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
      if (isSpendCapError(error)) {
        setSpendCapError((error as Error).message || 'Monthly spend cap reached.')
      }
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  const handledPrompt = useRef<NewThreadPrompt>(undefined)
  const consumePrompt = useEffectEvent((prompt: NewThreadPrompt) => {
    if (handledPrompt.current === prompt) return
    handledPrompt.current = prompt
    void navigate({ replace: true, state: (previous) => ({ ...previous, newThreadPrompt: undefined }) })
    if (prompt.mode === 'send') {
      void handleStart(prompt.text, { files: prompt.files }, true)
      return
    }
    setDraft({ text: prompt.text, tokens: new Map() })
    if (prompt.files?.length) void saveAttachmentDraft(NEW_SESSION_DRAFT_KEY, 'session', prompt.files.map(fileAttachment)).catch(() => {})
  })
  useEffect(() => {
    if (!incomingPrompt) return
    if (incomingPrompt.mode === 'send' && (!controls.ready || !workspaceReady || (search.project !== undefined && search.project !== directory))) return
    consumePrompt(incomingPrompt)
  }, [incomingPrompt, controls.ready, workspaceReady, search.project, directory])

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
      {isCloud ? (
        <label className="inline-flex h-7 items-center gap-1.5 rounded-full bg-surface px-2.5 text-[12px] text-ink-2">
          {spaceId ? <Users size={12} className="text-primary" /> : <Lock size={12} className="text-ink-3" />}
          <select
            aria-label="Thread visibility"
            value={spaceId}
            disabled={creating}
            onChange={(event) => setSpaceId(event.target.value)}
            className="cursor-pointer bg-transparent font-medium text-ink outline-none"
          >
            <option value="">Private</option>
            {(spaces.data ?? []).map((space) => (
              <option key={space.id} value={space.id}>
                {space.name}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      {isCloud && onlineMachines.length > 0 ? (
        <label className="inline-flex h-7 items-center gap-1.5 rounded-full bg-surface px-2.5 text-[12px] text-ink-2">
          {machineId ? <Laptop size={12} className="text-primary" /> : <Cloud size={12} className="text-ink-3" />}
          <select
            aria-label="Execution target"
            value={machineId}
            disabled={creating}
            onChange={(event) => setMachineId(event.target.value)}
            className="cursor-pointer bg-transparent font-medium text-ink outline-none"
          >
            <option value="">Cloud Sandbox</option>
            {onlineMachines.map((machine) => (
              <option key={machine.device_id} value={machine.device_id}>
                {machine.display_name || machine.device_id}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      {controls.showProjectPicker ? (
        <ProjectPicker
          value={directory}
          placement={controlPlacement}
          disabled={creating}
          onChange={setDirectory}
        />
      ) : null}
      {directoryIsGit || directoryInfo.isError ? (
        <TaskWorkspaceControl worktree={workspace.choice} revision={directoryInfo.data?.revision} directory={directory} disabled={creating} onChange={workspace.setChoice} />
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
        banner={isCloud ? <CloudSpendBanner capError={spendCapError} onClearError={() => setSpendCapError(null)} /> : null}
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
