import { execFile } from 'node:child_process'
import { dirname, join } from 'node:path'
import { promisify } from 'node:util'
import { app } from 'electron'
import { z } from 'zod'
import type { FocusState } from '@shared/notifications'

const execute = promisify(execFile)

const focusSchema = z.enum(['on', 'off', 'unknown']) satisfies z.ZodType<FocusState>
const statusSchema = z.object({
  model: z.string(),
  screens: z.array(z.object({
    id: z.number(),
    notch: z.object({ x: z.number(), width: z.number(), height: z.number() }).nullable(),
  })),
  focus: focusSchema,
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

export async function requestFocusStatus(): Promise<FocusState> {
  return z.object({ focus: focusSchema }).safeParse(await run(['request-focus'], 120_000)).data?.focus ?? 'unknown'
}

export function announce(text: string): void {
  void run(['announce', text], 5_000)
}
