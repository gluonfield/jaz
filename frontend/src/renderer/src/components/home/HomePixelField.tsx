import { useState } from 'react'
import { AnimatePresence } from 'motion/react'
import { MusicBubbles } from '@/components/home/MusicBubbles'
import { RocketVideo } from '@/components/home/RocketVideo'
import { PixelField } from '@/components/ui/PixelField'
import type {
  PixelFieldLifecycle,
  PixelFieldShapeChoiceContext,
  PixelFieldShapeFrame,
} from '@/components/ui/PixelField.types'

function chooseHomeShape({
  playlist,
  lastShape,
  constructionCount,
  defaultShape,
}: PixelFieldShapeChoiceContext) {
  if (
    playlist.includes('music') &&
    lastShape !== 'music' &&
    (constructionCount === 0 || constructionCount % 5 === 0)
  ) {
    return 'music'
  }
  if (
    playlist.includes('rocket') &&
    lastShape !== 'rocket' &&
    constructionCount > 0 &&
    constructionCount % 5 === 3
  ) {
    return 'rocket'
  }
  return defaultShape()
}

export function HomePixelField({ themeKey, calm }: { themeKey: string; calm: boolean }) {
  const [musicFrame, setMusicFrame] = useState<PixelFieldShapeFrame | null>(null)
  const [musicPlaybackActive, setMusicPlaybackActive] = useState(false)
  const [rocketFrame, setRocketFrame] = useState<PixelFieldShapeFrame | null>(null)
  const [rocketHovered, setRocketHovered] = useState(false)
  const [rocketOpen, setRocketOpen] = useState(false)

  const lifecycle: PixelFieldLifecycle = {
    chooseNextShape: chooseHomeShape,
    activeShape: ({ shape }) => {
      if (shape === 'music') return { hold: musicPlaybackActive }
      if (shape === 'rocket') {
        return {
          hold: rocketOpen,
          emphasis: rocketHovered && !rocketOpen,
        }
      }
      return null
    },
  }

  return (
    <>
      <PixelField
        key={themeKey}
        calm={calm}
        lifecycle={lifecycle}
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
    </>
  )
}
