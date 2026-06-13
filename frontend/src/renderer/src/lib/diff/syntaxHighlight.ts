import { createBundledHighlighter, type HighlighterGeneric } from '@shikijs/core'
import { createJavaScriptRegexEngine } from '@shikijs/engine-javascript'

export interface SyntaxToken {
  content: string
  color?: string
  fontStyle?: number
}

export type SyntaxLine = SyntaxToken[]

type DiffTheme = 'github-light' | 'github-dark'
type DiffLanguage =
  | 'astro'
  | 'bash'
  | 'c'
  | 'cpp'
  | 'csharp'
  | 'css'
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

const LIGHT_THEME: DiffTheme = 'github-light'
const DARK_THEME: DiffTheme = 'github-dark'
const MAX_HIGHLIGHT_LINES = 2500
const MAX_HIGHLIGHT_CHARS = 120 * 1024

const createHighlighter = createBundledHighlighter<DiffLanguage, DiffTheme>({
  langs: {
    astro: () => import('@shikijs/langs/astro'),
    bash: () => import('@shikijs/langs/bash'),
    c: () => import('@shikijs/langs/c'),
    cpp: () => import('@shikijs/langs/cpp'),
    csharp: () => import('@shikijs/langs/csharp'),
    css: () => import('@shikijs/langs/css'),
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
  },
  themes: {
    'github-dark': () => import('@shikijs/themes/github-dark'),
    'github-light': () => import('@shikijs/themes/github-light'),
  },
  engine: () => createJavaScriptRegexEngine(),
})

let highlighter: Promise<HighlighterGeneric<DiffLanguage, DiffTheme>> | null = null

const EXTENSIONS: Record<string, DiffLanguage> = {
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

const FILENAMES: Record<string, DiffLanguage> = {
  dockerfile: 'dockerfile',
  makefile: 'makefile',
}

export function syntaxTheme(resolvedTheme: 'light' | 'dark'): DiffTheme {
  return resolvedTheme === 'dark' ? DARK_THEME : LIGHT_THEME
}

export function languageForPath(path: string): DiffLanguage | null {
  const filename = path.split('/').pop()?.toLowerCase() ?? ''
  if (FILENAMES[filename]) return FILENAMES[filename]
  const extension = filename.includes('.') ? filename.split('.').pop() : ''
  return extension ? (EXTENSIONS[extension] ?? null) : null
}

export async function highlightLines(
  path: string,
  lines: string[],
  theme: DiffTheme,
): Promise<SyntaxLine[] | null> {
  const lang = languageForPath(path)
  if (!lang) return null
  if (!withinHighlightBudget(lines)) return null
  highlighter ??= createHighlighter({ themes: [LIGHT_THEME, DARK_THEME], langs: [] })
  const shiki = await highlighter
  await shiki.loadLanguage(lang)
  const result = shiki.codeToTokens(lines.join('\n'), { lang, theme })
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
