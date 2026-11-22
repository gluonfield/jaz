declare module '*.css'
declare module '*.svg' {
  const url: string
  export default url
}
declare module '*.png' {
  const url: string
  export default url
}
declare module '*?url' {
  const url: string
  export default url
}
declare module '*?worker' {
  const WorkerConstructor: { new(options?: WorkerOptions): Worker }
  export default WorkerConstructor
}
declare module '@fontsource-variable/inter'
declare module '@fontsource-variable/jetbrains-mono'

interface ImportMetaEnv {
  readonly BASE_URL: string
  readonly DEV: boolean
  readonly VITE_JAZ_API_URL?: string
  readonly VITE_JAZ_CLOUD_URL?: string
  readonly VITE_JAZ_LOCAL_ENABLED?: string
  readonly VITE_POSTHOG_TOKEN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare namespace JSX {
  interface IntrinsicElements {
    webview: React.DetailedHTMLProps<
      React.HTMLAttributes<HTMLElement> & {
        partition?: string
        src?: string
      },
      HTMLElement
    >
  }
}
