import type { Plugin } from 'unified'
import type { Node } from 'unist'

export const remarkMathDelimiters: Plugin = function () {
  const parse = this.parser!
  this.parser = (text, file) => {
    const tree = parse(text, file)
    if (!text.includes('\\(') && !text.includes('\\[')) return tree
    const code: { start: number; end: number }[] = []
    const visit = (node: Node & { children?: Node[] }) => {
      const start = node.position?.start.offset
      const end = node.position?.end.offset
      if ((node.type === 'code' || node.type === 'inlineCode') && start !== undefined && end !== undefined) {
        code.push({ start, end })
      }
      node.children?.forEach(visit)
    }
    visit(tree)
    let normalized = ''
    let cursor = 0
    for (const { start, end } of code) {
      normalized += normalizeMath(text.slice(cursor, start)) + text.slice(start, end)
      cursor = end
    }
    normalized += normalizeMath(text.slice(cursor))
    if (normalized === text) return tree
    file.value = normalized
    return parse(normalized, file)
  }
}

function normalizeMath(text: string): string {
  return text
    .replace(/\\\[([\s\S]*?)\\\]/g, (_, expr: string) => `\n$$\n${expr}\n$$\n`)
    .replace(/\\\(([\s\S]*?)\\\)/g, (_, expr: string) => `$$${expr}$$`)
}
