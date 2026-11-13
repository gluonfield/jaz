import { useState } from 'react'
import { hold } from './CanvasToolbar'

const STICKERS = ['👍', '👎', '❤️', '🔥', '🎉', '✅', '❌', '❓', '⭐', '💡', '⚠️', '🚀']

// Pick a sticker, then click the board where it goes.
export function StickerLayer({ onPlace }: { onPlace: (sticker: string, clientX: number, clientY: number) => void }) {
  const [sticker, setSticker] = useState(STICKERS[0])
  return (
    <>
      <div className="canvas-tool-layer" onPointerUp={(event) => onPlace(sticker, event.clientX, event.clientY)} />
      <div className="canvas-tool-options canvas-toolbar canvas-overlay" role="toolbar" aria-label="Stickers" onMouseDown={hold}>
        {STICKERS.map((value) => (
          <button key={value} type="button" className={value === sticker ? 'on' : ''} aria-label={value} aria-pressed={value === sticker} onClick={() => setSticker(value)}>
            <span className="canvas-sticker-face">{value}</span>
          </button>
        ))}
      </div>
    </>
  )
}
