// Mirrors the backend selection formatter (backend/internal/acp/turns.go,
// messageWithSelections). The backend folds quotes into the prompt for an
// immediate send; the queue stores plain text only, so when a message is queued
// while a turn runs we fold the selections in here. Keep the two in sync.
export function wrapMessageWithSelections(message: string, quotes: string[]): string {
  const selections = quotes.map((quote) => quote.trim()).filter(Boolean)
  if (selections.length === 0) return message
  const block = [
    '<selected_text>',
    ...selections.map((selection, index) => `<selection n="${index + 1}">\n${selection}\n</selection>`),
    '</selected_text>',
  ].join('\n')
  return message.trim() ? `${block}\n\n${message}` : block
}
