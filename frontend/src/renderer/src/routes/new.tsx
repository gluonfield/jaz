import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { useState } from 'react'
import { ComposerCard } from '@/components/session/Composer'
import { PixelField } from '@/components/ui/PixelField'
import { useToast } from '@/components/ui/toast'
import { createSession } from '@/lib/api/sessions'
import { setPendingMessage, setPendingVoice } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import { useQueryClient } from '@tanstack/react-query'

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

  const startThread = async (title: string | undefined, prepare: (sessionId: string) => void) => {
    setCreating(true)
    try {
      const session = await createSession(title ? { title } : {})
      prepare(session.id)
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      navigate({ to: '/sessions/$sessionId', params: { sessionId: session.id } })
    } catch (error) {
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  const handleSend = (text: string) => startThread(text.trim(), (id) => setPendingMessage(id, text))
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
      <PixelField calm={composing || creating} />
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
            onSend={handleSend}
            onVoice={handleVoice}
          />
        </motion.div>
      </motion.div>
    </div>
  )
}
