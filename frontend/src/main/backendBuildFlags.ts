type BackendFlagOptions = {
  requireTelegramBundle?: boolean
}

const telegramClientIDSymbol =
  'github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientID'
const telegramClientHashSymbol =
  'github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientHash'

export function telegramBackendLDFlags(
  env: NodeJS.ProcessEnv,
  options: BackendFlagOptions = {},
): string[] {
  const id = env.JAZ_BUNDLED_TELEGRAM_APP_ID?.trim() ?? ''
  const hash = env.JAZ_BUNDLED_TELEGRAM_APP_HASH?.trim() ?? ''
  if (Boolean(id) !== Boolean(hash)) {
    throw new Error('JAZ_BUNDLED_TELEGRAM_APP_ID and JAZ_BUNDLED_TELEGRAM_APP_HASH must both be set')
  }
  if (options.requireTelegramBundle && !id) {
    throw new Error('Bundled Telegram app credentials are required for release backend builds')
  }
  if (id && !/^\d+$/.test(id)) {
    throw new Error('JAZ_BUNDLED_TELEGRAM_APP_ID must be numeric')
  }
  if (!id) return []
  return [`-X ${telegramClientIDSymbol}=${id}`, `-X ${telegramClientHashSymbol}=${hash}`]
}
