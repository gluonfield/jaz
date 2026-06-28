// Single source of truth for usage share-visualization colors, shared by the
// agent/model pies and the activity stacked bar so the same hue can't drift
// between adjacent breakdowns. Pies assign by rank index; the activity bar
// assigns a fixed hue per category.
export const USAGE_SHARE_PALETTE = [
  'var(--color-primary)',
  'var(--color-accent)',
  'oklch(0.62 0.13 150)',
  'oklch(0.6 0.15 305)',
  'oklch(0.66 0.12 200)',
  'oklch(0.62 0.17 350)',
]

// Color for grouped "other"/uncategorized segments.
export const USAGE_SHARE_OTHER_COLOR = 'var(--color-ink-3)'
