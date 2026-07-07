declare module '*.css'
declare module '@fontsource-variable/inter'
declare module '@fontsource-variable/jetbrains-mono'

interface ImportMetaEnv {
  readonly DEV: boolean
  readonly VITE_JAZ_API_URL?: string
  readonly VITE_POSTHOG_TOKEN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare namespace JSX {
  interface IntrinsicElements {
    webview: React.DetailedHTMLProps<
      React.HTMLAttributes<HTMLElement> & {
        allowpopups?: boolean
        partition?: string
        src?: string
      },
      HTMLElement
    >
  }
}
