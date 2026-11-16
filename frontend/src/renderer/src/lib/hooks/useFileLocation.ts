import { useEffect } from 'react'
import type { FileLocation } from '@shared/fileReader'

export function useFileLocation(root: HTMLElement | null, location: FileLocation | undefined, content: string, active: boolean) {
  useEffect(() => {
    if (!root || !active || !location) return
    const line = location.line && root.querySelector<HTMLElement>(`[data-file-line="${location.line}"]`)
    const range = document.createRange()
    if (line && !location.text) range.selectNodeContents(line)
    else {
      const quote = location.text ?? content.split('\n')[(location.line ?? 0) - 1]?.trim()
      if (!quote) return
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
      const nodes: Text[] = []
      let text = ''
      while (walker.nextNode()) {
        const node = walker.currentNode as Text
        nodes.push(node)
        text += node.data
      }
      const start = text.toLocaleLowerCase().indexOf(quote.toLocaleLowerCase())
      if (start < 0) return
      let offset = 0
      for (const node of nodes) {
        if (offset <= start && start < offset + node.length) range.setStart(node, start - offset)
        if (offset < start + quote.length && start + quote.length <= offset + node.length) {
          range.setEnd(node, start + quote.length - offset)
          break
        }
        offset += node.length
      }
    }
    const target = location.text ? range.startContainer.parentElement : line || range.startContainer.parentElement
    target?.scrollIntoView({ block: 'center' })
    const highlight = new Highlight(range)
    CSS.highlights.set('file-location', highlight)
    const clear = () => {
      if (CSS.highlights.get('file-location') === highlight) CSS.highlights.delete('file-location')
    }
    const timer = setTimeout(clear, 3500)
    return () => {
      clearTimeout(timer)
      clear()
    }
  }, [root, location, content, active])
}
