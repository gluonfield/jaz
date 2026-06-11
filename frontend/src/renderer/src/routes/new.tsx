import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { NewSessionHome } from '@/components/home/NewSessionHome'
import { ModelSelect, ProjectPicker, RuntimeSelect } from '@/components/session/NewThreadControls'
import { Checkbox } from '@/components/ui/Checkbox'
import { useToast } from '@/components/ui/toast'
import { acpAgentsQuery, createSession, projectsQuery } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { acpAgentModelSuggestions, OPENAI_MODELS, openRouterModelsQuery } from '@/lib/models'
import { setPendingMessage, setPendingVoice } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { useTheme } from '@/lib/theme'
import { useQuery, useQueryClient } from '@tanstack/react-query'

type NewSearch = {
  project?: string
}

const NEW_SESSION_AGENT_KEY = 'jaz.newSession.agent'
const NEW_SESSION_DIRECTORY_KEY = 'jaz.newSession.directory'
const NEW_SESSION_DRAFT_KEY = 'jaz.newSession.prompt'
const EMPTY_AGENTS: string[] = []

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
  // 'native' or a configured ACP agent name; directory is the session cwd.
  const [runtime, setRuntime] = useState(() => storedString(NEW_SESSION_AGENT_KEY) || 'native')
  const [directory, setDirectory] = useState(
    () => search.project ?? storedString(NEW_SESSION_DIRECTORY_KEY),
  )
  // Worktree runs the session on a disposable git worktree (any runtime);
  // only offered when the chosen directory is a git repository.
  const [directoryIsGit, setDirectoryIsGit] = useState(false)
  const [worktree, setWorktree] = useState(false)
  // Per-session overrides of the Settings > Agents defaults; null follows the
  // default for the chosen runtime (and, for native, provider).
  const [providerOverride, setProviderOverride] = useState<string | null>(null)
  const [modelOverride, setModelOverride] = useState<string | null>(null)
  const [effortOverride, setEffortOverride] = useState<string | null>(null)
  const agentsQuery = useQuery(acpAgentsQuery)
  const agents = agentsQuery.data ?? EMPTY_AGENTS
  const { data: agentSettings } = useQuery(agentSettingsQuery)
  const projects = useQuery(projectsQuery)
  // PixelField samples the palette at mount; remount it when the theme flips.
  const { resolved } = useTheme()

  useEffect(() => {
    if (search.project === undefined) return
    setDirectory(search.project)
    setDirectoryIsGit(false)
    setWorktree(false)
  }, [search.project])

  useEffect(() => {
    localStorage.setItem(NEW_SESSION_AGENT_KEY, runtime)
  }, [runtime])

  useEffect(() => {
    localStorage.setItem(NEW_SESSION_DIRECTORY_KEY, directory)
  }, [directory])

  useEffect(() => {
    if (!agentsQuery.isSuccess || runtime === 'native' || agents.includes(runtime)) return
    setRuntime('native')
    setProviderOverride(null)
    setModelOverride(null)
    setEffortOverride(null)
  }, [agents, agentsQuery.isSuccess, runtime])

  useEffect(() => {
    if (!directory) {
      setDirectoryIsGit(false)
      setWorktree(false)
      return
    }
    const project = projects.data?.find((item) => item.path === directory)
    if (project) {
      setDirectoryIsGit(project.git)
      if (!project.git) setWorktree(false)
    }
  }, [directory, projects.data])

  const isNative = runtime === 'native'
  const defaultProvider = agentSettings?.native.model_provider ?? ''
  const provider = providerOverride ?? defaultProvider
  const defaultModel = isNative
    ? provider === defaultProvider
      ? (agentSettings?.native.model ?? '')
      : (agentSettings?.providers.find((p) => p.id === provider)?.default_model ?? '')
    : (agentSettings?.acp[runtime]?.model ?? '')
  const model = modelOverride ?? defaultModel
  // The backend applies the native settings effort whatever the provider, and
  // each ACP agent falls back to its configured effort; mirror that here so the
  // picker shows the effort a new session will actually run with.
  const defaultEffort = isNative
    ? (agentSettings?.native.reasoning_effort ?? '')
    : (agentSettings?.acp[runtime]?.reasoning_effort ?? '')
  const reasoningEffort = effortOverride ?? defaultEffort

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: isNative && provider === 'openrouter',
  })
  const modelSuggestions = isNative
    ? provider === 'openrouter'
      ? (openRouterModels.data ?? [])
      : OPENAI_MODELS
    : acpAgentModelSuggestions(runtime)

  const startThread = async (title: string | undefined, prepare: (sessionId: string) => void) => {
    setCreating(true)
    try {
      const session = await createSession(
        isNative
          ? {
              ...(title ? { title } : {}),
              ...(directory ? { directory } : {}),
              ...(worktree ? { worktree } : {}),
              ...(provider ? { model_provider: provider } : {}),
              ...(model ? { model } : {}),
              ...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
            }
          : {
              ...(title ? { title } : {}),
              runtime: 'acp',
              agent: runtime,
              directory,
              worktree,
              ...(model ? { model } : {}),
              ...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
            },
      )
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
        files: options.files ?? [],
      }),
    )
  const handleVoice = () => startThread(undefined, (id) => setPendingVoice(id))

  const composerControls = (
    <>
      {agents.length > 0 ? (
        <RuntimeSelect
          value={runtime}
          agents={agents}
          disabled={creating}
          onChange={(next) => {
            setRuntime(next)
            setProviderOverride(null)
            setModelOverride(null)
            setEffortOverride(null)
          }}
        />
      ) : null}
      <ModelSelect
        value={model}
        suggestions={modelSuggestions}
        loading={openRouterModels.isLoading}
        disabled={creating}
        onChange={setModelOverride}
        providers={
          isNative
            ? (agentSettings?.providers ?? [])
                .filter((p) => p.implemented)
                .map((p) => ({ value: p.id, label: p.label }))
            : undefined
        }
        provider={isNative ? provider : undefined}
        onProviderChange={
          isNative
            ? (next) => {
                setProviderOverride(next)
                setModelOverride(null)
                setEffortOverride(null)
              }
            : undefined
        }
        effort={reasoningEffort}
        // Default clears the override, falling back to the settings effort.
        onEffortChange={(next) => setEffortOverride(next === '' ? null : next)}
      />
      <ProjectPicker
        value={directory}
        disabled={creating}
        onChange={(path, git) => {
          setDirectory(path)
          setDirectoryIsGit(git)
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
    <NewSessionHome
      themeKey={resolved}
      calm={composing || creating}
      creating={creating}
      leftSlot={composerControls}
      draftStorageKey={NEW_SESSION_DRAFT_KEY}
      // Tokens freeze their absolute expansion at insert time, so re-picking
      // the directory after tagging keeps old tags valid rather than rebasing
      // them.
      fileRoot={directory}
      onDraftActivity={setComposing}
      onSend={handleSend}
      onVoice={runtime === 'native' ? handleVoice : undefined}
    />
  )
}
