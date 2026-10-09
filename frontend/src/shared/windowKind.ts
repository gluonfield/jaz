// Secondary windows are spawned with `--jaz-<kind>-window` so the renderer can
// drop the app chrome and render that surface full-bleed.
export const SECONDARY_WINDOW_KINDS = ['board', 'voice', 'launcher', 'permission'] as const

export type WindowKind = 'main' | (typeof SECONDARY_WINDOW_KINDS)[number]
