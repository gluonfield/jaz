import { type ReactNode, useCallback } from 'react'
import { motion } from 'motion/react'
import { HomePixelField } from '@/components/home/HomePixelField'
import { ComposerCard } from '@/components/session/Composer'
import { FileDropScope } from '@/components/ui/FileDrop'
import type { SendMessageHandler } from '@/lib/sendMessage'

export function NewSessionHome({
  themeKey,
  calm,
  creating,
  disabled = false,
  leftSlot,
  draftStorageKey,
  fileRoot,
  onDraftActivity,
  onSend,
  onVoice,
}: {
  themeKey: string
  calm: boolean
  creating: boolean
  disabled?: boolean
  leftSlot: ReactNode
  draftStorageKey?: string
  /** directory the composer's @-mention file picker indexes ('' = workspace root) */
  fileRoot?: string
  onDraftActivity: (active: boolean) => void
  onSend: SendMessageHandler
  onVoice?: () => void
}) {
  const handleTextChange = useCallback(
    (text: string) => onDraftActivity(text.trim().length > 0),
    [onDraftActivity],
  )

  return (
    <FileDropScope className="relative flex h-full flex-col items-center justify-center overflow-hidden px-10 pb-16">
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
            disabled={creating || disabled}
            leftSlot={leftSlot}
            draftStorageKey={draftStorageKey}
            clearOnSend={false}
            fileRoot={fileRoot}
            onSend={onSend}
            onVoice={onVoice}
            onTextChange={handleTextChange}
          />
        </motion.div>
      </motion.div>
    </FileDropScope>
  )
}
