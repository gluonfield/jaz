import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { useState } from 'react'
import { ComposerCard } from '@/components/session/Composer'
import { useToast } from '@/components/ui/toast'
import { createSession } from '@/lib/api/sessions'
import { setPendingMessage } from '@/lib/pendingMessage'
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

  const handleSend = async (text: string) => {
    setCreating(true)
    try {
      const session = await createSession({ title: text.trim() })
      setPendingMessage(session.id, text)
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      navigate({ to: '/sessions/$sessionId', params: { sessionId: session.id } })
    } catch (error) {
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
      setCreating(false)
    }
  }

  return (
    <div className="flex h-full flex-col items-center justify-center px-10 pb-16">
      <motion.div
        className="w-full max-w-[640px]"
        initial="hidden"
        animate="show"
        variants={{ hidden: {}, show: { transition: { staggerChildren: 0.07 } } }}
      >
        <motion.h1
          className="pb-4 text-center text-[1.375rem] font-medium text-ink"
          variants={{
            hidden: { opacity: 0, y: -10 },
            show: { opacity: 1, y: 0, transition: { type: 'spring', stiffness: 320, damping: 26 } },
          }}
        >
          I'm <span className="jaz-gradient font-semibold">Jaz</span>, what's on your mind?
        </motion.h1>
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
            placeholder="Ask anything, or hand your assistant a task…"
            onSend={handleSend}
          />
        </motion.div>
      </motion.div>
    </div>
  )
}
