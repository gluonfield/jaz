import { useEffect, useRef, useState } from 'react'
import type { FileLocation } from '@shared/fileReader'

export function useFileLocation(root: HTMLElement | null, location: FileLocation | undefined, content: string, active: boolean, lineText?: string) {
  const [navigation, setNavigation] = useState<{ location: FileLocation; target: HTMLElement | null } | null>(null)
  const applied = useRef<FileLocation | undefined>(undefined)
  useEffect(() => {
    if (!root || !location) return
    const line = location.line && root.querySelector<HTMLElement>(`[data-file-line="${location.line}"]`)
    const range = document.createRange()
    if (line && !location.text) range.selectNodeContents(line)
    else {
      const quote = location.text ?? lineText ?? content.split('\n')[(location.line ?? 0) - 1]?.trim()
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
    setNavigation({ location, target })
    const highlight = CSS.highlights.get('file-location') ?? new Highlight()
    highlight.add(range)
    CSS.highlights.set('file-location', highlight)
    return () => {
      highlight.delete(range)
      if (!highlight.size) CSS.highlights.delete('file-location')
    }
  }, [root, location, content, lineText])
  useEffect(() => {
    if (!active || !navigation || navigation.location !== location || applied.current === location) return
    navigation.target?.scrollIntoView({ block: 'center' })
    applied.current = location
  }, [active, location, navigation])
}
