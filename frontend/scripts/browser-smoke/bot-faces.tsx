import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BotAvatar } from '@/components/bots/BotAvatar'
import { getAppearance, setAppearance } from '@/lib/appearance'
import type { BotAvatar as Avatar } from '@/lib/api/types'
import type { BotMood } from '@/lib/bots'

const FACES = ['a', 'b', 'c', 'd'] as const
type FaceID = (typeof FACES)[number]

const AVATAR: Avatar = { shape: 'circle', color: 'amber' }

// Live faces in StrictMode, as the app mounts them: they move, act out each
// change of mood once, hold still when effects are off, and leave still faces
// and faces off screen alone.
export async function exerciseBotFaces(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:50;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const show = (moods: Record<FaceID, BotMood>, shapes: Partial<Record<FaceID, Avatar['shape']>> = {}) => {
    root.render(
      <StrictMode>
        <div style={{ display: 'flex', gap: 32, padding: 48 }}>
          {FACES.map((id) => (
            <span key={id} data-test-face={id}>
              <BotAvatar avatar={{ ...AVATAR, shape: shapes[id] ?? AVATAR.shape }} size={64} mood={moods[id]} live />
            </span>
          ))}
          <span data-test-face="still">
            <BotAvatar avatar={AVATAR} size={64} mood="working" />
          </span>
          <span data-test-face="offscreen" style={{ position: 'fixed', top: -400, left: 0 }}>
            <BotAvatar avatar={AVATAR} size={64} live />
          </span>
        </div>
      </StrictMode>,
    )
  }
  const part = (id: string, name: 'body' | 'eye') => element.querySelector(`[data-test-face="${id}"] [data-face="${name}"]`)
  const moved = (id: string) => part(id, 'body')?.getAttribute('transform') ?? null
  const eyes = (id: string) => part(id, 'eye')?.getAttribute('d') ?? ''
  // The body's offset, from its transform's leading translate.
  const offset = (id: string) => {
    const match = /^translate\(([-\d.e]+) ([-\d.e]+)\)/.exec(moved(id) ?? '')
    return match ? { x: Number(match[1]), y: Number(match[2]) } : { x: 0, y: 0 }
  }
  const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
  const until = async (what: string, check: () => boolean, ms = 3000) => {
    const end = performance.now() + ms
    while (!check()) {
      if (performance.now() > end) {
        throw new Error(`Bot faces: ${what}`)
      }
      await new Promise(requestAnimationFrame)
    }
  }
  const effects = getAppearance().effects

  try {
    setAppearance({ effects: true })
    const moods: Record<FaceID, BotMood> = { a: 'idle', b: 'idle', c: 'away', d: 'waiting' }
    show(moods)
    await until('the faces never rendered', () => part('offscreen', 'body') !== null)
    await until('an idle face never moved', () => moved('a') !== null)
    const before = moved('a')
    await wait(300)
    if (moved('a') === before) {
      throw new Error('Bot faces: an idle face froze')
    }
    if (moved('still') !== null || moved('offscreen') !== null) {
      throw new Error('Bot faces: a still face or one off screen moved')
    }
    if (!eyes('c').includes('h10')) {
      throw new Error('Bot faces: an away face is not asleep: ' + eyes('c'))
    }
    const look = /^translate\(([-\d.]+)/.exec(part('d', 'eye')?.getAttribute('transform') ?? '')
    if (!look || Math.abs(Number(look[1]) - 40) > 2) {
      throw new Error('Bot faces: a waiting face is not looking at the person: ' + part('d', 'eye')?.getAttribute('transform'))
    }
    await window.smoke.capture('bot-faces')

    show({ ...moods, a: 'working' })
    await until('a working face never bobbed', () => offset('a').y < -1.5)

    show({ ...moods, a: 'idle', b: 'failed' })
    await until('finishing work did not cheer', () => eyes('a').includes('q6 -10'), 800)
    await until('a failed face did not shake', () => eyes('b').includes('L') && Math.abs(offset('b').x) > 0.5, 800)
    await until('the cheer and the shake did not end', () => eyes('a').includes('l4 14') && eyes('b').includes('l4 14'))
    const end = performance.now() + 1500
    while (performance.now() < end) {
      if (eyes('a').includes('q6 -10') || eyes('b').includes('L')) {
        throw new Error('Bot faces: a one-off move played again')
      }
      await new Promise(requestAnimationFrame)
    }

    // A new shape moves the eyes to that shape's eye line.
    show(moods, { a: 'cloud' })
    await until('a face that changed shape kept the old eyes', () => eyes('a') === 'M45 47l4 14')

    // Effects going off as a mood changes leaves the new mood's eyes.
    setAppearance({ effects: false })
    show({ ...moods, c: 'idle' }, { a: 'cloud' })
    await until('turning effects off did not still the faces', () => FACES.every((id) => moved(id) === null))
    if (eyes('c') !== 'M45 39l4 14' || eyes('a') !== 'M45 47l4 14') {
      throw new Error(`Bot faces: stilled faces kept stale eyes: ${eyes('c')} / ${eyes('a')}`)
    }
    await wait(200)
    if (FACES.some((id) => moved(id) !== null)) {
      throw new Error('Bot faces: a face moved with effects off')
    }
  } finally {
    setAppearance({ effects })
    root.unmount()
    element.remove()
  }
}
