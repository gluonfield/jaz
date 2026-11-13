import { uploadCanvasFile, type CanvasItem } from '@/lib/api/canvas'
import { newId } from './board'

const MOST = { width: 480, height: 360 }
const STEP = 24

// A picture keeps its shape within a comfortable size; anything else gets a
// viewer-sized card.
async function sizeOf(file: File): Promise<{ width: number; height: number }> {
  const bitmap = file.type.startsWith('image/') ? await createImageBitmap(file).catch(() => null) : null
  if (!bitmap) return { width: 420, height: 320 }
  const scale = Math.min(1, MOST.width / bitmap.width, MOST.height / bitmap.height)
  const size = { width: Math.round(bitmap.width * scale), height: Math.round(bitmap.height * scale) }
  bitmap.close()
  return size
}

// The files, saved into the canvas folder, as file items stepped around a
// point on the board.
export function uploadedItems(sessionId: string, files: File[], centre: { x: number; y: number }): Promise<Partial<CanvasItem>[]> {
  return Promise.all(files.map(async (file, index) => {
    const [{ file: path }, size] = await Promise.all([uploadCanvasFile(sessionId, file), sizeOf(file)])
    return {
      id: newId('file'),
      type: 'file',
      file: path,
      x: Math.round(centre.x - size.width / 2 + index * STEP),
      y: Math.round(centre.y - size.height / 2 + index * STEP),
      ...size,
    }
  }))
}
