import { parser as createXMLParser } from 'sax'

export function visit(xml: string, onTag: (name: string, attributes: Record<string, string>) => void, onText?: (name: string, text: string) => void) {
  if (!xml) return
  const parser = createXMLParser(true, { xmlns: true })
  let current = ''
  parser.onopentag = (tag) => {
    current = tag.local
    onTag(tag.local, Object.fromEntries(Object.values(tag.attributes).map((attribute) => [attribute.local, attribute.value])))
  }
  parser.onclosetag = () => {
    current = ''
  }
  if (onText) parser.ontext = (text) => onText(current, text)
  parser.write(xml).close()
}

export function relationships(read: (path: string) => string, part: string): Map<string, { path: string; type: string }> {
  const directory = part.slice(0, part.lastIndexOf('/'))
  const result = new Map<string, { path: string; type: string }>()
  visit(read(`${directory}/_rels/${part.slice(directory.length + 1)}.rels`), (name, attrs) => {
    if (name !== 'Relationship' || attrs.TargetMode === 'External') return
    const parts: string[] = []
    for (const segment of (attrs.Target.startsWith('/') ? attrs.Target.slice(1) : `${directory}/${attrs.Target}`).split('/')) {
      if (segment === '..') parts.pop()
      else if (segment !== '.') parts.push(segment)
    }
    result.set(attrs.Id, { path: parts.join('/'), type: attrs.Type })
  })
  return result
}
