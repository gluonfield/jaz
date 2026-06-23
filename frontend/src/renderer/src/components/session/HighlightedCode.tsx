import { type CSSProperties, useEffect, useState } from 'react'
import { highlightCode, highlightLines, syntaxTheme, type SyntaxLine, type SyntaxToken } from '@/lib/code/syntaxHighlight'
import { useTheme } from '@/lib/theme'

export type HighlightedCodeLines = SyntaxLine[]
export type HighlightedCodeTokens = SyntaxLine

// Pipe an in-flight highlight into `setHighlighted`, ignoring it if the effect
// is torn down first. Returns the effect cleanup.
function applyHighlight(
  pending: Promise<HighlightedCodeLines | null>,
  setHighlighted: (lines: HighlightedCodeLines | null) => void,
): () => void {
  let cancelled = false
  void pending
    .then((next) => {
      if (!cancelled) setHighlighted(next)
    })
    .catch(() => {
      if (!cancelled) setHighlighted(null)
    })
  return () => {
    cancelled = true
  }
}

export function useSyntaxHighlightedLines(path: string, lines: string[]) {
  const { resolved } = useTheme()
  const [highlighted, setHighlighted] = useState<HighlightedCodeLines | null>(null)

  useEffect(() => {
    setHighlighted(null)
    if (!path || !lines.length) return
    return applyHighlight(highlightLines(path, lines, syntaxTheme(resolved)), setHighlighted)
  }, [lines, path, resolved])

  return highlighted
}

// Like useSyntaxHighlightedLines but keyed on a markdown fence language hint.
// Unlike its sibling it keeps the previous tokens while re-highlighting so a
// streaming code block doesn't flash back to unstyled text on every delta.
export function useHighlightedCode(language: string, code: string) {
  const { resolved } = useTheme()
  const [highlighted, setHighlighted] = useState<HighlightedCodeLines | null>(null)

  useEffect(() => {
    if (!language || !code) {
      setHighlighted(null)
      return
    }
    return applyHighlight(highlightCode(language, code, syntaxTheme(resolved)), setHighlighted)
  }, [language, code, resolved])

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
