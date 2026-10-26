import type {} from './renderer'
import '@/styles/globals.css'
import { exerciseChatHistory } from './chat-history'
import { exerciseBotWelcome } from './bot-welcome'

async function run() {
  await exerciseChatHistory()
  await exerciseBotWelcome()
  window.smoke.result({ ok: true, checks: ['paged and live bot history; internal coordination hidden; human sends and intentional deliveries retained; whitespace preserved'] })
}

void run().catch((error) => window.smoke.result({ ok: false, error: String(error) }))
