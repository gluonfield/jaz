import { readdirSync, readFileSync } from 'node:fs'
import { join, resolve, sep } from 'node:path'
import type { Plugin } from 'vite'

const source = resolve('node_modules/pdfjs-dist')
const folders = ['cmaps', 'iccs', 'standard_fonts', 'wasm']
const types: Record<string, string> = { js: 'text/javascript', wasm: 'application/wasm' }

// pdf.js reads its fonts, character maps, colour profile and image decoders by
// file name from a folder, so they are served as they ship, under pdfjs/.
export function pdfjsData(): Plugin {
  return {
    name: 'pdfjs-data',
    configureServer(server) {
      server.middlewares.use('/pdfjs', (request, response, next) => {
        try {
          const file = join(source, decodeURIComponent(request.url?.split('?')[0] ?? ''))
          if (!folders.some((folder) => file.startsWith(join(source, folder) + sep))) return next()
          const body = readFileSync(file)
          response.setHeader('Content-Type', types[file.split('.').at(-1) ?? ''] ?? 'application/octet-stream')
          response.end(body)
        } catch {
          next()
        }
      })
    },
    generateBundle() {
      for (const folder of folders) {
        for (const name of readdirSync(join(source, folder))) {
          this.emitFile({ type: 'asset', fileName: `pdfjs/${folder}/${name}`, source: readFileSync(join(source, folder, name)) })
        }
      }
    },
  }
}
