import { app } from 'electron'
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import process from 'node:process'
import console from 'node:console'

const output = process.env.JAZ_UPDATER_SMOKE_DIR
if (process.platform === 'darwin') app.setActivationPolicy('prohibited')
app.disableHardwareAcceleration()
app.setPath('userData', join(output, 'profile'))
writeFileSync(join(output, 'pid'), String(process.pid))
process.on('uncaughtException', (error) => {
  console.error(error)
  app.exit(1)
})
process.on('unhandledRejection', (error) => {
  console.error(error)
  app.exit(1)
})
try {
  await import('./main.js')
} catch (error) {
  console.error(error)
  app.exit(1)
}
