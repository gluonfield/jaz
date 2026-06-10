import type { InputHTMLAttributes } from 'react'

// The single text-input primitive — Button's counterpart for fields.
export function Input({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`w-full rounded-control bg-bg px-3 py-2 text-[13px] text-ink ring-1 ring-border outline-none transition duration-150 placeholder:text-ink-3 focus:ring-primary ${className}`}
      {...props}
    />
  )
}
