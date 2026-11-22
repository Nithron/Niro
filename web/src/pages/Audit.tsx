import { useTranslation } from 'react-i18next'
import type { AuditFinding } from '../api/client'
import { useAudit } from '../api/queries'
import { Badge, Button, Card, ErrorState, Spinner } from '../components/ui'

const tone = { critical: 'danger', high: 'danger', medium: 'warning', info: 'brand' } as const

function FindingText({ f }: { f: AuditFinding }) {
  const { t } = useTranslation()
  const permissions = (f.permissions ?? []).map((p) => t(`audit.permissions.${p}` as never, { defaultValue: p })).join(', ')
  const count = f.count === undefined || f.count < 0 ? t('audit.unknownCount') : String(f.count)
  return <>{t(`audit.findings.${f.code}`, { subject: f.subject ?? '', permissions, count })}</>
}

export function AuditPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const audit = useAudit(guildId)

  return (
    <div className="max-w-3xl">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">{t('audit.title')}</h2>
          <p className="mt-1 text-muted">{t('audit.subtitle')}</p>
        </div>
        <Button variant="secondary" onClick={() => void audit.refetch()} disabled={audit.isFetching}>
          {t('audit.rerun')}
        </Button>
      </div>
      <div className="mt-6">
        {audit.isPending && <Spinner />}
        {audit.isError && <ErrorState onRetry={() => void audit.refetch()} />}
        {audit.data && (
          <ul className="grid gap-2">
            {audit.data.findings.length === 0 && (
              <Card>
                <p className="text-success">{t('audit.clean')}</p>
              </Card>
            )}
            {audit.data.findings.map((f, i) => (
              <li key={i}>
                <Card className="flex items-start gap-3 p-4">
                  <Badge tone={tone[f.severity]}>{t(`audit.severity.${f.severity}`)}</Badge>
                  <p className="text-sm">
                    <FindingText f={f} />
                  </p>
                </Card>
              </li>
            ))}
            {!audit.data.webhooksChecked && (
              <li>
                <Card className="flex items-start gap-3 p-4">
                  <Badge tone="brand">{t('audit.severity.info')}</Badge>
                  <p className="text-sm">{t('audit.webhooksUnchecked')}</p>
                </Card>
              </li>
            )}
          </ul>
        )}
      </div>
    </div>
  )
}
