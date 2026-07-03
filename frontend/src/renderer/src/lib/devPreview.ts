// Dev-only URL pins (`?launch`, `?onboarding`): keep a boot surface open in a
// browser for visual iteration against a live backend. Always null in
// packaged builds — import.meta.env.DEV compiles the check away.
export function devPreview(name: string): string | null {
  if (!import.meta.env.DEV) return null
  return new URLSearchParams(window.location.search).get(name)
}
