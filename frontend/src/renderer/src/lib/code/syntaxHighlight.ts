import { createBundledHighlighter, type HighlighterGeneric } from '@shikijs/core'
import { createJavaScriptRegexEngine } from '@shikijs/engine-javascript'

export interface SyntaxToken {
  content: string
  color?: string
  fontStyle?: number
}

export type SyntaxLine = SyntaxToken[]

type CodeTheme = 'github-light' | 'github-dark'
type CodeLanguage =
  | 'astro'
  | 'bash'
  | 'c'
  | 'cpp'
  | 'csharp'
  | 'css'
  | 'diff'
  | 'dockerfile'
  | 'go'
  | 'graphql'
  | 'html'
  | 'java'
  | 'js'
  | 'json'
  | 'jsonc'
  | 'jsx'
  | 'kotlin'
  | 'less'
  | 'lua'
  | 'makefile'
  | 'md'
  | 'mdx'
  | 'php'
  | 'proto'
  | 'python'
  | 'ruby'
  | 'rust'
  | 'scss'
  | 'sql'
  | 'svelte'
  | 'swift'
  | 'toml'
  | 'ts'
  | 'tsx'
  | 'vue'
  | 'xml'
  | 'yaml'
  | 'zig'
  | 'zsh'

const LIGHT_THEME: CodeTheme = 'github-light'
const DARK_THEME: CodeTheme = 'github-dark'
const MAX_HIGHLIGHT_LINES = 2500
const MAX_HIGHLIGHT_CHARS = 120 * 1024

const LANGS = {
  astro: () => import('@shikijs/langs/astro'),
  bash: () => import('@shikijs/langs/bash'),
  c: () => import('@shikijs/langs/c'),
  cpp: () => import('@shikijs/langs/cpp'),
  csharp: () => import('@shikijs/langs/csharp'),
  css: () => import('@shikijs/langs/css'),
  diff: () => import('@shikijs/langs/diff'),
  dockerfile: () => import('@shikijs/langs/dockerfile'),
  go: () => import('@shikijs/langs/go'),
  graphql: () => import('@shikijs/langs/graphql'),
  html: () => import('@shikijs/langs/html'),
  java: () => import('@shikijs/langs/java'),
  js: () => import('@shikijs/langs/js'),
  json: () => import('@shikijs/langs/json'),
  jsonc: () => import('@shikijs/langs/jsonc'),
  jsx: () => import('@shikijs/langs/jsx'),
  kotlin: () => import('@shikijs/langs/kotlin'),
  less: () => import('@shikijs/langs/less'),
  lua: () => import('@shikijs/langs/lua'),
  makefile: () => import('@shikijs/langs/makefile'),
  md: () => import('@shikijs/langs/md'),
  mdx: () => import('@shikijs/langs/mdx'),
  php: () => import('@shikijs/langs/php'),
  proto: () => import('@shikijs/langs/proto'),
  python: () => import('@shikijs/langs/python'),
  ruby: () => import('@shikijs/langs/ruby'),
  rust: () => import('@shikijs/langs/rust'),
  scss: () => import('@shikijs/langs/scss'),
  sql: () => import('@shikijs/langs/sql'),
  svelte: () => import('@shikijs/langs/svelte'),
  swift: () => import('@shikijs/langs/swift'),
  toml: () => import('@shikijs/langs/toml'),
  ts: () => import('@shikijs/langs/ts'),
  tsx: () => import('@shikijs/langs/tsx'),
  vue: () => import('@shikijs/langs/vue'),
  xml: () => import('@shikijs/langs/xml'),
  yaml: () => import('@shikijs/langs/yaml'),
  zig: () => import('@shikijs/langs/zig'),
  zsh: () => import('@shikijs/langs/zsh'),
} satisfies Record<CodeLanguage, unknown>

const createHighlighter = createBundledHighlighter<CodeLanguage, CodeTheme>({
  langs: LANGS,
  themes: {
    'github-dark': () => import('@shikijs/themes/github-dark'),
    'github-light': () => import('@shikijs/themes/github-light'),
  },
  engine: () => createJavaScriptRegexEngine(),
})

let highlighter: Promise<HighlighterGeneric<CodeLanguage, CodeTheme>> | null = null

const EXTENSIONS: Record<string, CodeLanguage> = {
  astro: 'astro',
  bash: 'bash',
  c: 'c',
  cc: 'cpp',
  cjs: 'js',
  cpp: 'cpp',
  cs: 'csharp',
  css: 'css',
  cts: 'ts',
  dockerfile: 'dockerfile',
  go: 'go',
  gql: 'graphql',
  graphql: 'graphql',
  h: 'c',
  hpp: 'cpp',
  html: 'html',
  java: 'java',
  js: 'js',
  json: 'json',
  jsonc: 'jsonc',
  jsx: 'jsx',
  kt: 'kotlin',
  less: 'less',
  lua: 'lua',
  md: 'md',
  mdx: 'mdx',
  mjs: 'js',
  mts: 'ts',
  php: 'php',
  proto: 'proto',
  py: 'python',
  rb: 'ruby',
  rs: 'rust',
  scss: 'scss',
  sh: 'bash',
  sql: 'sql',
  svelte: 'svelte',
  swift: 'swift',
  toml: 'toml',
  ts: 'ts',
  tsx: 'tsx',
  vue: 'vue',
  xml: 'xml',
  yaml: 'yaml',
  yml: 'yaml',
  zig: 'zig',
  zsh: 'zsh',
}

const FILENAMES: Record<string, CodeLanguage> = {
  dockerfile: 'dockerfile',
  makefile: 'makefile',
}

const SUPPORTED_LANGUAGES = new Set(Object.keys(LANGS) as CodeLanguage[])

// Markdown fence labels (```javascript) use full names that are neither Shiki
// language ids nor file extensions; map those onto a supported language. Plain
// extension-style labels (sh, yml, py…) already resolve via EXTENSIONS below.
const LANGUAGE_ALIASES: Record<string, CodeLanguage> = {
  javascript: 'js',
  node: 'js',
  typescript: 'ts',
  shell: 'bash',
  console: 'bash',
  golang: 'go',
  patch: 'diff',
  markdown: 'md',
  'c++': 'cpp',
  'c#': 'csharp',
  'objective-c': 'c',
}

export function syntaxTheme(resolvedTheme: 'light' | 'dark'): CodeTheme {
  return resolvedTheme === 'dark' ? DARK_THEME : LIGHT_THEME
}

export function languageForPath(path: string): CodeLanguage | null {
  const filename = path.split('/').pop()?.toLowerCase() ?? ''
  if (FILENAMES[filename]) return FILENAMES[filename]
  const extension = filename.includes('.') ? filename.split('.').pop() : ''
  return extension ? (EXTENSIONS[extension] ?? null) : null
}

function languageForHint(hint: string): CodeLanguage | null {
  const key = hint.trim().toLowerCase()
  if (!key) return null
  if (LANGUAGE_ALIASES[key]) return LANGUAGE_ALIASES[key]
  if (SUPPORTED_LANGUAGES.has(key as CodeLanguage)) return key as CodeLanguage
  return EXTENSIONS[key] ?? null
}

export async function highlightLines(
  path: string,
  lines: string[],
  theme: CodeTheme,
): Promise<SyntaxLine[] | null> {
  const lang = languageForPath(path)
  if (!lang || !withinHighlightBudget(lines)) return null
  return highlightWithLanguage(lang, lines.join('\n'), theme)
}

export async function highlightCode(
  hint: string,
  code: string,
  theme: CodeTheme,
): Promise<SyntaxLine[] | null> {
  const lang = languageForHint(hint)
  if (!lang || !withinHighlightBudget(code.split('\n'))) return null
  return highlightWithLanguage(lang, code, theme)
}

async function highlightWithLanguage(
  lang: CodeLanguage,
  code: string,
  theme: CodeTheme,
): Promise<SyntaxLine[]> {
  highlighter ??= createHighlighter({ themes: [LIGHT_THEME, DARK_THEME], langs: [] })
  const shiki = await highlighter
  await shiki.loadLanguage(lang)
  const result = shiki.codeToTokens(code, { lang, theme })
  return result.tokens.map((line) =>
    line.map((token) => ({
      content: token.content,
      color: token.color,
      fontStyle: token.fontStyle,
    })),
  )
}

function withinHighlightBudget(lines: string[]): boolean {
  if (lines.length > MAX_HIGHLIGHT_LINES) return false
  let chars = 0
  for (const line of lines) {
    chars += line.length + 1
    if (chars > MAX_HIGHLIGHT_CHARS) return false
  }
  return true
}
