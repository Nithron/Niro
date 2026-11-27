import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Card, LinkButton, Logo, Status } from '../components/ui'

export function LoginPage() {
  const { t } = useTranslation()
  const status = new URLSearchParams(window.location.search).get('login')

  return (
    <div className="flex min-h-dvh items-center justify-center px-4">
      <Card className="w-full max-w-md p-8 text-center">
        <Logo className="mx-auto h-10" />
        <p className="mt-3 text-sm text-muted">{t('app.tagline')}</p>
        <h1 className="mt-8 text-xl font-semibold">{t('login.title')}</h1>
        <p className="mt-2 text-muted">{t('login.body')}</p>
        {status === 'failed' && (
          <p role="alert" className="mt-4 text-sm">
            <Status state="failed">{t('login.failed')}</Status>
          </p>
        )}
        {status === 'cancelled' && <p className="mt-4 text-sm">{t('login.cancelled')}</p>}
        <LinkButton href="/auth/login" className="mt-6 w-full py-3">
          {t('login.button')}
        </LinkButton>
        <p className="mt-6 text-xs text-muted">{t('login.privacy')}</p>
        <p className="mt-3 flex justify-center gap-4 text-xs text-muted">
          <Link to="/terms" className="underline underline-offset-2 hover:text-fg">
            {t('legal.terms')}
          </Link>
          <Link to="/privacy" className="underline underline-offset-2 hover:text-fg">
            {t('legal.privacy')}
          </Link>
        </p>
      </Card>
    </div>
  )
}
