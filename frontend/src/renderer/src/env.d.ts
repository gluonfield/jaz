declare module '*.css'
declare module '@fontsource-variable/inter'
declare module '@fontsource-variable/jetbrains-mono'

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
