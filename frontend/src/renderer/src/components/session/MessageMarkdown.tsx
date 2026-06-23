import { useQuery } from '@tanstack/react-query'
import { FileText, Globe } from 'lucide-react'
import {
  createContext,
  memo,
  useContext,
  useMemo,
  type ComponentProps,
  type ComponentType,
  type MouseEvent,
  type ReactNode,
} from 'react'
import Markdown, { type Components, type ExtraProps } from 'react-markdown'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import { skillsQuery, type SkillInfo } from '@/lib/api/skills'
import { findFileReferences, parseFileReference, shouldPreviewFileReference, type FileReference } from '../../../../shared/fileReader'
import { shouldPreviewURLByDefault } from '../../../../shared/preview'
import { CodeBlock } from './CodeBlock'
import { encodeMention } from './mentionCodec'
import { MentionPill } from './mentions'

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

function shouldPreviewLink(event: MouseEvent<HTMLElement>): boolean {
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
  const matches = findFileReferences(value)
  if (!matches.length) return null
  const nodes: MarkdownNode[] = []
  let lastIndex = 0
  for (const { raw, index, reference } of matches) {
    if (index > lastIndex) nodes.push({ type: 'text', value: value.slice(lastIndex, index) })
    nodes.push({
      type: 'link',
      url: reference.line ? `${reference.path}:${reference.line}` : reference.path,
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

type AnchorComponent = ComponentType<ComponentProps<'a'> & ExtraProps>

function BaseMarkdown({
  text,
  className,
  Link,
}: {
  text: string
  className: string
  Link: AnchorComponent
}) {
  const prepared = useMemo(() => normalizeMath(text), [text])
  const components = useMemo<Components>(() => ({ a: Link, pre: CodeBlock }), [Link])
  return (
    <div className={className}>
      <Markdown
        remarkPlugins={[remarkGfm, [remarkMath, { singleDollarTextMath: false }], remarkFileReferences]}
        rehypePlugins={[rehypeKatex]}
        components={components}
      >
        {prepared}
      </Markdown>
    </div>
  )
}

const MessageMarkdownLink: AnchorComponent = ({ children, href, ...props }) => {
  const label = textFromChildren(children)
  const sigil = mentionSigil(label)
  if (sigil && typeof href === 'string' && href !== '') {
    return <MentionPill mention={{ sigil, name: label.slice(1), target: decodeMentionHref(href) }} />
  }
  return <PlainMarkdownLink {...props} href={href}>{children}</PlainMarkdownLink>
}

const PlainMarkdownLink: AnchorComponent = ({ node: _node, children, href, onClick, ...props }) => {
  const openPreview = useContext(PreviewLinkContext)
  const openFile = useContext(FileReaderLinkContext)
  const localFile = localFileFromLink(href, children)
  const urlLink = isUrlLink(href)
  const Icon = markdownLinkIcon(localFile, urlLink)
  if (localFile) {
    return (
      <button
        type="button"
        className="chat-prose-link-button"
        onClick={(event) => {
          if (openFile && shouldPreviewLink(event)) openFile(localFile)
        }}
      >
        <FileText
          aria-hidden="true"
          className="chat-prose-link-icon"
          size={13}
          strokeWidth={1.7}
        />
        {children}
      </button>
    )
  }
  if (!urlLink) return <>{children}</>
  return (
    <a
      {...props}
      href={href}
      target="_blank"
      rel="noreferrer"
      onClick={(event) => {
        onClick?.(event)
        if (
          !openPreview ||
          typeof href !== 'string' ||
          !shouldPreviewURLByDefault(href) ||
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
}

function markdownLinkIcon(localFile: FileReference | null, urlLink: boolean) {
  if (localFile) return shouldPreviewFileReference(localFile) ? Globe : FileText
  return urlLink ? Globe : null
}

export const RenderedMarkdown = memo(function RenderedMarkdown({
  text,
  className = 'chat-prose',
}: {
  text: string
  className?: string
}) {
  return <BaseMarkdown text={text} className={className} Link={PlainMarkdownLink} />
})

// Shared renderer for assistant prose: GitHub-flavored markdown + LaTeX via KaTeX.
// Memoized: the remark/rehype pipeline is the priciest per-item work in a
// transcript, so it must only run when the text actually changes.
export const MessageMarkdown = memo(function MessageMarkdown({ text }: { text: string }) {
  const skills = useQuery(skillsQuery())
  const prepared = useMemo(() => linkifyKnownSkills(text, skills.data ?? []), [text, skills.data])
  return <BaseMarkdown text={prepared} className="chat-prose" Link={MessageMarkdownLink} />
})

// The markdown pipeline percent-encodes hrefs; show the filesystem path.
function decodeMentionHref(href: string): string {
  try {
    return decodeURI(href)
  } catch {
    return href
  }
}
