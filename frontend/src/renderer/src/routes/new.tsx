import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { useState } from 'react'
import { ComposerCard } from '@/components/session/Composer'
import { DirectoryPicker, RuntimeSelect } from '@/components/session/NewThreadControls'
import { Checkbox } from '@/components/ui/Checkbox'
import { PixelField } from '@/components/ui/PixelField'
import { useToast } from '@/components/ui/toast'
import { acpAgentsQuery, createSession } from '@/lib/api/sessions'
import { setPendingMessage, setPendingVoice } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { useTheme } from '@/lib/theme'
import { useQuery, useQueryClient } from '@tanstack/react-query'

export const Route = createFileRoute('/new')({
  component: NewSessionPage,
})

// Welcome mode (agent-council pattern): heading + composer centered as one
// group in the middle of the page; the conversation view takes over once the
// first message is on its way.
function NewSessionPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const [composing, setComposing] = useState(false)
  // 'native' or a configured ACP agent name; the directory only applies to ACP.
  const [runtime, setRuntime] = useState('native')
  const [directory, setDirectory] = useState('')
  // Worktree runs the ACP session on a disposable git worktree; only offered
  // when the chosen directory is a git repository.
  const [directoryIsGit, setDirectoryIsGit] = useState(false)
  const [worktree, setWorktree] = useState(false)
  const { data: agents = [] } = useQuery(acpAgentsQuery)
  // PixelField samples the palette at mount; remount it when the theme flips.
  const { resolved } = useTheme()

  const startThread = async (title: string | undefined, prepare: (sessionId: string) => void) => {
    setCreating(true)
    try {
      const session = await createSession(
        runtime === 'native'
          ? title
            ? { title }
            : {}
          : { ...(title ? { title } : {}), runtime: 'acp', agent: runtime, directory, worktree },
      )
      prepare(session.id)
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
        planRequested: runtime !== 'native' && Boolean(options.planRequested),
        files: options.files ?? [],
      }),
    )
  const handleVoice = () => startThread(undefined, (id) => setPendingVoice(id))

  return (
    <div
      className="relative flex h-full flex-col items-center justify-center px-10 pb-16"
      // composer keystrokes bubble up here; a non-empty draft settles the field
      onInput={(e) => {
        const el = e.target as HTMLElement
        if (el instanceof HTMLTextAreaElement) setComposing(el.value.trim().length > 0)
      }}
    >
      <PixelField key={resolved} calm={composing || creating} />
      <motion.div
        className="relative w-full max-w-[640px]"
        initial="hidden"
        animate="show"
        variants={{ hidden: {}, show: { transition: { staggerChildren: 0.07 } } }}
      >
        <motion.div
          variants={{
            hidden: { opacity: 0, y: 14, scale: 0.985 },
            show: {
              opacity: 1,
              y: 0,
              scale: 1,
              transition: { type: 'spring', stiffness: 320, damping: 28 },
            },
          }}
        >
          <ComposerCard
            streaming={creating}
            autoFocus
            translucent
            placeholder="Ask anything, or hand your assistant a task…"
            planAvailable={runtime !== 'native'}
            leftSlot={
              agents.length > 0 ? (
                <>
                  <RuntimeSelect
                    value={runtime}
                    agents={agents}
                    disabled={creating}
                    onChange={setRuntime}
                  />
                  {runtime !== 'native' ? (
                    <DirectoryPicker
                      value={directory}
                      disabled={creating}
                      onChange={(path, git) => {
                        setDirectory(path)
                        setDirectoryIsGit(git)
                        if (!git) setWorktree(false)
                      }}
                    />
                  ) : null}
                  {runtime !== 'native' && directoryIsGit ? (
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
              ) : undefined
            }
            onSend={handleSend}
            onVoice={runtime === 'native' ? handleVoice : undefined}
          />
        </motion.div>
      </motion.div>
    </div>
  )
}
