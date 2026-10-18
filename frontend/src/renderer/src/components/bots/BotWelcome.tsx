import { motion } from 'motion/react'
import { useEffect, useState } from 'react'
import type { Bot } from '@/lib/api/types'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { ChatBubble, ChatStamp } from './ChatLog'

// Things a new bot can start on, for someone who does not know yet what to
// ask. They show only until the user says anything; picking one sends it as
// their first message.
const STARTERS = [
  'Every morning, go through my inbox and tell me what needs me',
  'Before each meeting, brief me on who I am meeting',
  'Keep my CRM up to date from my email and chats',
  'Research a topic for me and send me a summary',
  'Remind me about follow-ups I owe people',
]

const NO_MENTIONS = new Map<string, string>()
const TYPING_MS = 900
const SPRING = { type: 'spring', duration: 0.35, bounce: 0 } as const

// A new bot's first message, laid out like any message it sends: a hello and
// what it can start on. Just created, the bot types for a moment and the
// message arrives; opened later, the message is simply there.
export function BotWelcome({ bot, newBot, onPick }: { bot: Bot; newBot: boolean; onPick: (text: string) => void }) {
  const reduced = useReducedEffectsMotion()
  const animate = newBot && !reduced
  const [typing, setTyping] = useState(animate)
  useEffect(() => {
    if (!typing) return
    const timer = setTimeout(() => setTyping(false), TYPING_MS)
    return () => clearTimeout(timer)
  }, [typing])
  const arrive = (delay: number) =>
    animate ? { initial: { opacity: 0, y: 8 }, animate: { opacity: 1, y: 0 }, transition: { ...SPRING, delay } } : {}

  return (
    <div className="@container flex flex-col">
      <ChatStamp at={bot.created_at} />
      {typing ? (
        <motion.div {...arrive(0)} role="status" aria-label={`${bot.name} is typing`} className="flex w-fit gap-1 rounded-[20px] bg-surface px-4 py-4">
          {[0, 1, 2].map((dot) => (
            <motion.span
              key={dot}
              className="size-1.5 rounded-full bg-ink-3"
              animate={{ opacity: [0.3, 1, 0.3] }}
              transition={{ duration: 1, repeat: Infinity, delay: dot * 0.15 }}
            />
          ))}
        </motion.div>
      ) : (
        <div className="flex flex-col items-start gap-2">
          <motion.div {...arrive(0)} className="w-full">
            <ChatBubble text={`Hi, I'm ${bot.name}. Tell me what you need, or pick something to start with.`} at={bot.created_at} mentions={NO_MENTIONS} />
          </motion.div>
          <motion.div {...arrive(0.15)} className="max-w-[84%] rounded-[20px] bg-surface px-3 pt-3.5 pb-2">
            <p className="px-1 text-sm font-medium text-ink">What should I take on first?</p>
            <div className="mt-2 flex flex-col">
              {STARTERS.map((text, index) => (
                <button
                  key={text}
                  type="button"
                  onClick={() => onPick(text)}
                  className="flex cursor-pointer items-center gap-3 rounded-xl px-1 py-1.5 text-left text-sm text-ink transition-colors duration-150 hover:bg-list-hover"
                >
                  <span className="grid size-7 shrink-0 place-items-center rounded-lg border border-border text-[12px] text-ink-2">
                    {String.fromCharCode(65 + index)}
                  </span>
                  {text}
                </button>
              ))}
            </div>
          </motion.div>
        </div>
      )}
    </div>
  )
}
