import type { Element, ElementContent } from 'hast'
import { Fragment, type ComponentProps } from 'react'
import type { ExtraProps } from 'react-markdown'
import { CopyToggleIcon } from '@/components/ui/CopyToggleIcon'
import { useCopyAction } from '@/lib/useCopyAction'
import { HighlightedCodeLine, type HighlightedCodeTokens, useHighlightedCode } from './HighlightedCode'

const LANGUAGE_LABELS: Record<string, string> = {
  js: 'JavaScript',
  javascript: 'JavaScript',
  jsx: 'JSX',
  ts: 'TypeScript',
  typescript: 'TypeScript',
  tsx: 'TSX',
  py: 'Python',
  python: 'Python',
  rb: 'Ruby',
  ruby: 'Ruby',
  rs: 'Rust',
  rust: 'Rust',
  go: 'Go',
  golang: 'Go',
  c: 'C',
  cpp: 'C++',
  'c++': 'C++',
  cs: 'C#',
  csharp: 'C#',
  java: 'Java',
  kt: 'Kotlin',
  kotlin: 'Kotlin',
  swift: 'Swift',
  php: 'PHP',
  sh: 'Shell',
  bash: 'Bash',
  zsh: 'Shell',
  shell: 'Shell',
  console: 'Shell',
  json: 'JSON',
  jsonc: 'JSON',
  yaml: 'YAML',
  yml: 'YAML',
  toml: 'TOML',
  xml: 'XML',
  html: 'HTML',
  css: 'CSS',
  scss: 'SCSS',
  less: 'Less',
  sql: 'SQL',
  md: 'Markdown',
  markdown: 'Markdown',
  graphql: 'GraphQL',
  proto: 'Protobuf',
  lua: 'Lua',
  zig: 'Zig',
  vue: 'Vue',
  svelte: 'Svelte',
  astro: 'Astro',
  dockerfile: 'Dockerfile',
  makefile: 'Makefile',
}

function nodeText(node: ElementContent | undefined): string {
  if (!node) return ''
  if (node.type === 'text') return node.value
  if (node.type === 'element') return node.children.map(nodeText).join('')
  return ''
}

function codeChild(node: Element | undefined): Element | undefined {
  return node?.children.find(
    (child): child is Element => child.type === 'element' && child.tagName === 'code',
  )
}

function languageFromClass(className: unknown): string {
  const list = Array.isArray(className) ? className : typeof className === 'string' ? [className] : []
  for (const entry of list) {
    if (typeof entry === 'string' && entry.startsWith('language-')) {
      return entry.slice('language-'.length)
    }
  }
  return ''
}

function displayLanguage(hint: string): string {
  const key = hint.trim().toLowerCase()
  if (!key) return 'Code'
  return LANGUAGE_LABELS[key] ?? key
}

// Highlighting trails the text by a tick while streaming, so reuse a line's
// cached tokens only when they still reconstruct to the current line — the
// growing line falls back to plain text instead of showing stale characters.
function freshTokens(tokens: HighlightedCodeTokens | undefined, line: string): HighlightedCodeTokens | undefined {
  if (!tokens) return undefined
  return tokens.map((token) => token.content).join('') === line ? tokens : undefined
}

// Replaces react-markdown's default <pre> for fenced code blocks: a header with
// the language label and an always-visible copy button, plus a syntax-highlighted
// body. Inline `code` is untouched (it never renders inside a <pre>).
export function CodeBlock({ node, children }: ComponentProps<'pre'> & ExtraProps) {
  const code = codeChild(node)
  const language = languageFromClass(code?.properties?.className)
  const text = nodeText(code).replace(/\n$/, '')
  const highlighted = useHighlightedCode(language, text)

  if (!code) return <pre>{children}</pre>

  const lines = text.split('\n')
  return (
    <div className="my-3 overflow-hidden rounded-card bg-surface ring-1 ring-border/60">
      <div className="flex items-center justify-between gap-2 bg-surface-2/60 py-1 pl-3.5 pr-1.5">
        <span className="select-none text-[11px] font-medium text-ink-3">{displayLanguage(language)}</span>
        <CodeCopyButton text={text} />
      </div>
      <pre className="overflow-x-auto px-3.5 py-3 leading-relaxed">
        <code>
          {lines.map((line, index) => (
            <Fragment key={index}>
              {index > 0 ? '\n' : null}
              <HighlightedCodeLine text={line} tokens={freshTokens(highlighted?.[index], line)} />
            </Fragment>
          ))}
        </code>
      </pre>
    </div>
  )
}

function CodeCopyButton({ text }: { text: string }) {
  const { copied, copy } = useCopyAction(text)
  return (
    <button
      type="button"
      aria-label={copied ? 'Copied code' : 'Copy code'}
      title={copied ? 'Copied' : 'Copy code'}
      onClick={() => void copy()}
      className="group inline-flex h-6 cursor-pointer items-center gap-1 rounded-md px-1.5 text-[11px] font-medium text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface hover:text-ink active:scale-[0.96]"
    >
      <CopyToggleIcon copied={copied} />
      <span>{copied ? 'Copied' : 'Copy'}</span>
    </button>
  )
}
