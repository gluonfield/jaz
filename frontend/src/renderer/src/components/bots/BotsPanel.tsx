import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Repeat } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type DragEvent, Fragment, type Ref, useMemo, useState } from 'react'
import { stateDot } from '@/components/sidebar/SessionRow'
import { PANEL_ICON_BUTTON_CLASS, SidebarHeader, SidebarScroll } from '@/components/sidebar/SidebarScroll'
import { MarkdownText } from '@/components/session/MessageMarkdown'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { botsQuery, type BotListEntry } from '@/lib/api/bots'
import type { Bot } from '@/lib/api/types'
import { botAvatars, type BotMood, botMood, dragSections, pinOrder, placePin } from '@/lib/bots'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import { BotIcon } from './BotAvatar'
import { BotMenu } from './BotMenu'
import { BotNameInput } from './BotNameInput'
import { NewBotPicker } from './NewBotPicker'
import { usePins } from './usePins'

// A dragged bot and the pinned order it would leave behind if dropped now.
type PinDrag = { id: string; pins: string[] }

const SPRING = { type: 'spring', duration: 0.3, bounce: 0 } as const

export function BotsPanel({ mobile }: { mobile: boolean }) {
  const bots = useQuery(botsQuery)
  const list = useMemo(() => bots.data ?? [], [bots.data])
  const pins = useMemo(() => pinOrder(list), [list])
  const pin = usePins()
  const [drag, setDrag] = useState<PinDrag | null>(null)
  const { tiles, rows } = dragSections(list, pins, drag ?? undefined)
  const startDrag = (id: string) => setDrag({ id, pins })

  return (
    <>
      <SidebarHeader>
        <Link to="/loops" aria-label="Loops" title="Loops" activeProps={{ className: 'bg-list-active! text-ink!' }} className={`ml-auto ${PANEL_ICON_BUTTON_CLASS}`}>
          <Repeat size={15} className="max-sm:size-[18px]" />
        </Link>
        <NewBotPicker bots={list} />
      </SidebarHeader>
      <SidebarScroll mobile={mobile} tight>
        {bots.isPending ? (
          <SkeletonRows count={4} />
        ) : bots.isError && !bots.data ? (
          <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
        ) : (
          <section
            onDragOver={(e) => {
              if (!drag) return
              e.preventDefault()
              const next = landing(e, drag, tiles.flatMap(({ bot, hidden }) => (hidden ? [] : [bot.id])))
              if (next.join() !== drag.pins.join()) setDrag({ ...drag, pins: next })
            }}
            onDrop={(e) => {
              if (!drag) return
              e.preventDefault()
              setDrag(null)
              pin(() => drag.pins)
            }}
            onDragEnd={() => setDrag(null)}
            className="flex flex-1 shrink-0 flex-col gap-1.5"
          >
            {tiles.length ? (
              <div data-pins className="grid auto-rows-fr grid-cols-[repeat(auto-fill,minmax(76px,1fr))] gap-px">
                <AnimatePresence initial={false} mode="popLayout">
                  {tiles.map(({ bot, hidden }) => (
                    <BotEntry key={bot.id} bot={bot} bots={list} tile dragged={drag?.id === bot.id} hidden={hidden} onDrag={startDrag} />
                  ))}
                </AnimatePresence>
              </div>
            ) : null}
            <div className="flex flex-col gap-0.5">
              <AnimatePresence initial={false} mode="popLayout">
                {rows.map(({ bot, hidden }) => (
                  <BotEntry key={bot.id} bot={bot} bots={list} dragged={drag?.id === bot.id} hidden={hidden} onDrag={startDrag} />
                ))}
              </AnimatePresence>
            </div>
            {!list.length ? <p className="px-2.5 py-1 text-[13px] text-ink-3">No bots yet</p> : null}
          </section>
        )}
      </SidebarScroll>
    </>
  )
}

// Where a dragged bot lands among the pins: beside the tile in the grid cell
// under the pointer, last past the tiles, or out of the pins anywhere else.
// Cells come from the grid's tracks, not the tiles, so a tile still sliding
// cannot bounce the drop back, and the columns follow the panel's width.
function landing(e: DragEvent, drag: PinDrag, tiles: string[]): string[] {
  const grid = (e.target as Element).closest('[data-pins]')
  if (!grid) return drag.pins.filter((id) => id !== drag.id)
  const box = grid.getBoundingClientRect()
  const style = getComputedStyle(grid)
  const columns = style.gridTemplateColumns.split(' ').length
  const width = parseFloat(style.gridTemplateColumns) + parseFloat(style.columnGap)
  const height = parseFloat(style.gridTemplateRows) + parseFloat(style.rowGap)
  const x = e.clientX - box.left
  const column = Math.min(columns - 1, Math.floor(x / width))
  const cell = Math.floor((e.clientY - box.top) / height) * columns + column
  return placePin(drag.pins, drag.id, tiles[cell], x - column * width > width / 2)
}

// A question waiting on the person always gets a dot, as do an unread reply
// and a failure; work gets one only when the face holds still and can't show
// it.
function botDot(bot: Bot, mood: BotMood, still: boolean): { title: string; className: string } | null {
  if (mood === 'waiting') {
    return { title: 'Waiting for your answer', className: 'bg-accent' }
  }
  if ((mood === 'working' || mood === 'thinking') && !still) {
    return null
  }
  return stateDot(bot)
}

// A pinned bot is a tile (face over name); the rest are rows with a preview.
// Tiles and rows slide to new places, and scale in and out as they come and go.
function BotEntry({ ref, bot, bots, tile = false, dragged, hidden, onDrag }: {
  ref?: Ref<HTMLDivElement>
  bot: BotListEntry
  bots: Bot[]
  tile?: boolean
  dragged: boolean
  hidden: boolean
  onDrag: (id: string) => void
}) {
  const [renaming, setRenaming] = useState(false)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const menuTriggers = useContextMenuTrigger(setMenu)
  // What the bot is doing shows on its face; what the face can't carry shows
  // as a dot: on a tile's face, so its name keeps the full width, and beside a
  // row's name.
  const mood = botMood(bot)
  const state = botDot(bot, mood, useReducedEffectsMotion())
  const dot = state ? <span role="img" aria-label={state.title} title={state.title} className={`size-1.5 shrink-0 rounded-full ${state.className}`} /> : null
  const name = renaming ? (
    <BotNameInput
      bot={bot}
      autoFocus
      onDone={() => setRenaming(false)}
      className={`w-full rounded bg-surface-1 px-1.5 py-0.5 text-[13px] ring-1 ring-primary ${tile ? 'text-center' : ''}`}
    />
  ) : tile ? (
    // Words wrap whole; one too long for the tile ends in an ellipsis instead
    // of splitting.
    <span className="line-clamp-2 max-w-full min-w-0 text-center leading-tight">
      {bot.name.split(/\s+/).map((word, index) => (
        <Fragment key={index}>
          {index ? ' ' : null}
          <span className="inline-block max-w-full truncate align-top">{word}</span>
        </Fragment>
      ))}
    </span>
  ) : (
    <span className="min-w-0 truncate">{bot.name}</span>
  )

  return (
    <motion.div
      ref={ref}
      hidden={hidden}
      layout="position"
      initial={{ opacity: 0, scale: 0.9 }}
      animate={{ opacity: dragged ? 0.4 : 1, scale: 1 }}
      exit={{ opacity: 0, scale: 0.9 }}
      transition={SPRING}
      className="min-w-0"
    >
      <Link
        to="/bots/$botId"
        params={{ botId: bot.id }}
        activeProps={{ className: 'bg-list-active!' }}
        {...menuTriggers}
        onDragStart={(e) => {
          // The bot itself is dragged, not its link: no URL card, and nothing
          // to drop into a text field.
          const box = e.currentTarget.getBoundingClientRect()
          e.dataTransfer.clearData()
          e.dataTransfer.setDragImage(e.currentTarget, e.clientX - box.left, e.clientY - box.top)
          e.dataTransfer.effectAllowed = 'move'
          onDrag(bot.id)
        }}
        className={`select-none rounded-lg text-ink transition-colors duration-150 [-webkit-touch-callout:none] hover:bg-list-hover ${
          tile
            ? 'flex h-full min-w-0 flex-col items-center gap-1.5 px-1 pt-2.5 pb-2 text-[13px] max-sm:text-[14px]'
            : 'flex h-13 items-center gap-2.5 px-2.5 text-[13px] max-sm:h-16 max-sm:text-[15px]'
        }`}
      >
        <BotIcon avatars={botAvatars(bot, bots)} size={tile ? 56 : 28} mood={mood} completion={bot.celebrate} live badge={tile ? state : null} main={bot.main} />
        {tile ? (
          name
        ) : (
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="flex items-center justify-between gap-2">
              {name}
              {dot}
            </span>
            {bot.preview ? (
              <span className="truncate text-[12px] text-ink-3">
                <MarkdownText text={bot.preview} />
              </span>
            ) : null}
          </span>
        )}
      </Link>
      {menu ? <BotMenu bot={bot} point={menu} onClose={() => setMenu(null)} onRename={() => setRenaming(true)} /> : null}
    </motion.div>
  )
}
