export function formatTokens(value = 0): string {
  if (value < 1_000) return String(value)
  if (value < 1_000_000) return trimZero((value / 1_000).toFixed(1)) + 'k'
  return trimZero((value / 1_000_000).toFixed(1)) + 'M'
}

function trimZero(value: string): string {
  return value.endsWith('.0') ? value.slice(0, -2) : value
}
