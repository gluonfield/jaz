import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FileText } from 'lucide-react'
import {
  createContext,
  memo,
  useContext,
  useId,
  useMemo,
  type ComponentProps,
  type ComponentType,
  type ReactNode,
} from 'react'
import Markdown, { defaultUrlTransform, type Components, type ExtraProps, type Options } from 'react-markdown'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import { Favicon } from '@/components/ui/Favicon'
import { MarkdownImage, MarkdownImageLinkContext } from '@/components/session/MarkdownImage'
import { skillsQuery, type SkillInfo } from '@/lib/api/skills'
import { markdownImageSource } from '@/lib/markdownImages'
import { decoded, isPlainClick, markdownLinkTarget } from '@/lib/markdownLinks'
import { remarkMathDelimiters } from '@/lib/markdownMath'
import { remarkHeadingAnchors, remarkMarkdownDocuments } from '@/lib/markdownDocuments'
import { MarkdownDocumentCard } from '@/components/session/MarkdownDocumentCard'
import { MarkdownArtifactContext } from '@/components/session/MarkdownArtifact'
import { findFileReferences, isMarkdownPath, parseFileReference, resolveFileLink } from '@shared/fileReader'
import { FileReaderLinkProvider, useFileLinks } from '@/lib/fileLinks'
export { FileReaderLinkProvider } from '@/lib/fileLinks'
import { CodeBlock } from './CodeBlock'
import { ConnectCard } from './ConnectCard'
import { encodeMention } from './mentionCodec'
import { MentionPill } from '@/components/session/mentions'
import { botIdFromTarget } from '@/lib/bots'

const PreviewLinkContext = createContext<((url: string) => void) | null>(null)
export function usePreviewLink() {
  return useContext(PreviewLinkContext)
}

export function PreviewLinkProvider({
  onOpen,
  children,
}: {
  onOpen: (url: string) => void
  children: ReactNode
}) {
  return <PreviewLinkContext.Provider value={onOpen}>{children}</PreviewLinkContext.Provider>
}

function textFromChildren(children: unknown): string {
  if (typeof children === 'string' || typeof children === 'number') return String(children)
  if (Array.isArray(children)) return children.map(textFromChildren).join('')
  if (children && typeof children === 'object' && 'props' in children) {
    return textFromChildren((children as { props?: { children?: unknown } }).props?.children)
  }
  return ''
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
        const pattern = new RegExp(`(?<![\\w.[\\\\:-])\\$${escapeRegExp(skill.name)}(?![\\w:-]|\\.\\w)`, 'g')
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
  data?: { hName: string; hProperties: Record<string, string> }
}

const CONNECT_LINK = /^jaz:\/\/(?:connect\/([\w.-]+)|plugin\/([\w.-]+)\/connect)$/

// A paragraph holding only jaz://plugin/<id>/connect or jaz://connect/<id>
// links, bare or labelled, becomes one connect card per link. In a sentence or
// code the link stays text.
function remarkConnectCards() {
  return function replace(node: MarkdownNode): void {
    if (!node.children) return
    node.children = node.children.flatMap((child) => {
      const plugins = child.type === 'paragraph' ? connectCardPlugins(child) : null
      if (!plugins) {
        replace(child)
        return [child]
      }
      return plugins.map((plugin) => ({ type: 'connectCard', data: { hName: 'connect-card', hProperties: { plugin } } }))
    })
  }
}

function connectCardPlugins(paragraph: MarkdownNode): string[] | null {
  const plugins: string[] = []
  for (const child of paragraph.children ?? []) {
    if (child.type !== 'link' && child.type !== 'text' && child.type !== 'break') return null
    const links = child.type === 'link' ? [child.url ?? ''] : (child.value ?? '').split(/\s+/).filter(Boolean)
    for (const link of links) {
      const match = CONNECT_LINK.exec(link)
      const plugin = match?.[1] || match?.[2]
      if (!plugin) return null
      plugins.push(plugin)
    }
  }
  return plugins.length ? plugins : null
}

function remarkFileReferences() {
  return (tree: MarkdownNode) => {
    rewriteTextNodes(tree, fileReferenceTextNodes)
  }
}

function remarkLineBreaks() {
  return (tree: MarkdownNode) => {
    rewriteTextNodes(tree, lineBreakTextNodes)
  }
}

function remarkMentions(mentions: ReadonlyMap<string, string>) {
  return (tree: MarkdownNode) => {
    rewriteTextNodes(tree, (value) => {
      const nodes: MarkdownNode[] = []
      let end = 0
      for (const match of value.matchAll(/\[@([^[\]\r\n]+)\]/g)) {
        const target = mentions.get(match[1])
        if (!target) continue
        nodes.push({ type: 'text', value: value.slice(end, match.index) })
        nodes.push({
          type: 'link',
          url: target,
          children: [{ type: 'text', value: '@' + match[1] }],
        })
        end = match.index + match[0].length
      }
      return nodes.length ? [...nodes, { type: 'text', value: value.slice(end) }] : null
    })
  }
}

// Code, math, HTML and images carry no child nodes. Link labels are left as
// written, so a file reference inside one never becomes a nested link.
function rewriteTextNodes(node: MarkdownNode, rewrite: (value: string) => MarkdownNode[] | null): void {
  if (!node.children || node.type === 'link' || node.type === 'linkReference') {
    return
  }
  for (let i = 0; i < node.children.length; i++) {
    const child = node.children[i]
    if (child.type === 'text' && typeof child.value === 'string') {
      const replacement = rewrite(child.value)
      if (replacement) {
        node.children.splice(i, 1, ...replacement)
        i += replacement.length - 1
      }
      continue
    }
    rewriteTextNodes(child, rewrite)
  }
}

function lineBreakTextNodes(value: string): MarkdownNode[] | null {
  const [first, ...rest] = value.split(/\r?\n|\r/)
  if (!rest.length) {
    return null
  }
  return [{ type: 'text', value: first }, ...rest.flatMap((line) => [{ type: 'break' }, { type: 'text', value: line }])]
}

function fileReferenceTextNodes(value: string): MarkdownNode[] | null {
  const matches = findFileReferences(value)
  if (!matches.length) return null
  const nodes: MarkdownNode[] = []
  let lastIndex = 0
  for (const { raw, index } of matches) {
    if (index > lastIndex) nodes.push({ type: 'text', value: value.slice(lastIndex, index) })
    nodes.push({
      type: 'link',
      url: raw,
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

// Wide tables can't shrink below their column min-widths; left bare they force
// the prose column wider than the viewport. A scroll wrapper (min-content 0)
// keeps the table inside its own horizontal scroll without stretching the chat,
// on the same card as a code block.
const MarkdownTable: ComponentType<ComponentProps<'table'> & ExtraProps> = ({ node: _node, ...props }) => (
  <div className="prose-card markdown-table">
    <table {...props} />
  </div>
)

const REMARK_PLUGINS = [remarkGfm, [remarkMath, { singleDollarTextMath: false }], remarkMathDelimiters, remarkFileReferences, remarkHeadingAnchors] satisfies Options['remarkPlugins']
const CHAT_REMARK_PLUGINS = [...REMARK_PLUGINS, remarkConnectCards]
const USER_REMARK_PLUGINS = [...CHAT_REMARK_PLUGINS, remarkLineBreaks]

const MarkdownFileImage: NonNullable<Components['img']> = ({ src, ...props }) => {
  const files = useFileLinks()
  const imageId = useId()
  return <MarkdownImage {...props} src={markdownImageSource(src ?? '', imageId, files?.sessionId, files?.documentPath, files?.cloudRunId)} />
}

function BaseMarkdown({
  text,
  className,
  Link,
  mentions,
  remarkPlugins = REMARK_PLUGINS,
  documentCards = false,
}: {
  text: string
  className: string
  Link: AnchorComponent
  mentions?: ReadonlyMap<string, string>
  remarkPlugins?: NonNullable<Options['remarkPlugins']>
  documentCards?: boolean
}) {
  const files = useFileLinks()
  const plugins = useMemo<NonNullable<Options['remarkPlugins']>>(() => {
    const plugins = [...remarkPlugins]
    if (documentCards && files) plugins.unshift([remarkMarkdownDocuments, { documentPath: files.documentPath }])
    if (mentions?.size) plugins.push([remarkMentions, mentions])
    return plugins
  }, [documentCards, files, mentions, remarkPlugins])
  const components = useMemo<Components>(() => ({ a: Link, img: MarkdownFileImage, pre: CodeBlock, table: MarkdownTable, 'connect-card': ConnectCard, 'markdown-document': MarkdownDocument }), [Link])
  return (
    <div className={className} data-markdown-root>
      <Markdown
        remarkPlugins={plugins}
        rehypePlugins={[[rehypeKatex, { trust: false }]]}
        components={components}
        urlTransform={(url, key, node) => key === 'src' && node.tagName === 'img'
          ? url
          : (files ? resolveFileLink(url, files.documentPath) : parseFileReference(url)) || botIdFromTarget(url) ? url : defaultUrlTransform(url)}
      >
        {text}
      </Markdown>
    </div>
  )
}

function MarkdownDocument({ path, label }: { path: string; label: string }) {
  const files = useFileLinks()!
  const openFile = files.onOpen
  return <MarkdownDocumentCard sessionId={files.sessionId} cloudRunId={files.cloudRunId} file={{ path }} label={label} onOpen={openFile}
    renderContent={(content, documentPath) => (
      <FileReaderLinkProvider sessionId={files.sessionId} cloudRunId={files.cloudRunId} documentPath={documentPath} onOpen={openFile}>
        <RenderedMarkdown text={content} className="file-prose" artifactPreview />
      </FileReaderLinkProvider>
    )} />
}

const MessageMarkdownLink: AnchorComponent = ({ children, href, ...props }) => {
  const label = textFromChildren(children)
  const sigil = mentionSigil(label)
  if (sigil && typeof href === 'string' && href !== '') {
    return <MentionPill mention={{ sigil, name: label.slice(1), target: decoded(href, decodeURI) }} />
  }
  return <PlainMarkdownLink {...props} href={href}>{children}</PlainMarkdownLink>
}

const PlainMarkdownLink: AnchorComponent = ({ node: _node, children, href, ...props }) => {
  const openPreview = usePreviewLink()
  const files = useFileLinks()
  const openFile = files?.onOpen
  const target = markdownLinkTarget(href, textFromChildren(children), files)
  const linkedChildren = <MarkdownImageLinkContext value={true}>{children}</MarkdownImageLinkContext>
  switch (target?.kind) {
    case 'canvas':
      return <Link {...props} className="chat-prose-link" to="/sessions/$sessionId" params={{ sessionId: target.sessionId }} search={{ page: target.page, item: target.item }} state={() => ({ canvasNavigation: crypto.randomUUID() })}>{children}</Link>
    case 'anchor':
      return <a {...props} href={href} className="chat-prose-link" onClick={(event) => {
        if (!isPlainClick(event)) return
        event.preventDefault()
        const root = event.currentTarget.closest('[data-markdown-root]')
        const heading = [...(root?.querySelectorAll<HTMLElement>('[id]') ?? [])].find((node) => node.id === target.id)
        heading?.scrollIntoView({ block: 'start' })
      }}>{children}</a>
    case 'file':
      // An inline link role rather than a button, so the text around it flows on.
      return (
        <span
          role="link"
          tabIndex={0}
          className="chat-prose-link"
          onClick={(event) => {
            if (isPlainClick(event)) openFile?.(target.file)
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter') openFile?.(target.file)
          }}
        >
          <FileText
            aria-hidden="true"
            className="chat-prose-link-icon"
            size={13}
            strokeWidth={1.7}
          />
          {linkedChildren}
        </span>
      )
    case 'url':
      return (
        <a
          {...props}
          className="chat-prose-link"
          href={target.url}
          target="_blank"
          rel="noreferrer"
          onClick={(event) => {
            if (openPreview && isPlainClick(event)) {
              event.preventDefault()
              openPreview(target.url)
            }
          }}
        >
          <Favicon url={target.url} className="chat-prose-link-icon" />
          {linkedChildren}
        </a>
      )
    default:
      return <>{children}</>
  }
}

export const RenderedMarkdown = memo(function RenderedMarkdown({
  text,
  className = 'chat-prose',
  artifactPreview = false,
}: {
  text: string
  className?: string
  artifactPreview?: boolean
}) {
  const files = useFileLinks()
  const artifacts = useContext(MarkdownArtifactContext)
  const scope = useMemo(() => ({
    mode: files?.documentPath && isMarkdownPath(files.documentPath) ? artifactPreview ? 'preview' as const : 'live' as const : 'source' as const,
    onSendPrompt: artifacts.onSendPrompt,
  }), [artifactPreview, artifacts.onSendPrompt, files?.documentPath])
  return <MarkdownArtifactContext value={scope}><BaseMarkdown text={text} className={className} Link={PlainMarkdownLink} /></MarkdownArtifactContext>
})

// User messages already carry mentions as Markdown links, so they use the same
// chat renderer without the assistant-only expansion of bare skill names. Typed
// line breaks are kept as <br>, since users press Enter to start a new line.
export const UserMessageMarkdown = memo(function UserMessageMarkdown({
  text,
  mentions,
}: {
  text: string
  mentions?: ReadonlyMap<string, string>
}) {
  return <BaseMarkdown text={text} mentions={mentions} className="chat-prose" Link={MessageMarkdownLink} remarkPlugins={USER_REMARK_PLUGINS} documentCards />
})

// Shared renderer for assistant prose: GitHub-flavored Markdown + LaTeX via KaTeX.
// Memoized: the remark/rehype pipeline is the priciest per-item work in a
// transcript, so it must only run when the text actually changes.
export const MessageMarkdown = memo(function MessageMarkdown({ text }: { text: string }) {
  const skills = useQuery(skillsQuery())
  const prepared = useMemo(() => linkifyKnownSkills(text, skills.data ?? []), [text, skills.data])
  return <BaseMarkdown text={prepared} className="chat-prose" Link={MessageMarkdownLink} remarkPlugins={CHAT_REMARK_PLUGINS} documentCards />
})

const TEXT_REMARK_PLUGINS = [remarkGfm]

// Markdown read as plain text for one-line previews: every element is
// unwrapped to its text, so `**Deal**` reads Deal and a mention reads @Name.
export const MarkdownText = memo(function MarkdownText({ text }: { text: string }) {
  return <Markdown remarkPlugins={TEXT_REMARK_PLUGINS} allowedElements={[]} unwrapDisallowed>{text}</Markdown>
})
