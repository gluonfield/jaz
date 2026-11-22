declare module 'sax' {
  interface Parser {
    onopentag: (tag: { local: string; attributes: Record<string, { local: string; value: string }> }) => void
    onclosetag: () => void
    ontext: (text: string) => void
    write(xml: string): Parser
    close(): void
  }

  export function parser(strict: true, options: { xmlns: true }): Parser
}
