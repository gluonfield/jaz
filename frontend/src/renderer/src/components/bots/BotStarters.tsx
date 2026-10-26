import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { ARRIVE } from './ChatLog'

// Things a new bot can start on, for someone who does not know yet what to
// ask. They sit under the bot's hello while its history is empty; picking one
// sends it as their first message.
const STARTERS = [
  'Every morning, go through my inbox and tell me what needs me',
  'Before each meeting, brief me on who I am meeting',
  'Keep my CRM up to date from my email and chats',
  'Research a topic for me and send me a summary',
  'Remind me about follow-ups I owe people',
]

export function BotStarters({ arriving, onPick }: { arriving: boolean; onPick: (text: string) => void }) {
  const reduced = useReducedEffectsMotion()
  return (
    <div className={`mt-2 w-fit max-w-[84%] rounded-[20px] bg-surface px-3 pt-3.5 pb-2 ${arriving && !reduced ? `${ARRIVE} [animation-delay:150ms]` : ''}`}>
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
    </div>
  )
}
