import { Boxes } from 'lucide-react'

// Brand marks for the model providers jaz connects to, drawn monochrome via
// currentColor so they tint to the design tokens (ink) and read in both themes.
// Kept as inline SVG so they load from file:// in the packaged app with no extra
// request — mirrors AgentLogo. Predefined providers carry their own glyph; an
// unknown/custom provider falls back to a generic mark. Swap a path here to drop
// in an exact brand asset later.

type Props = { provider: string; className?: string; size?: number }

// Per-mark optical sizing: each viewBox fills its frame differently, so nudge the
// rendered box so every glyph reads at the same visual weight.
const SIZES: Record<string, number> = {
  openai: 18,
  openrouter: 19,
  ollama: 19,
}

function canonical(provider: string): string {
  return provider.trim().toLowerCase()
}

export function hasProviderLogo(provider: string): boolean {
  return canonical(provider) in SIZES
}

export function ProviderLogo({ provider, className = '', size }: Props) {
  const slug = canonical(provider)
  const base = SIZES[slug] ?? 18
  const px = size ? Math.round((base / 18) * size) : base
  const dims = { width: px, height: px }
  const cls = `shrink-0 ${className}`

  // OpenAI — the official knot, sourced from the published logo.
  if (slug === 'openai') {
    return (
      <svg viewBox="0 0 24 24" style={dims} className={cls} fill="currentColor" aria-hidden="true">
        <path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z" />
      </svg>
    )
  }

  // OpenRouter — a routing fan-out: one source node branching to many models.
  if (slug === 'openrouter') {
    return (
      <svg
        viewBox="0 0 24 24"
        style={dims}
        className={cls}
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <path d="M6 12h3.5" />
        <path d="M9.5 12 15 7.2" />
        <path d="M9.5 12 15 16.8" />
        <circle cx="4.4" cy="12" r="1.7" fill="currentColor" stroke="none" />
        <circle cx="17.6" cy="6.6" r="1.7" fill="currentColor" stroke="none" />
        <circle cx="17.6" cy="17.4" r="1.7" fill="currentColor" stroke="none" />
      </svg>
    )
  }

  // Ollama — the llama silhouette, simplified to a single monochrome mark.
  if (slug === 'ollama') {
    return (
      <svg viewBox="0 0 24 24" style={dims} className={cls} fill="currentColor" aria-hidden="true">
        <path d="M7.4 2.1c-.86 0-1.55.74-1.55 1.64v1.3c0 .3.03.58.1.85-.96.66-1.6 1.78-1.6 3.06v.46c-.78.5-1.3 1.4-1.3 2.43 0 .73.26 1.4.69 1.9-.3.5-.48 1.1-.48 1.74 0 1.05.47 1.99 1.2 2.6-.06.27-.1.55-.1.84 0 .43.34.78.76.78h.83c.05.62.55 1.1 1.16 1.1.5 0 .94-.33 1.1-.8h5.18c.16.47.6.8 1.1.8.61 0 1.11-.48 1.16-1.1h.83c.42 0 .76-.35.76-.78 0-.29-.04-.57-.1-.84.73-.61 1.2-1.55 1.2-2.6 0-.63-.18-1.23-.48-1.74.43-.5.69-1.17.69-1.9 0-1.03-.52-1.93-1.3-2.43V8.9c0-1.28-.64-2.4-1.6-3.06.07-.27.1-.55.1-.85v-1.3c0-.9-.7-1.64-1.55-1.64-.86 0-1.55.74-1.55 1.64v.9c-.3-.05-.6-.08-.92-.08s-.62.03-.92.08v-.9c0-.9-.7-1.64-1.55-1.64Zm1.4 9.1c.52 0 .94.45.94 1s-.42 1-.94 1-.94-.45-.94-1 .42-1 .94-1Zm6.4 0c.52 0 .94.45.94 1s-.42 1-.94 1-.94-.45-.94-1 .42-1 .94-1Zm-3.2 2.6c.78 0 1.45.42 1.77 1.02.13.24-.05.53-.32.53h-2.9c-.27 0-.45-.29-.32-.53.32-.6.99-1.02 1.77-1.02Z" />
      </svg>
    )
  }

  // Custom / unknown providers — a neutral mark so the row still reads as a card.
  return <Boxes size={px} className={cls} aria-hidden="true" />
}
