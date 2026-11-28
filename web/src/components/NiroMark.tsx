import { clsx } from 'clsx'

/**
 * Niro's mark, "In view": four corners holding the "in order" square in view (assets/brand/mark.json, picked
 * 2026-11-28). Drawn on the 16 grid in ink (currentColor), at 16, 32 or 48 px only, so every grid unit is whole
 * pixels. It is a trademark: show it only on Nithron's own deployment (isNithronSite), never in a self-hosted copy.
 */
export const markRows = [
  '................',
  '.####......####.',
  '.####......####.',
  '.##..........##.',
  '.##..........##.',
  '................',
  '......####......',
  '......####......',
  '......####......',
  '......####......',
  '................',
  '.##..........##.',
  '.##..........##.',
  '.####......####.',
  '.####......####.',
  '................',
]

const d = markRows.flatMap((row, y) => [...row.matchAll(/#+/g)].map((m) => `M${m.index} ${y}h${m[0].length}v1h-${m[0].length}z`)).join('')

export function NiroMark({ size = 32, className }: { size?: 16 | 32 | 48; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="currentColor" shapeRendering="crispEdges" aria-hidden="true" focusable="false" className={clsx('block flex-none', className)}>
      <path d={d} />
    </svg>
  )
}
