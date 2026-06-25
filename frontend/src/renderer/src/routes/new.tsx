import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'
import { NewSessionHome } from '@/components/home/NewSessionHome'
import { NewThreadOptions } from '@/components/home/NewThreadOptions'
import { ModelSelect, ProjectPicker, RuntimeSelect } from '@/components/session/NewThreadControls'
import { Checkbox } from '@/components/ui/Checkbox'
import { useToast } from '@/components/ui/toast'
import { ApiError } from '@/lib/api/client'
import { createSession, listFilesystemDirs, projectsQuery } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { agentLabel } from '@/lib/agentLabel'
import {
  acpAgentSupportsNativeGoal,
  enabledACPAgents,
  runtimeModelState,
} from '@/lib/agentRuntimes'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import {
  acpAgentModelSuggestions,
  modelSuggestionLabel,
  modelSuggestionsForProvider,
  openRouterModelsQuery,
} from '@/lib/models'
import { setPendingMessage } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import { acpReasoningEffortOptions } from '@/lib/reasoningEfforts'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { useTheme } from '@/lib/theme'
import { useQuery, useQueryClient } from '@tanstack/react-query'

type NewSearch = {
  project?: string
}

const NEW_SESSION_AGENT_KEY = 'jaz.newSession.agent'
const NEW_SESSION_DIRECTORY_KEY = 'jaz.newSession.directory'
const NEW_SESSION_DRAFT_KEY = 'jaz.newSession.prompt'

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
  // Configured ACP agent name; directory is the session cwd.
  const [runtime, setRuntime] = useState(() => storedString(NEW_SESSION_AGENT_KEY) || 'jaz')
  const [directory, setDirectory] = useState(
    () => search.project ?? storedString(NEW_SESSION_DIRECTORY_KEY),
  )
  // Worktree runs the session on a disposable git worktree (any agent);
  // only offered when the chosen directory is a git repository.
  const [worktree, setWorktree] = useState(false)
  // Per-session overrides of the Settings > Agents defaults; null follows the
  // default for the chosen agent and provider.
  const [providerOverride, setProviderOverride] = useState<string | null>(null)
  const [modelOverride, setModelOverride] = useState<string | null>(null)
  const [effortOverride, setEffortOverride] = useState<string | null>(null)
  const settingsQuery = useQuery(agentSettingsQuery)
  const agentSettings = settingsQuery.data
  const agents = useMemo(() => enabledACPAgents(agentSettings), [agentSettings])
  const runtimeReady = settingsQuery.isSuccess
  const runtimeAvailable = runtimeReady && agents.length > 0
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
    localStorage.setItem(NEW_SESSION_AGENT_KEY, runtime)
  }, [runtime])

  useEffect(() => {
    localStorage.setItem(NEW_SESSION_DIRECTORY_KEY, directory)
  }, [directory])

  useEffect(() => {
    if (!runtimeReady) return
    if (agents.includes(runtime)) return
    const next = agents.includes('jaz') ? 'jaz' : (agents[0] ?? '')
    if (next === runtime) return
    setRuntime(next)
    setProviderOverride(null)
    setModelOverride(null)
    setEffortOverride(null)
  }, [agents, runtime, runtimeReady])

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

  const runtimeModel = runtimeModelState(agentSettings, runtime, providerOverride)
  const { usesProvider, providers: runtimeProviders, provider, selectedProvider } = runtimeModel
  const defaultModel = runtimeModel.defaultModel
  const model = modelOverride ?? defaultModel
  const reasoningEffort = effortOverride ?? runtimeModel.defaultEffort

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: usesProvider && provider === 'openrouter',
  })
  const modelSuggestions = usesProvider
    ? modelSuggestionsForProvider(selectedProvider, openRouterModels.data ?? [])
    : acpAgentModelSuggestions(runtime)

  const startThread = async (title: string | undefined, prepare: (sessionId: string) => void) => {
    if (!runtimeAvailable) {
      toast('Connect an agent in Settings before starting a session.', 'danger')
      return
    }
    setCreating(true)
    try {
      const session = await createSession({
        ...(title ? { title } : {}),
        runtime: 'acp',
        agent: runtime,
        directory,
        worktree,
        ...(usesProvider && provider ? { model_provider: provider } : {}),
        ...(model ? { model } : {}),
        ...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
      })
      prepare(session.id)
      sessionStorage.removeItem(NEW_SESSION_DRAFT_KEY)
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      navigate({ to: '/sessions/$sessionId', params: { sessionId: session.id } })
    } catch (error) {
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  const handleSend = (text: string, options: SendMessageOptions = {}) =>
    startThread(text.trim(), (id) =>
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
  const modelLabel = model === '' ? agentLabel(runtime) : modelSuggestionLabel(modelSuggestions, model)
  const directoryLabel = directory
    ? (project?.name ?? directory.split(/[\\/]+/).filter(Boolean).at(-1) ?? directory)
    : ''
  // Collapsed header summary: reasoning effort and the working directory, so
  // a chosen project stays visible without opening the panel.
  const optionsSubtitle = [reasoningEffort, directoryLabel].filter(Boolean).join(' · ')

  const composerControls = (
    <>
      {runtimeReady && !runtimeAvailable ? (
        <span className="px-1.5 text-[13px] text-ink-3">Connect an agent in Settings</span>
      ) : null}
      {runtimeAvailable && agents.length > 0 ? (
        <RuntimeSelect
          value={runtime}
          agents={agents}
          placement={controlPlacement}
          disabled={creating}
          onChange={(next) => {
            setRuntime(next)
            setProviderOverride(null)
            setModelOverride(null)
            setEffortOverride(null)
          }}
        />
      ) : null}
      {runtimeAvailable ? (
        <ModelSelect
          value={model}
          suggestions={modelSuggestions}
          loading={openRouterModels.isLoading}
          placement={controlPlacement}
          disabled={creating}
          onChange={setModelOverride}
          providers={
            usesProvider
              ? runtimeProviders.map((p) => ({ value: p.id, label: p.label }))
              : undefined
          }
          provider={usesProvider ? provider : undefined}
          onProviderChange={
            usesProvider
              ? (next) => {
                  setProviderOverride(next)
                  setModelOverride(null)
                  setEffortOverride(null)
                }
              : undefined
          }
          effort={reasoningEffort}
          effortOptions={acpReasoningEffortOptions(agentSettings, runtime)}
          // Default clears the override, falling back to the settings effort.
          onEffortChange={(next) => setEffortOverride(next === '' ? null : next)}
        />
      ) : null}
      <ProjectPicker
        value={directory}
        placement={controlPlacement}
        disabled={creating}
        onChange={(path, git) => {
          setDirectory(path)
          if (!git) setWorktree(false)
        }}
      />
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
      {isMobile ? (
        <NewThreadOptions title={modelLabel} subtitle={optionsSubtitle || undefined}>
          {composerControls}
        </NewThreadOptions>
      ) : null}
      <NewSessionHome
        themeKey={resolved}
        calm={composing || creating}
        creating={creating}
        disabled={!runtimeAvailable}
        goalAvailable={acpAgentSupportsNativeGoal(agentSettings, runtime)}
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
