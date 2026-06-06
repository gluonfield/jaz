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
          a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
        }}
      >
        {normalizeMath(text)}
      </Markdown>
    </div>
  )
}
