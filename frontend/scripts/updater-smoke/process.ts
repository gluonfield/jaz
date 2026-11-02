import { spawn as spawnProcess } from 'node:child_process'

export let ready: Promise<void>
export let child: ReturnType<typeof spawnProcess>

export const execFileSync = () => ''

export function spawn(): ReturnType<typeof spawnProcess> {
  child = spawnProcess(process.execPath, ['-e', `
process.on('SIGTERM', () => {})
process.stdout.write('ready\\n')
setInterval(() => {}, 1000)
`], {
    env: { ...process.env, ELECTRON_RUN_AS_NODE: '1' },
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  ready = new Promise((resolve) => child.stdout!.once('data', () => resolve()))
  return child
}
