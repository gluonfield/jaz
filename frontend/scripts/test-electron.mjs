import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'

const directory = dirname(createRequire(import.meta.url).resolve('electron/package.json'))
export const electron = join(directory, 'dist', readFileSync(join(directory, 'path.txt'), 'utf8').trim())
