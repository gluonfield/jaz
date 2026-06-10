import { type ReactNode } from 'react'
import { motion } from 'motion/react'
import { HomePixelField } from '@/components/home/HomePixelField'
import { ComposerCard } from '@/components/session/Composer'
import type { SendMessageOptions } from '@/lib/sendMessage'

export function NewSessionHome({
  themeKey,
  calm,
  creating,
  leftSlot,
  onDraftActivity,
  onSend,
  onVoice,
}: {
  themeKey: string
  calm: boolean
  creating: boolean
  leftSlot: ReactNode
  onDraftActivity: (active: boolean) => void
  onSend: (text: string, options?: SendMessageOptions) => void
  onVoice?: () => void
}) {
  return (
    <div
      className="relative flex h-full flex-col items-center justify-center overflow-hidden px-10 pb-16"
      onInput={(event) => {
        const el = event.target as HTMLElement
        if (el instanceof HTMLTextAreaElement) onDraftActivity(el.value.trim().length > 0)
      }}
    >
      <HomePixelField themeKey={themeKey} calm={calm} />
      <motion.div
        className="relative z-[2] w-full max-w-[640px]"
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
            planAvailable
            leftSlot={leftSlot}
            onSend={onSend}
            onVoice={onVoice}
          />
        </motion.div>
      </motion.div>
    </div>
  )
}
