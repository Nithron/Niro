import { clsx } from 'clsx'
import { icons, type IconName, type IconSize } from '../vendor/nithron-icons/index.js'

export type { IconName, IconSize }

/**
 * The one place for icons. Every icon in the dashboard is a Nithron pixel icon (@nithron/icons, vendored in
 * src/vendor/nithron-icons): ink on a 16 by 16 grid, in currentColor, at 16 px or 32 px and no other size,
 * crisp, never scaled, rotated or smoothed. Where the set has no icon for a meaning, the control has words
 * only. Do not draw icons here.
 *
 * Leave `label` out when the icon stands beside its word; a button that holds only an icon needs its own name.
 */
export function Icon({ name, size = 16, label, className }: { name: IconName; size?: IconSize; label?: string; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="currentColor"
      shapeRendering="crispEdges"
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      focusable="false"
      className={clsx('block flex-none', className)}
    >
      <path d={icons[name].path} />
    </svg>
  )
}
