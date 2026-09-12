import type { ReactNode } from 'react'
import { motion } from 'motion/react'
import { DitherTerrain, DitherWordmark } from '@/components/launch/DitherArt'
import { ComposerCard } from '@/components/session/Composer'
import { FileDropScope } from '@/components/ui/FileDrop'
import { useHomeWordmark } from '@/lib/appearance'
import type { SendMessageHandler } from '@/lib/sendMessage'

// Welcome mode in the launcher's clothes: the dithered wordmark over a
// spotlight-style composer, standing on the boot screen's brandscape under a
// sky that follows the theme.
export function NewSessionHome({
  creating,
  disabled = false,
  goalAvailable = false,
  leftSlot,
  draftStorageKey,
  fileRoot,
  onSend,
  onVoice,
}: {
  creating: boolean
  disabled?: boolean
  goalAvailable?: boolean
  leftSlot: ReactNode
  draftStorageKey?: string
  /** directory the composer's @-mention file picker indexes ('' = workspace root) */
  fileRoot?: string
  onSend: SendMessageHandler
  onVoice?: () => void
}) {
  const wordmark = useHomeWordmark()

  return (
    <FileDropScope className="relative flex h-full flex-col overflow-hidden">
      <motion.div
        className="flex flex-1 flex-col items-center justify-center gap-8 px-10 py-8 max-sm:px-4"
        initial={{ opacity: 0, y: 14, scale: 0.985 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ type: 'spring', stiffness: 320, damping: 28 }}
      >
        <DitherWordmark text={wordmark} dot={2} />
        <div className="w-full max-w-[720px]">
          <ComposerCard
            variant="launcher"
            streaming={creating}
            autoFocus
            placeholder="Ask anything, or hand your assistant a task…"
            planAvailable
            goalControlVisible
            goalAvailable={goalAvailable}
            disabled={creating || disabled}
            leftSlot={leftSlot}
            draftStorageKey={draftStorageKey}
            clearTiming="never"
            fileRoot={fileRoot}
            onSend={onSend}
            onVoice={onVoice}
          />
        </div>
      </motion.div>
      {/* in flow, so the hero centers in whatever the brandscape leaves; a short
          window shrinks the sky, never the composer */}
      <DitherTerrain sky className="flex min-h-0 shrink flex-col justify-end" />
    </FileDropScope>
  )
}
