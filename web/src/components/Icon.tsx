import { clsx } from 'clsx'

/**
 * The one place for icons. Every icon in the dashboard goes through this component, so the day the shared
 * Nithron pixel set arrives (16 by 16 grid, drawn once for nOS, nAI and Niro) the swap is this file alone.
 * Until then the icons are the text characters the dashboard used before. Do not draw pixel icons here.
 *
 * Two sizes only, because a pixel icon cannot be scaled by a fraction: 16 px in rows, buttons and
 * navigation; 32 px for a screen title or an empty screen.
 */
const glyphs = {
  back: '←',
  up: '↑',
  down: '↓',
  /** The mark of a notice that is no state: none of the three status marks. */
  note: 'i',
} as const

export type IconName = keyof typeof glyphs
export type IconSize = 16 | 32

export function Icon({ name, size = 16, label, className }: { name: IconName; size?: IconSize; label?: string; className?: string }) {
  return (
    <span
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      style={{ width: size, height: size, fontSize: size * 0.875 }}
      className={clsx('inline-flex shrink-0 items-center justify-center leading-none', className)}
    >
      {glyphs[name]}
    </span>
  )
}
