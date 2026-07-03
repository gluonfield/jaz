export function formatTokens(value = 0): string {
  if (value < 1_000) return String(value)
  if (value < 1_000_000) return (value / 1_000).toFixed(2) + 'k'
  if (value < 1_000_000_000) return (value / 1_000_000).toFixed(2) + 'M'
  return (value / 1_000_000_000).toFixed(2) + 'B'
}
