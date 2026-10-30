import type {} from './renderer'
import '@/styles/globals.css'
import { exerciseChatHistory } from './chat-history'
import { exerciseBotWelcome } from './bot-welcome'

async function run() {
  await exerciseChatHistory()
  await exerciseBotWelcome()
  window.smoke.result({ ok: true, checks: ['artifact live/reload/replay/prompt/error rendering; paged and live bot history; human sends retained; stale session refresh cannot erase acknowledged queue; queue removal, drain and metadata/history refresh'] })
}

void run().catch((error) => window.smoke.result({ ok: false, error: String(error) }))
