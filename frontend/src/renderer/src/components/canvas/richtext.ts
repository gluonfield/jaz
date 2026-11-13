// The part of Markdown a board item can be typed in as formatted words:
// paragraphs, bulleted and numbered lists, bold, italic, strikethrough, code
// and links. Text using anything else is edited as Markdown source.

export const MARKS = ['bold', 'italic', 'strike', 'code'] as const
export type Mark = (typeof MARKS)[number]

// A run whose text is a newline is a line break.
export type Run = { text: string; href?: string } & Partial<Record<Mark, true>>
export interface Block {
  kind: 'p' | 'ul' | 'ol'
  depth: number
  runs: Run[]
}
export type TextFormat = Mark | 'list' | 'numbered'

const ALNUM = /[\p{L}\p{N}]/u
const PUNCTUATION = /[!-/:-@[-`{-~]/
const RULE = /^ {0,3}(?:[-*_]\s*){3,}$/
const NOT_RICH = /^ {0,3}(#{1,6}(\s|$)|>|```|~~~|\||<|=+\s*$)|^(\t| {4})|^\s*\|?\s*:?-{3,}:?\s*\|/
const ITEM = /^( *)([-*+]|\d+[.)])[ \t]+(.*)$/
const LINK = /^\[((?:[^[\]\\]|\\.)*)\]\(([^()\s]*)\)/

function same(a: Run, b: Run): boolean {
  return a.href === b.href && MARKS.every((mark) => a[mark] === b[mark])
}

// Adjacent runs that look alike become one, and a block loses the space and
// breaks around its words.
function tidy(blocks: Block[]): Block[] {
  return blocks.flatMap((block) => {
    const runs: Run[] = []
    for (const run of block.runs) {
      const last = runs.at(-1)
      if (!run.text) continue
      if (last && last.text !== '\n' && run.text !== '\n' && same(last, run)) last.text += run.text
      else runs.push({ ...run })
    }
    while (runs.length && !runs[0].text.trimStart()) runs.shift()
    while (runs.length && !runs.at(-1)!.text.trimEnd()) runs.pop()
    if (!runs.length) return []
    runs[0].text = runs[0].text.trimStart()
    runs.at(-1)!.text = runs.at(-1)!.text.trimEnd()
    return [{ ...block, runs }]
  })
}

function inline(source: string, href?: string): Run[] | null {
  const runs: Run[] = []
  const open: Partial<Record<Mark, string>> = {}
  let text = ''
  const marks = () => ({ ...(href ? { href } : {}), ...Object.fromEntries(MARKS.filter((mark) => open[mark]).map((mark) => [mark, true])) })
  const flush = () => {
    if (text) runs.push({ text, ...marks() })
    text = ''
  }
  // A delimiter opens before a word and closes after one; elsewhere it is itself.
  const toggle = (mark: Mark, delimiter: string, at: number, word: boolean): boolean => {
    const before = source[at - 1] ?? ' '
    const after = source[at + delimiter.length] ?? ' '
    const closes = open[mark] === delimiter && !/\s/.test(before) && !(word && ALNUM.test(after))
    const opens = !open[mark] && !/\s/.test(after) && !(word && ALNUM.test(before))
    if (!closes && !opens) return false
    flush()
    if (closes) delete open[mark]
    else open[mark] = delimiter
    return true
  }
  for (let i = 0; i < source.length;) {
    const rest = source.slice(i)
    const char = source[i]
    if (char === '\\') {
      const next = source[i + 1] ?? ''
      if ('()[]'.includes(next)) return null
      if (PUNCTUATION.test(next)) {
        text += next
        i += 2
        continue
      }
    }
    if (char === '`') {
      const fence = /^`+/.exec(rest)![0]
      const end = source.indexOf(fence, i + fence.length)
      if (end < 0) return null
      flush()
      runs.push({ text: source.slice(i + fence.length, end).replace(/^ (.*) $/, '$1'), ...marks(), code: true })
      i = end + fence.length
      continue
    }
    if (char === '[' && !href) {
      const link = LINK.exec(rest)
      if (link) {
        const inner = inline(link[1], link[2])
        if (!inner) return null
        flush()
        runs.push(...inner.map((run) => ({ ...run, ...marks(), href: link[2] })))
        i += link[0].length
        continue
      }
    }
    if (rest.startsWith('![') || /^<[a-zA-Z/!]/.test(rest) || rest.startsWith('$$') || /^&#?\w+;/.test(rest)) return null
    const delimiter = (['**', '__', '~~', '*', '_'] as const).find((mark) => rest.startsWith(mark))
    if (delimiter) {
      const mark = delimiter === '~~' ? 'strike' : delimiter.length === 2 ? 'bold' : 'italic'
      if (toggle(mark, delimiter, i, delimiter.includes('_'))) {
        i += delimiter.length
        continue
      }
    }
    text += char
    i++
  }
  flush()
  return MARKS.some((mark) => open[mark]) ? null : runs
}

export function parseRich(source: string): Block[] | null {
  const blocks: Block[] = []
  let paragraph: Block | null = null
  let broke = false
  let blank = true
  for (const raw of source.split('\n')) {
    if (!raw.trim()) {
      paragraph = null
      blank = true
      continue
    }
    if (RULE.test(raw)) return null
    const item = ITEM.exec(raw)
    if (item) {
      const runs = /^\[[ xX]\]\s/.test(item[3]) ? null : inline(item[3])
      if (!runs) return null
      blocks.push({ kind: /\d/.test(item[2]) ? 'ol' : 'ul', depth: Math.floor(item[1].length / 2), runs })
      paragraph = null
      blank = false
      continue
    }
    // A line right under a list item would continue that item.
    if (NOT_RICH.test(raw) || (!paragraph && !blank)) return null
    const hard = /( {2,}|\\)$/.exec(raw)
    const runs = inline((hard ? raw.slice(0, hard.index) : raw).trim())
    if (!runs) return null
    if (paragraph) paragraph.runs.push({ text: broke ? '\n' : ' ' }, ...runs)
    else blocks.push(paragraph = { kind: 'p', depth: 0, runs })
    broke = Boolean(hard)
    blank = false
  }
  return tidy(blocks)
}

function escape(text: string, start: boolean): string {
  const safe = text
    .replace(/[\\*`]/g, '\\$&')
    .replace(/~~/g, '\\~~')
    .replace(/_/g, (mark, at: number, whole: string) => (ALNUM.test(whole[at - 1] ?? '') && ALNUM.test(whole[at + 1] ?? '') ? mark : '\\_'))
    .replace(/\](?=\()/g, '&#93;')
    .replace(/<(?=[a-zA-Z/!])/g, '\\<')
    .replace(/&(?=#?\w+;)/g, '\\&')
    .replace(/!(?=\[)/g, '\\!')
  return start ? safe.replace(/^(#{1,6}(?=\s|$)|>|\||[-+](?=\s)|(\d+)([.)])(?=\s))/, (lead, _all, digits, dot) => (digits ? `${digits}\\${dot}` : `\\${lead}`)) : safe
}

function writeRun(run: Run, start: boolean): string {
  if (run.text === '\n') return '\\\n'
  const [, lead, core, trail] = /^(\s*)([\s\S]*?)(\s*)$/.exec(run.text)!
  if (!core) return run.text
  const fence = core.includes('`') ? '``' : '`'
  const pad = core.startsWith('`') || core.endsWith('`') ? ' ' : ''
  let words = run.code ? fence + pad + core + pad + fence : escape(core, start && !lead)
  if (run.italic) words = `*${words}*`
  if (run.bold) words = `**${words}**`
  if (run.strike) words = `~~${words}~~`
  return lead + words + trail
}

function writeRuns(runs: Run[]): string {
  let out = ''
  for (let i = 0; i < runs.length;) {
    const start = i === 0 || runs[i - 1].text === '\n'
    const href = runs[i].href
    if (!href) {
      out += writeRun(runs[i++], start)
      continue
    }
    let label = ''
    for (; i < runs.length && runs[i].href === href; i++) label += writeRun(runs[i], false)
    out += `[${label}](${href})`
  }
  return out
}

export function writeRich(blocks: Block[]): string {
  const counts: number[] = []
  let out = ''
  tidy(blocks).forEach((block, index, all) => {
    const previous = all[index - 1]
    if (previous) out += block.kind === 'p' || previous.kind === 'p' || (block.depth === 0 && previous.kind !== block.kind) ? '\n\n' : '\n'
    if (block.kind === 'p') {
      counts.length = 0
      out += writeRuns(block.runs)
      return
    }
    counts.length = block.depth + 1
    counts[block.depth] = block.kind === 'ol' ? (counts[block.depth] ?? 0) + 1 : 0
    const line = block.runs.map((run) => (run.text === '\n' ? { text: ' ' } : run))
    out += `${'  '.repeat(block.depth)}${block.kind === 'ol' ? `${counts[block.depth]}.` : '-'} ${writeRuns(line)}`
  })
  return out
}

const TAGS: Record<Mark, string> = { code: 'code', italic: 'em', bold: 'strong', strike: 's' }

function text(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

function runsHTML(runs: Run[]): string {
  return runs.map((run) => {
    if (run.text === '\n') return '<br>'
    const marked = MARKS.filter((mark) => run[mark]).reduce((inner, mark) => `<${TAGS[mark]}>${inner}</${TAGS[mark]}>`, text(run.text))
    return run.href ? `<a href="${text(run.href)}">${marked}</a>` : marked
  }).join('')
}

// The blocks as the HTML an editable element starts from.
export function richHTML(blocks: Block[]): string {
  const lists: string[] = []
  let out = ''
  const close = (to: number) => {
    while (lists.length > to) out += `</li></${lists.pop()}>`
  }
  for (const block of blocks) {
    if (block.kind === 'p') {
      close(0)
      out += `<p>${runsHTML(block.runs)}</p>`
      continue
    }
    const depth = Math.min(block.depth, lists.length)
    close(depth + 1)
    if (lists.length === depth + 1 && lists[depth] !== block.kind) close(depth)
    if (lists.length === depth + 1) out += '</li><li>'
    else {
      out += `<${block.kind}><li>`
      lists.push(block.kind)
    }
    out += runsHTML(block.runs)
  }
  close(0)
  return out || '<p><br></p>'
}

const BLOCK_TAGS = ['P', 'DIV', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'BLOCKQUOTE', 'PRE']
const MARK_TAGS: Record<string, Mark> = { B: 'bold', STRONG: 'bold', I: 'italic', EM: 'italic', S: 'strike', STRIKE: 'strike', DEL: 'strike', CODE: 'code' }

function isList(node: Node): node is HTMLElement {
  return node.nodeName === 'UL' || node.nodeName === 'OL'
}

function readInline(node: Node, marks: Omit<Run, 'text'>, out: Run[]) {
  if (node.nodeType === Node.TEXT_NODE) {
    const lines = node.textContent!.replace(/\u00a0/g, ' ').split('\n')
    lines.forEach((line, index) => out.push(...(index ? [{ text: '\n' }] : []), { text: line, ...marks }))
    return
  }
  if (!(node instanceof HTMLElement)) return
  if (node.tagName === 'BR') {
    out.push({ text: '\n' })
    return
  }
  const next = { ...marks }
  const mark = MARK_TAGS[node.tagName]
  if (mark) next[mark] = true
  if (/^(bold|[6-9]00)$/.test(node.style.fontWeight)) next.bold = true
  if (node.style.fontStyle === 'italic') next.italic = true
  if (node.style.textDecorationLine.includes('line-through')) next.strike = true
  const href = node.tagName === 'A' ? node.getAttribute('href') : null
  if (href) next.href = href
  node.childNodes.forEach((child) => readInline(child, next, out))
}

// What the user left in an editable element, as blocks.
export function readRich(root: Node): Block[] {
  const blocks: Block[] = []
  let loose: Block | null = null
  const readList = (list: HTMLElement, depth: number) => {
    for (const child of list.childNodes) {
      if (isList(child)) readList(child, depth + 1)
      if (child.nodeName !== 'LI') continue
      const block: Block = { kind: list.tagName === 'OL' ? 'ol' : 'ul', depth, runs: [] }
      blocks.push(block)
      child.childNodes.forEach((node) => (isList(node) ? readList(node, depth + 1) : readInline(node, {}, block.runs)))
    }
  }
  for (const node of root.childNodes) {
    if (isList(node)) {
      loose = null
      readList(node, 0)
    } else if (BLOCK_TAGS.includes(node.nodeName)) {
      loose = null
      const block: Block = { kind: 'p', depth: 0, runs: [] }
      blocks.push(block)
      readInline(node, {}, block.runs)
    } else {
      if (!loose) blocks.push(loose = { kind: 'p', depth: 0, runs: [] })
      readInline(node, {}, loose.runs)
    }
  }
  return tidy(blocks)
}

// Every word gets the mark, or loses it when every word already has it.
export function toggleRichMark(blocks: Block[], mark: Mark): Block[] {
  const words = blocks.flatMap((block) => block.runs).filter((run) => run.text.trim())
  const on = !words.every((run) => run[mark])
  return blocks.map((block) => ({
    ...block,
    runs: block.runs.map((run) => {
      const next = { ...run }
      delete next[mark]
      return on && run.text.trim() ? { ...next, [mark]: true } : next
    }),
  }))
}

// Every block becomes an item of that list, or a paragraph when all are.
export function toggleRichList(blocks: Block[], kind: 'ul' | 'ol'): Block[] {
  const all = blocks.every((block) => block.kind === kind)
  return blocks.map((block) => ({ ...block, kind: all ? 'p' : kind, depth: all ? 0 : block.depth }))
}
