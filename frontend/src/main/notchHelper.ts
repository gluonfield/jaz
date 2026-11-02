import { execFile } from 'node:child_process'
import { dirname, join } from 'node:path'
import { promisify } from 'node:util'
import { app } from 'electron'
import { z } from 'zod'
import type { NotchAccess } from '@shared/notifications'

const execute = promisify(execFile)

const statusSchema = z.object({
  model: z.string(),
  screens: z.array(z.object({
    id: z.number(),
    notch: z.object({ x: z.number(), width: z.number(), height: z.number() }).nullable(),
  })),
  focus: z.enum(['on', 'off', 'unknown']),
  focus_access: z.enum(['authorized', 'denied', 'restricted', 'not_determined', 'unknown']),
  previews: z.enum(['always', 'when_unlocked', 'never', 'unknown']),
  notifications: z.enum(['authorized', 'provisional', 'denied', 'not_determined', 'unknown']),
})

export type NotchStatus = z.infer<typeof statusSchema>

async function run(args: string[], timeout: number): Promise<unknown> {
  if (process.platform !== 'darwin') {
    return undefined
  }
  try {
    const helper = app.isPackaged ? join(dirname(process.execPath), 'jaz-notch') : join(app.getAppPath(), 'resources/macos/jaz-notch')
    const { stdout } = await execute(helper, args, { timeout })
    return JSON.parse(stdout)
  } catch {
    return undefined
  }
}

export async function notchStatus(): Promise<NotchStatus | null> {
  return statusSchema.safeParse(await run(['status'], 3_000)).data ?? null
}

export async function notchAccess(): Promise<NotchAccess> {
  return access(await notchStatus())
}

// Asks macOS once to share Focus status; only a person's choice reaches this.
export async function shareFocus(): Promise<NotchAccess> {
  const status = await notchStatus()
  return access(status?.focus_access === 'not_determined' ? statusSchema.safeParse(await run(['request-focus'], 120_000)).data ?? null : status)
}

function access(status: NotchStatus | null): NotchAccess {
  if (!status) return 'unavailable'
  if (status.notifications !== 'authorized') return 'notifications'
  switch (status.focus_access) {
    case 'authorized':
      return 'ready'
    case 'not_determined':
      return 'ask'
    case 'denied':
      return 'denied'
    default:
      return 'unavailable'
  }
}

export function announce(text: string): void {
  void run(['announce', text], 5_000)
}
