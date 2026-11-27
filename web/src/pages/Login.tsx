import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { FamilyBar, ProductName } from '../components/Layout'
import { Picture } from '../components/Picture'
import { LinkButton, Status } from '../components/ui'
import { isNithronSite } from '../deployment'

/** Sign-in: words and one action. The sign-in picture slot stays empty until the shared pixel set is issued. */
export function LoginPage() {
  const { t } = useTranslation()
  const status = new URLSearchParams(window.location.search).get('login')

  return (
    <div className="flex min-h-dvh flex-col">
      {isNithronSite() && <FamilyBar />}
      <div className="flex flex-1 items-center justify-center px-4 py-10">
      <main className="w-full max-w-sm">
        <Picture name="signIn" className="mb-8" />
        <h1>
          <ProductName large />
        </h1>
        <p className="mt-6 text-muted">{t('login.body')}</p>
        {status === 'failed' && (
          <p role="alert" className="mt-4 text-sm">
            <Status state="failed">{t('login.failed')}</Status>
          </p>
        )}
        {status === 'cancelled' && <p className="mt-4 text-sm">{t('login.cancelled')}</p>}
        <LinkButton href="/auth/login" className="mt-6 w-full">
          {t('login.button')}
        </LinkButton>
        <p className="mt-6 text-help text-muted">{t('login.privacy')}</p>
        <p className="mt-4 flex gap-4 text-help text-muted">
          <Link to="/terms" className="underline underline-offset-2 hover:text-fg">
            {t('legal.terms')}
          </Link>
          <Link to="/privacy" className="underline underline-offset-2 hover:text-fg">
            {t('legal.privacy')}
          </Link>
        </p>
      </main>
      </div>
    </div>
  )
}
