import { isAbsoluteFilePath, isMarkdownPath, resolveFileLink } from '@shared/fileReader'

type Node = {
  type: string
  value?: string
  url?: string
  children?: Node[]
  data?: { hName?: string; hProperties?: Record<string, string> }
}

export function markdownDocumentTitle(content: string, path: string, label = ''): string {
  return content.match(/^#\s+(.+)$/m)?.[1] ?? (label || path.split(/[\\/]/).at(-1) || 'Document')
}

export function standaloneFileReference(text: string, documentPath?: string, linked = false) {
  let value: string
  try {
    value = decodeURI(linked ? text : text.trim())
  } catch {
    return null
  }
  if (/[\r\n]/.test(value) || (!linked && !isAbsoluteFilePath(value) && /\s/.test(value.split(/[\\/]/)[0]))) return null
  return resolveFileLink(value, documentPath)
}

// headingAnchors names each heading of one document in turn, numbering repeats.
export function headingAnchors() {
  const used = new Set<string>()
  return (heading: string) => {
    const slug = heading.toLowerCase().replace(/[^\p{L}\p{N}\p{M}_\s-]/gu, '').replace(/\s/g, '-') || 'section'
    let id = slug
    let suffix = 0
    while (used.has(id)) id = `${slug}-${++suffix}`
    used.add(id)
    return id
  }
}

export function remarkHeadingAnchors() {
  return (tree: Node) => {
    const anchor = headingAnchors()
    const visit = (node: Node) => {
      if (node.type === 'heading') {
        const text = (child: Node): string => child.value ?? child.children?.map(text).join('') ?? ''
        node.data = { ...node.data, hProperties: { ...node.data?.hProperties, id: anchor(text(node)) } }
      }
      node.children?.forEach(visit)
    }
    visit(tree)
  }
}

export function remarkMarkdownDocuments({ documentPath }: { documentPath?: string }) {
  return (tree: Node) => {
    tree.children = tree.children?.map((paragraph) => {
      if (paragraph.type !== 'paragraph' || paragraph.children?.length !== 1) return paragraph
      const child = paragraph.children[0]
      const label = child.type === 'link' && child.children?.every((node) => node.type === 'text')
        ? child.children.map((node) => node.value ?? '').join('') : ''
      if (/^[@$]/.test(label)) return paragraph
      const value = child.type === 'link' ? child.url : child.type === 'text' ? child.value : undefined
      if (!value) return paragraph
      const file = standaloneFileReference(value, documentPath, child.type === 'link')
      if (!file || file.location || !isMarkdownPath(file.path)) return paragraph
      return { type: 'markdownDocument', data: { hName: 'markdown-document', hProperties: { path: file.path, label } } }
    })
  }
}
