/**
 * Whether this dashboard is Nithron's own deployment (niro.nithron.com). The API adds
 * <meta name="niro-deployment" content="nithron"> to index.html when NIRO_NITHRON_SITE is true; then the
 * dashboard shows the Nithron family bar and Niro's marks. Self-hosted copies leave it off
 * (TRADEMARK_POLICY.md). In development, VITE_NITHRON_SITE=true does the same.
 */
export function isNithronSite(): boolean {
  if (import.meta.env.VITE_NITHRON_SITE === 'true') return true
  return document.querySelector('meta[name="niro-deployment"]')?.getAttribute('content') === 'nithron'
}
