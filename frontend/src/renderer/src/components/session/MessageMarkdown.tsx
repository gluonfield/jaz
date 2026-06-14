import { useQuery } from '@tanstack/react-query'
import { FileText, Globe } from 'lucide-react'
import { createContext, memo, useContext, useMemo, type MouseEvent, type ReactNode } from 'react'
import Markdown from 'react-markdown'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import { skillsQuery, type SkillInfo } from '@/lib/api/skills'
import { parseFileReference, type FileReference } from '../../../../shared/fileReader'
import { encodeMention, MentionPill } from './mentions'

const PreviewLinkContext = createContext<((url: string) => void) | null>(null)
const FileReaderLinkContext = createContext<((file: FileReference) => void) | null>(null)

export function PreviewLinkProvider({
  onOpen,
  children,
}: {
  onOpen: (url: string) => void
  children: ReactNode
}) {
  return <PreviewLinkContext.Provider value={onOpen}>{children}</PreviewLinkContext.Provider>
}

export function FileReaderLinkProvider({
  onOpen,
  children,
}: {
  onOpen: (file: FileReference) => void
  children: ReactNode
}) {
  return <FileReaderLinkContext.Provider value={onOpen}>{children}</FileReaderLinkContext.Provider>
}

// Models often emit \[...\] / \(...\) math delimiters; remark-math only
// parses dollar-style math. Convert outside of code spans/fences.
function normalizeMath(text: string): string {
  return text
    .split(/(```[\s\S]*?```|`[^`]*`)/g)
    .map((part, i) =>
      i % 2 === 1
        ? part
        : part
            .replace(/\\\[([\s\S]*?)\\\]/g, (_, expr: string) => `\n$$\n${expr}\n$$\n`)
            .replace(/\\\(([\s\S]*?)\\\)/g, (_, expr: string) => `$$${expr}$$`),
    )
    .join('')
}

function textFromChildren(children: unknown): string {
  if (typeof children === 'string' || typeof children === 'number') return String(children)
  if (Array.isArray(children)) return children.map(textFromChildren).join('')
  if (children && typeof children === 'object' && 'props' in children) {
    return textFromChildren((children as { props?: { children?: unknown } }).props?.children)
  }
  return ''
}

function localFileFromLink(href: unknown, children: unknown): FileReference | null {
  if (typeof href === 'string') {
    const fromHref = parseFileReference(decodeMentionHref(href))
    if (fromHref) return fromHref
  }
  return parseFileReference(textFromChildren(children).trim())
}

function isUrlLink(href: unknown): boolean {
  return typeof href === 'string' && /^https?:\/\//i.test(href)
}

function shouldPreviewLink(event: MouseEvent<HTMLAnchorElement>): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey &&
    !event.defaultPrevented
  )
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// When the assistant echoes a skill by its $name (without the linked-mention
// form the composer sends), wrap it as a linked mention so it renders as the
// same pill the user's message shows. Only catalog names qualify — arbitrary
// $words stay untouched — and code spans/fences are left alone.
function linkifyKnownSkills(text: string, skills: SkillInfo[]): string {
  if (skills.length === 0 || !text.includes('$')) return text
  return text
    .split(/(```[\s\S]*?```|`[^`]*`)/g)
    .map((part, i) => {
      if (i % 2 === 1) return part
      let out = part
      for (const skill of skills) {
        const pattern = new RegExp(`(?<![\\w[\\\\-])\\$${escapeRegExp(skill.name)}(?![\\w-])`, 'g')
        out = out.replace(pattern, () => encodeMention('$', skill.name, skill.path))
      }
      return out
    })
    .join('')
}

const FILE_REFERENCE_PATTERN =
  /(?:file:\/\/\/|\/)(?:[^\s<>(){}]+\/)+[^\s<>(){}]+\.[A-Za-z0-9][A-Za-z0-9]*(?::\d+)?/g

type MarkdownNode = {
  type: string
  value?: string
  url?: string
  title?: string | null
  children?: MarkdownNode[]
}

const FILE_REFERENCE_SKIP_NODES = new Set([
  'code',
  'definition',
  'html',
  'image',
  'imageReference',
  'inlineCode',
  'inlineMath',
  'link',
  'linkReference',
  'math',
])

function remarkFileReferences() {
  return (tree: MarkdownNode) => {
    linkifyFileReferenceNodes(tree)
  }
}

function linkifyFileReferenceNodes(node: MarkdownNode): void {
  if (!node.children || FILE_REFERENCE_SKIP_NODES.has(node.type)) return
  for (let i = 0; i < node.children.length; i++) {
    const child = node.children[i]
    if (child.type === 'text' && typeof child.value === 'string') {
      const replacement = fileReferenceTextNodes(child.value)
      if (replacement) {
        node.children.splice(i, 1, ...replacement)
        i += replacement.length - 1
      }
      continue
    }
    linkifyFileReferenceNodes(child)
  }
}

function fileReferenceTextNodes(value: string): MarkdownNode[] | null {
  if (!value.includes('/')) return null
  const nodes: MarkdownNode[] = []
  let lastIndex = 0
  FILE_REFERENCE_PATTERN.lastIndex = 0
  for (const match of value.matchAll(FILE_REFERENCE_PATTERN)) {
    const raw = match[0]
    const ref = parseFileReference(raw)
    if (!ref) continue
    const index = match.index ?? 0
    if (index > lastIndex) nodes.push({ type: 'text', value: value.slice(lastIndex, index) })
    nodes.push({
      type: 'link',
      url: ref.line ? `${ref.path}:${ref.line}` : ref.path,
      title: null,
      children: [{ type: 'text', value: raw }],
    })
    lastIndex = index + raw.length
  }
  if (!nodes.length) return null
  if (lastIndex < value.length) nodes.push({ type: 'text', value: value.slice(lastIndex) })
  return nodes
}

function mentionSigil(label: string): '$' | '@' | null {
  return label.startsWith('$') || label.startsWith('@') ? (label[0] as '$' | '@') : null
}

// Shared renderer for assistant prose: GitHub-flavored markdown + LaTeX via KaTeX.
// Memoized: the remark/rehype pipeline is the priciest per-item work in a
// transcript, so it must only run when the text actually changes.
export const MessageMarkdown = memo(function MessageMarkdown({ text }: { text: string }) {
  // Cached by the composer; lets assistant echoes of $skill-name render as
  // mention pills. An empty catalog simply skips the pass.
  const skills = useQuery(skillsQuery)
  const openPreview = useContext(PreviewLinkContext)
  const openFile = useContext(FileReaderLinkContext)
  const prepared = useMemo(
    () => normalizeMath(linkifyKnownSkills(text, skills.data ?? [])),
    [text, skills.data],
  )
  return (
    <div className="chat-prose">
      <Markdown
        remarkPlugins={[remarkGfm, [remarkMath, { singleDollarTextMath: false }], remarkFileReferences]}
        rehypePlugins={[rehypeKatex]}
        components={{
          a: ({ node: _node, children, href, onClick, ...props }) => {
            // Linked mentions ([$skill](path) / [@path](abs)) render as the
            // composer's pills, not as links.
            const label = textFromChildren(children)
            const sigil = mentionSigil(label)
            if (sigil && typeof href === 'string' && href !== '') {
              return (
                <MentionPill
                  mention={{ sigil, name: label.slice(1), target: decodeMentionHref(href) }}
                />
              )
            }
            const localFile = localFileFromLink(href, children)
            const urlLink = isUrlLink(href)
            const Icon = localFile ? FileText : urlLink ? Globe : null
            return (
              <a
                {...props}
                href={href}
                target={localFile ? undefined : '_blank'}
                rel="noreferrer"
                onClick={(event) => {
                  onClick?.(event)
                  if (localFile && openFile && shouldPreviewLink(event)) {
                    event.preventDefault()
                    openFile(localFile)
                    return
                  }
                  if (
                    !openPreview ||
                    typeof href !== 'string' ||
                    !urlLink ||
                    !shouldPreviewLink(event)
                  ) {
                    return
                  }
                  event.preventDefault()
                  openPreview(href)
                }}
              >
                {Icon ? (
                  <Icon
                    aria-hidden="true"
                    className="chat-prose-link-icon"
                    size={13}
                    strokeWidth={1.7}
                  />
                ) : null}
                {children}
              </a>
            )
          },
        }}
      >
        {prepared}
      </Markdown>
    </div>
  )
})

// The markdown pipeline percent-encodes hrefs; show the filesystem path.
function decodeMentionHref(href: string): string {
  try {
    return decodeURI(href)
  } catch {
    return href
  }
}
