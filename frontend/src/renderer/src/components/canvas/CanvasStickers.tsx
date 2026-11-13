import { useState } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import type { Rect } from './board'
import { hold } from './CanvasMenu'

const STICKERS = ['👍', '👎', '❤️', '🔥', '🎉', '✅', '❌', '❓', '⭐', '💡', '⚠️', '🚀']

// Pick a sticker, then click the board where it goes.
export function StickerLayer({ onPlace }: { onPlace: (make: (x: number, y: number) => Partial<CanvasItem> & Rect, clientX: number, clientY: number) => void }) {
  const [sticker, setSticker] = useState(STICKERS[0])
  return (
    <>
      <div className="canvas-tool-layer" onPointerUp={(event) => onPlace((x, y) => ({ type: 'text', text: sticker, size: 'huge', textAlign: 'center', x: x - 80, y: y - 80, width: 160, height: 160 }), event.clientX, event.clientY)} />
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
