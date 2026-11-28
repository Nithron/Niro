/**
 * A date and time as the dashboard shows it: "2026-11-22 13:00" in the reader's own time zone, in the mono face.
 * One form for every reader: no month names, no 12-hour clock.
 */
export function formatDateTime(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
