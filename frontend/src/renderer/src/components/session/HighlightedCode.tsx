import { type CSSProperties, useEffect, useState } from 'react'
import { highlightLines, syntaxTheme, type SyntaxLine, type SyntaxToken } from '@/lib/code/syntaxHighlight'
import { useTheme } from '@/lib/theme'

export type HighlightedCodeLines = SyntaxLine[]
export type HighlightedCodeTokens = SyntaxLine

export function useSyntaxHighlightedLines(path: string, lines: string[]) {
  const { resolved } = useTheme()
  const [highlighted, setHighlighted] = useState<HighlightedCodeLines | null>(null)

  useEffect(() => {
    let cancelled = false
    setHighlighted(null)
    if (!path || !lines.length) return
    void highlightLines(path, lines, syntaxTheme(resolved))
      .then((next) => {
        if (!cancelled) setHighlighted(next)
      })
      .catch(() => {
        if (!cancelled) setHighlighted(null)
      })
    return () => {
      cancelled = true
    }
  }, [lines, path, resolved])

  return highlighted
}

export function HighlightedCodeLine({ text, tokens }: { text: string; tokens?: HighlightedCodeTokens | null }) {
  if (!tokens?.length) return <>{text || ' '}</>
  return (
    <>
      {tokens.map((token, index) => (
        <span key={index} style={tokenStyle(token)}>
          {token.content}
        </span>
      ))}
    </>
  )
}

function tokenStyle(token: SyntaxToken): CSSProperties {
  const fontStyle = token.fontStyle ?? 0
  return {
    color: token.color,
    fontStyle: fontStyle & 1 ? 'italic' : undefined,
    fontWeight: fontStyle & 2 ? 600 : undefined,
    textDecorationLine: fontStyle & 4 ? 'underline' : undefined,
  }
}
