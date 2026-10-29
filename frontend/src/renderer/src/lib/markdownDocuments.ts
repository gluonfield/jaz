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

export function remarkHeadingAnchors() {
  return (tree: Node) => {
    const used = new Set<string>()
    const visit = (node: Node) => {
      if (node.type === 'heading') {
        const text = (child: Node): string => child.value ?? child.children?.map(text).join('') ?? ''
        const slug = text(node).toLowerCase().replace(/[^\p{L}\p{N}\p{M}_\s-]/gu, '').replace(/\s/g, '-') || 'section'
        let id = slug
        let suffix = 0
        while (used.has(id)) id = `${slug}-${++suffix}`
        used.add(id)
        node.data = { ...node.data, hProperties: { ...node.data?.hProperties, id } }
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
      let value = child.type === 'link' ? child.url : child.type === 'text' ? child.value?.trim() : undefined
      if (!value || /[\r\n]/.test(value)) return paragraph
      try {
        value = decodeURI(value)
      } catch {
        return paragraph
      }
      if (child.type === 'text' && !isAbsoluteFilePath(value) && /\s/.test(value.split(/[\\/]/)[0])) return paragraph
      const file = resolveFileLink(value, documentPath)
      if (!file || file.line || !isMarkdownPath(file.path)) return paragraph
      return { type: 'markdownDocument', data: { hName: 'markdown-document', hProperties: { path: file.path, label } } }
    })
  }
}
