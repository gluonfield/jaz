import { type ReactNode, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { ComposerCard } from '@/components/session/Composer'
import { MusicBubbles } from '@/components/home/MusicBubbles'
import { RocketVideo } from '@/components/home/RocketVideo'
import { PixelField, type PixelFieldShapeFrame } from '@/components/ui/PixelField'
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
  const [musicFrame, setMusicFrame] = useState<PixelFieldShapeFrame | null>(null)
  const [musicPlaybackActive, setMusicPlaybackActive] = useState(false)
  const [rocketFrame, setRocketFrame] = useState<PixelFieldShapeFrame | null>(null)
  const [rocketHovered, setRocketHovered] = useState(false)
  const [rocketOpen, setRocketOpen] = useState(false)

  return (
    <div
      className="relative flex h-full flex-col items-center justify-center overflow-hidden px-10 pb-16"
      onInput={(event) => {
        const el = event.target as HTMLElement
        if (el instanceof HTMLTextAreaElement) onDraftActivity(el.value.trim().length > 0)
      }}
    >
      <PixelField
        key={themeKey}
        calm={calm}
        freezeShape={musicPlaybackActive || rocketOpen}
        emphasizeShape={rocketHovered && !rocketOpen}
        onShapeFrame={(frame) => {
          setMusicFrame(frame?.shape === 'music' ? frame : null)
          setRocketFrame(frame?.shape === 'rocket' ? frame : null)
        }}
      />
      <AnimatePresence>
        {musicFrame ? (
          <MusicBubbles
            key="music-bubbles"
            frame={musicFrame}
            onPlaybackActiveChange={setMusicPlaybackActive}
          />
        ) : null}
      </AnimatePresence>
      <AnimatePresence>
        {rocketFrame ? (
          <RocketVideo
            key="rocket-video"
            frame={rocketFrame}
            onHoverChange={setRocketHovered}
            onOpenChange={setRocketOpen}
          />
        ) : null}
      </AnimatePresence>
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
