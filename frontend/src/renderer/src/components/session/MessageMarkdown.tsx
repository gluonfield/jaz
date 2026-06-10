import { FileText } from 'lucide-react'
import Markdown from 'react-markdown'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'

// Models often emit \[...\] / \(...\) math delimiters; remark-math only
// parses $-style. Convert outside of code spans/fences.
function normalizeMath(text: string): string {
  return text
    .split(/(```[\s\S]*?```|`[^`]*`)/g)
    .map((part, i) =>
      i % 2 === 1
        ? part
        : part
            .replace(/\\\[([\s\S]*?)\\\]/g, (_, expr: string) => `$$${expr}$$`)
            .replace(/\\\(([\s\S]*?)\\\)/g, (_, expr: string) => `$${expr}$`),
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

function isAbsoluteLocalPath(value: string): boolean {
  return (value.startsWith('/') && !value.startsWith('//')) || value.startsWith('file:///')
}

function isLocalPathLink(href: unknown, children: unknown): boolean {
  return (
    (typeof href === 'string' && isAbsoluteLocalPath(href)) ||
    isAbsoluteLocalPath(textFromChildren(children).trim())
  )
}

// Shared renderer for assistant prose: GitHub-flavored markdown + LaTeX
// ($...$ / $$...$$ via KaTeX). Styles live in globals.css under .chat-prose.
export function MessageMarkdown({ text }: { text: string }) {
  return (
    <div className="chat-prose">
      <Markdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex]}
        components={{
          // External links open in the system browser (main process denies
          // window.open and calls shell.openExternal).
          a: ({ node: _node, children, href, ...props }) => {
            const localPath = isLocalPathLink(href, children)
            return (
              <a {...props} href={href} target="_blank" rel="noreferrer">
                {localPath ? (
                  <FileText
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
        {normalizeMath(text)}
      </Markdown>
    </div>
  )
}
