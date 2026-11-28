import { clsx } from 'clsx'

/**
 * The one place for the product's pixel pictures: sign-in and the pictures of empty screens (the mark is NiroMark). They are
 * drawn once for nOS, nAI and Niro by the website session and issued as files; until then every slot is empty
 * and renders nothing, so a screen stays words and one action. Do not draw pictures here.
 *
 * A picture is shown at whole-number sizes only, crisp (image-rendering: pixelated).
 */
export type PictureName = 'signIn' | 'noServers' | 'noCases' | 'noDetections' | 'noRules' | 'noPanels' | 'noTickets'

const pictures: Partial<Record<PictureName, { src: string; width: number; height: number }>> = {}

export function Picture({ name, scale = 1, className }: { name: PictureName; scale?: 1 | 2 | 3 | 4; className?: string }) {
  const p = pictures[name]
  if (!p) return null
  return <img src={p.src} alt="" width={p.width * scale} height={p.height * scale} className={clsx('pixelated', className)} />
}
