import { useQuery } from '@tanstack/react-query'
import { FileText } from 'lucide-react'
import { useMemo } from 'react'
import Markdown from 'react-markdown'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import { skillsQuery, type SkillInfo } from '@/lib/api/skills'
import { encodeMention, MentionPill } from './mentions'

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

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// Escape the $ of [$skill](path) links so remark-math can't pair the dollar
// signs of two mentions in one paragraph into a formula (the backslash is
// consumed by markdown, so the rendered label is still `$name`).
function escapeMentionDollars(text: string): string {
  return text.replace(/\[\$(?=[^\]\n]*\]\()/g, '[\\$')
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
        // The lookbehind skips $names already inside a link ('[' or the '\'
        // escapeMentionDollars adds) and mid-word matches.
        const pattern = new RegExp(`(?<![\\w[\\\\-])\\$${escapeRegExp(skill.name)}(?![\\w-])`, 'g')
        out = out.replace(pattern, () =>
          encodeMention('$', skill.name, skill.path).replace('[$', '[\\$'),
        )
      }
      return out
    })
    .join('')
}

function mentionSigil(label: string): '$' | '@' | null {
  return label.startsWith('$') || label.startsWith('@') ? (label[0] as '$' | '@') : null
}

// Shared renderer for assistant prose: GitHub-flavored markdown + LaTeX
// ($...$ / $$...$$ via KaTeX). Styles live in globals.css under .chat-prose.
export function MessageMarkdown({ text }: { text: string }) {
  // Cached by the composer; lets assistant echoes of $skill-name render as
  // mention pills. An empty catalog simply skips the pass.
  const skills = useQuery(skillsQuery)
  const prepared = useMemo(
    () => normalizeMath(linkifyKnownSkills(escapeMentionDollars(text), skills.data ?? [])),
    [text, skills.data],
  )
  return (
    <div className="chat-prose">
      <Markdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex]}
        components={{
          // External links open in the system browser (main process denies
          // window.open and calls shell.openExternal).
          a: ({ node: _node, children, href, ...props }) => {
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
        {prepared}
      </Markdown>
    </div>
  )
}

// The markdown pipeline percent-encodes hrefs; show the filesystem path.
function decodeMentionHref(href: string): string {
  try {
    return decodeURI(href)
  } catch {
    return href
  }
}
